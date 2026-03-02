# Kafka Sender Type 유지/삭제 검토

> 2026-03-02 논의. 결론: **유지** (KafkaToElastic 수정 후 효율화 경로로 활용)

## 배경

ResourceAgent는 3가지 sender_type을 지원한다: `kafkarest`, `kafka`, `file`.
프로덕션 환경에서는 `kafkarest`만 사용 중이며 `kafka`는 미사용 상태.
kafka sender를 삭제할지, 유지할지 검토했다.

## 현재 프로덕션 데이터 흐름

```
Client (ARSAgent/ResourceAgent)
  → [HTTP] KafkaRest Proxy (k8s, Kafka broker와 동일 노드)
  → [TCP] Kafka
  → KafkaToElastic (Grok Parser로 평문 파싱)
  → ElasticSearch
```

- KafkaRest Proxy 주소: ServiceDiscovery(Redis)로 자동 입수
- Kafka broker 주소: ServiceDiscovery 미관리 (수동 설정 필요)

## 비효율 2가지

| 비효율 | 설명 |
|--------|------|
| HTTP 홉 | ResourceAgent → KafkaRest Proxy → Kafka. REST proxy를 거치는 불필요한 중간 단계 |
| Grok 파싱 | KafkaToElastic이 평문을 Grok 정규식으로 파싱. 이미 구조화된 데이터를 평문으로 만들었다가 다시 파싱 |

kafka sender로 JSON(ParsedDataList)을 직접 Kafka에 넣으면 두 비효율 모두 해소된다.

## 삭제 vs 유지 논의

### 삭제 찬성 논거

1. **프로덕션 미사용** — ServiceDiscovery가 Kafka broker를 관리하지 않아 10,000대 PC에 수동 설정 필요
2. **sarama 의존성 제거** — 바이너리 크기 감소, 빌드 단순화
3. **코드 유지 부담** — SaramaTransport ~250줄, KafkaConfig 13개 필드, 두 포맷 동시 테스트
4. **git history로 복원 가능** — 삭제해도 코드는 보존됨

### 유지 찬성 논거

1. **효율화 경로** — kafka 직접 연결 + JSON 포맷으로 HTTP 홉과 Grok 파싱 모두 제거 가능
2. **이미 완성된 코드** — TLS, SASL, SOCKS5, 압축 모두 구현 완료, 런타임 비용 0
3. **KafkaToElastic이 JSON ParsedDataList를 이미 지원** (아래 분석 참조)

## KafkaToElastic 분석 결과

### Consumer 타입 4가지

| Consumer | parserType | raw 필드 처리 |
|----------|-----------|-------------|
| ConsumerGrok | `"GROK"` | Grok 패턴으로 평문 파싱 |
| **ConsumerNoParser** | `"NO"` | **JSON ParsedDataList 직접 파싱** |
| ConsumerCustom | `"CUSTOM"` | Scala 런타임 파싱 |
| ConsumerResource | `"RESOURCE"` | Grok 파싱 (리소스 전용) |

### ParsedDataList 호환성

KafkaToElastic의 `Common.scala`:
```scala
case class ParsedData(field:String, value:String, dataformat:String)
case class ParsedDataList(iso_timestamp:String, parsed: List[ParsedData])
```

ResourceAgent의 `grokformat.go`:
```go
type ParsedData struct {
    Field      string `json:"field"`
    Value      string `json:"value"`
    DataFormat string `json:"dataformat"`
}
type ParsedDataList struct {
    ISOTimestamp string       `json:"iso_timestamp"`
    Parsed      []ParsedData `json:"parsed"`
}
```

**구조 완벽 일치.** `ConsumerNoParser`가 정확히 이 형식을 처리하도록 설계되어 있다.

### 호환성 검증 상세

| 항목 | 결과 | 비고 |
|------|:----:|------|
| KafkaValue의 `process` extra field | OK | json4s가 무시 |
| `iso_timestamp` 파싱 | OK | Joda-time ISODateTimeFormat 호환 |
| `EARS_TIMESTAMP` 생성 | OK | `iso_timestamp`에서 자동 생성 |
| `dataformat` 타입 매칭 | OK | String/Integer/Double 정확히 일치 |
| `EARS_FILENAME` | OK | actor 설정에서 지정 |
| **토픽 라우팅** | **문제** | 아래 참조 |

### 토픽 라우팅 문제

ResourceAgent 토픽(`tp_{process}_all_resource`)은 `ConsumerResource`로 라우팅되는데,
`ConsumerResource`는 **Grok 파싱만 지원**한다. JSON ParsedDataList를 보내면 파싱 실패.

```
ResourceAgent (kafka sender, JSON) → topic: tp_PROC1_all_resource
  → ConsumerResource (Grok 파싱) → Grok으로 JSON 파싱 시도 → 실패!
```

같은 토픽에 Grok과 JSON이 섞여도 Kafka 자체는 문제없지만,
consumer가 하나의 parserType만 처리하므로 혼합 불가.

## 해결 방안: ConsumerResource 수정

### 변경 내용

ConsumerResource의 `runParsing`에서 `raw` 필드 포맷을 감지하여 분기:

```scala
if (kafkaMsg.raw.trim().startsWith("{")) {
  // JSON ParsedDataList 경로
  val parsed = parse(kafkaMsg.raw).extract[ParsedDataList]
  parsed.parsed.foreach(p => {
    data(p.field) = convertValue(p.value, p.dataFormat)
  })
} else {
  // Grok 경로 (기존 로직)
  for (_gkIndex <- 0 to GrokList.length-1 if !matched) { ... }
}
```

### 포맷 감지 안전성

- Grok 평문: `2026-03-02 10:30:45,123 category:cpu,...` — 절대 `{`로 시작 안 함
- JSON: 반드시 `{`로 시작
- false positive/negative 가능성: **0**

### 변경 규모

- 신규 코드: ~30줄 (ConsumerNoParser의 ParsedDataList 처리 로직 복사)
- 기존 Grok 경로: 변경 없음
- 복잡도 증가: 10.8% (279줄 → 309줄)

### 기존 버그 (함께 수정 권장)

ConsumerResource.scala에 오타 버그 3건:
- `bulkLists.lenth` → `bulkLists.length` (2곳)
- `bulkLbulkLists` → `bulkLists` (1곳)
- `caes _` → `case _` (1곳)

## 최종 결론

### kafka sender: 유지

삭제하면 효율화 경로가 닫힌다. 현재 미사용이지만 런타임 비용 0이며,
KafkaToElastic ConsumerResource 수정(~30줄)만으로 아래 목표 구조를 달성할 수 있다:

```
ARSAgent (기존)        → kafkarest → Kafka (Grok 평문) ─┐
                                                        ├→ ConsumerResource → ES
ResourceAgent (신규)   → kafka 직접 → Kafka (JSON)     ─┘
                                      (같은 토픽, 포맷 자동 감지)
```

### 남은 블로커

| 블로커 | 설명 | 해결 방법 |
|--------|------|----------|
| KafkaToElastic 수정 | ConsumerResource가 JSON 미지원 | `raw.startsWith("{")` 분기 추가 (~30줄) |
| Kafka broker 주소 | ServiceDiscovery 미관리 | SD에 Kafka 등록 추가, 또는 설정 파일 직접 지정 |

### 실행 순서 (향후)

1. KafkaToElastic ConsumerResource 수정 (JSON ParsedDataList 지원 + 버그 수정)
2. 테스트 환경에서 ResourceAgent kafka sender → KafkaToElastic 검증
3. ServiceDiscovery에 Kafka broker 등록 방안 결정
4. 프로덕션 점진적 전환 (kafkarest → kafka)
