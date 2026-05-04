# BufferedHTTPTransport (KafkaRest 버퍼) 모니터링 가이드

> **대상**: ResourceAgent 운영자 / 현장 담당자 / 개발자
> **연관 변경**: Phase 2-1 — `BufferedHTTPTransport` 버퍼 상한 (Oldest-drop FIFO + atomic 카운터 + sampled 로깅)
> **연관 plan**: `docs/plans/memory-leak-mitigation-plan.md` (v2.4.2)
> **자매 가이드**:
> - `docs/runbooks/lhm-provider-timeout-monitoring.md` (Phase 1-1)
> - `docs/runbooks/wmi-query-monitoring.md` (Phase 1-2)

---

## TL;DR — "한 번에 봐야 할 한 줄"

```bash
grep "BUFFER_DROP_OLDEST" log/ResourceAgent/ResourceAgent.log | tail -20
```

이 결과가:
- **거의 비어있음** → 정상. KafkaRest 정상 송신 중. 작업 끝.
- 가끔 보임 (시간당 1~2건) → KafkaRest 일시 unreachable 후 회복. 정상 운영 가능.
- 빈발 (시간당 10건+, `dropped_total` 급증) → KafkaRest endpoint 또는 네트워크 지속 문제. Q3 진단.

---

## 배경 — 왜 버퍼 상한이 필요한가

`BufferedHTTPTransport`는 KafkaRest로의 HTTP POST를 비동기로 처리. 정상 환경에서는 매 5분 collect cycle 후 즉시 flush → buffer는 거의 비어있음.

**문제 시나리오** (수정 전):
- KafkaRest endpoint가 unreachable → flush 실패
- 다음 cycle도 실패 → buffer에 records 계속 누적
- 누적되는 buffer는 **상한이 없음** → RSS 무한 증가 → 결국 OOM
- 이 누수는 v2.4.0에서 확정한 EPS-induced Paged Pool 증가와 **구분 어려움** (둘 다 시스템 메모리 증가)

**Phase 2-1 수정**:
- `MaxBufferedRecords` (기본 10,000 ≈ 3MB) 상한 도입
- 초과 시 **oldest-drop** (FIFO) — 최근 데이터 보존 우선
- mu critical section 안에서 enforcement → race-free
- atomic 관측 카운터로 외부 SelfMetrics가 lock-free 조회 가능
- BasicSampler{N:10}로 drop burst 시 로그 폭주 방지 (atomic counter는 정확)

### 다른 두 작업(Phase 1-1, 1-2)과의 차이

| 항목 | Phase 1-1 (LhmProvider) | Phase 1-2 (WMI) | Phase 2-1 (HTTP buffer) |
|------|----------------------|-----------------|------------------------|
| 누수 종류 | goroutine 누수 (영구 block) | goroutine 누수 (영구 block) | **메모리 누수 (slice 무한 증가)** |
| 회수 메커니즘 | Process.Kill | bounded leak (1개 한정) | Oldest-drop FIFO |
| 데이터 영향 | 1회 메트릭 손실 + 재시작 | inflight 동안 stale cache | drop된 records 영구 손실 |
| 보장 | 누수 0 | 1 goroutine 한정 | RSS bounded |

---

## 추가된 로그 prefix (1개)

`[kafkarest-buffer]` 컴포넌트에서 발생.

| Prefix | Level | 발생 조건 | 샘플링 | 함께 보이는 필드 |
|--------|-------|----------|--------|----------------|
| `BUFFER_DROP_OLDEST` | ERROR | buffer가 `MaxBufferedRecords` 초과 → 가장 오래된 entry FIFO drop | **1/10 (BasicSampler)** | `dropped_records`, `buffer_count`, `max_buffered_records`, `dropped_total` |

> **샘플링 주의**: 로그는 10번 중 1번만 기록되지만 `dropped_total` 카운터는 **모든 drop을 정확히 누적**. 운영자는 마지막 로그의 `dropped_total` 값으로 정확한 누수 규모 파악.

### 로그 포맷 예시 (FixedFormatWriter)

```
2026-05-04 14:23:12.456 [ERR] [kafkarest-buffer] BUFFER_DROP_OLDEST oldest records dropped due to buffer cap (sampled 1/10) dropped_records=50 buffer_count=10000 max_buffered_records=10000 dropped_total=523
2026-05-04 14:33:42.789 [ERR] [kafkarest-buffer] BUFFER_DROP_OLDEST oldest records dropped due to buffer cap (sampled 1/10) dropped_records=50 buffer_count=10000 max_buffered_records=10000 dropped_total=1247
```

---

## Q1. "drop이 발생하고 있는가?" — 가장 먼저 보는 것

### 명령

```bash
# Windows PowerShell
Get-Content "D:\EARS\EEGAgent\log\ResourceAgent\ResourceAgent.log" | Select-String "BUFFER_DROP_OLDEST" | Select-Object -Last 20

# Linux/macOS (로그 수집 후 분석)
grep "BUFFER_DROP_OLDEST" log/ResourceAgent/ResourceAgent.log | tail -20
```

### 판정 기준

| 결과 | 해석 | 조치 |
|------|------|------|
| **결과가 비어있음** | KafkaRest 송신 정상. 매우 좋은 상태. | 작업 끝. |
| 1~2건 (지난 24h 기준) | KafkaRest 일시 hiccup → 회복 후 정상. | 정상. KafkaRest endpoint 안정성 모니터링 권장. |
| 시간당 5건 이상 | KafkaRest 또는 네트워크 지속 문제 | Q3 진단. |
| 매 cycle (5분) 마다 발생 | KafkaRest 영구 unreachable. cap이 회수만 보장하고 데이터는 계속 손실 중. | **즉시** Q3 + 인프라 점검. |

---

## Q2. "drop 누적 규모는?" — 데이터 손실량 측정

가장 최근 로그의 `dropped_total` 필드가 프로세스 lifetime 누적 drop record 수.

### 명령

```bash
# 최근 dropped_total 값
grep "BUFFER_DROP_OLDEST" log/ResourceAgent/ResourceAgent.log | tail -1 | grep -oP 'dropped_total=\K\d+'

# 시간대별 dropped_total 추이 (시간당 증가량 파악)
grep "BUFFER_DROP_OLDEST" log/ResourceAgent/ResourceAgent.log | awk '{print $1, substr($2, 1, 5)}' | sort | uniq -c | tail -24
```

### 판정

- `dropped_total` 0~수백: 정상 운영 noise
- 시간당 1,000+ 증가: **매 cycle 손실 중**. 즉시 KafkaRest 점검 필요
- 일별 100,000+ 누적: 메트릭 일관성 깨짐. 사용자 알림 + 서비스 재시작 검토

### 데이터 손실량 추정

- 1 record ≈ 1개 메트릭 데이터 포인트 (CPU/Memory/Disk 등)
- 5분 cycle에 ~수십~수백 records 생성 (수집기 활성화 수에 따라)
- `dropped_total = 10000` 이면 약 100~500 cycle (8~40시간) 분량 손실

---

## Q3. "왜 KafkaRest 송신이 실패하는가?" — 원인 진단

### 같은 로그 파일에서 KafkaRest 송신 실패 로그 확인

```bash
# Buffered KafkaRest send 실패 로그 (sender 자체 로그)
grep "Buffered KafkaRest send failed" log/ResourceAgent/ResourceAgent.log | tail -20
```

### 가능한 원인

| 원인 | 진단 신호 | 조치 |
|------|----------|------|
| KafkaRest endpoint 자체 다운 | `connection refused` 또는 `no route to host` 빈발 | KafkaRest 서비스 점검 (Ops 팀) |
| ServiceDiscovery 응답 변경 (endpoint stale) | 로그에 `KafkaRest URL` 변경 없는데 실패 | ServiceDiscovery 재조회 트리거 (재시작 또는 hot reload) |
| 네트워크 단절 (SOCKS proxy 포함) | `dial tcp ... timeout` | 네트워크 인프라 점검 |
| HTTP 4xx/5xx (인증 / 잘못된 topic) | `KafkaRest returned HTTP 4xx` 또는 `5xx` | KafkaRest 설정 점검, topic 존재 여부 |
| Backpressure (KafkaRest가 느림) | `HTTP request failed: context deadline exceeded` | KafkaRest 처리 용량 / Kafka broker 상태 |

---

## Q4. "RSS가 안정적인가?" — 본 작업의 본질적 검증

상한 enforcement가 정상 동작하면 KafkaRest가 영구 단절돼도 RSS는 **±3MB 이내로 안정**해야 함.

### Windows Task Manager
1. Task Manager → Details 탭
2. `ResourceAgent.exe`의 **Working Set (Memory)** 컬럼
3. KafkaRest 단절 상태에서 1시간 후 비교

### 정상/비정상 판정

| 시나리오 | 정상 RSS | 비정상 신호 |
|---------|----------|------------|
| KafkaRest 정상 | 시작 RSS + ±5MB | - |
| KafkaRest 단절 1시간 | 시작 RSS + ±10MB (3MB buffer + 기타) | +50MB 이상 → 본 enforcement 우회 의심 |
| KafkaRest 단절 24시간 | 시작 RSS + ±10MB (안정) | 시간 따라 선형 증가 → enforcement 미동작 |

---

## Q5. "Buffer high water mark 추세" — 사이즈 적정성 평가

`BufferStats()` 메서드가 `bufferHighWaterMark`를 노출하지만, 본 phase 작업에는 **자동 메트릭 송신 없음**. Phase 2.5 SelfMetrics 도입 시 통합.

### 임시 검증 방법 (debug 빌드)

`net/http/pprof` endpoint를 일시 활성화 시:
```bash
curl http://localhost:6060/debug/vars | jq .bufferStats
```

또는 운영 1주차에 직접 체크 코드 일시 삽입.

### MaxBufferedRecords 조정 가이드

- hwm이 cap의 50% 미만 → 여유 충분. 기본값 유지.
- hwm이 cap의 80% 이상 (drop 없이) → 정상 부하가 cap에 근접. **cap 증가 검토** (예: 10000 → 30000).
- hwm = cap (drop 발생) → 부하 vs cap 불균형. 두 가지 옵션:
  - cap 증가 (RSS 더 사용 가능하면)
  - flush 주기 단축 또는 KafkaRest 처리 용량 확장

---

## 운영 1주차 체크리스트 (배포 후 권장)

```bash
DATE=$(date +%Y-%m-%d)
LOG=log/ResourceAgent/ResourceAgent.log

echo "=== $DATE BufferedHTTPTransport Daily Check ==="

echo "--- BUFFER_DROP_OLDEST 발생 횟수 (오늘) ---"
grep "$DATE.*BUFFER_DROP_OLDEST" $LOG | wc -l

echo "--- 가장 최근 dropped_total ---"
grep "BUFFER_DROP_OLDEST" $LOG | tail -1 | grep -oP 'dropped_total=\K\d+'

echo "--- KafkaRest send 실패 (오늘) ---"
grep "$DATE.*Buffered KafkaRest send failed" $LOG | wc -l

echo "--- ResourceAgent.exe RSS (현재) ---"
# Windows PowerShell:
# (Get-Process ResourceAgent).WorkingSet64
```

### Day 7 종합 판정

- 모두 0~소수 → ✅ Phase 2-1 정상 완료
- `BUFFER_DROP_OLDEST` 0건 + `dropped_total` 0 → ✅ 정상 (KafkaRest 안정)
- `BUFFER_DROP_OLDEST` 일별 1~10건 + RSS 안정 → ✅ enforcement 동작 (KafkaRest 일시 hiccup 회수 중)
- RSS 시간 따라 선형 증가 → ❌ enforcement 우회 의심 → 즉시 분석
- `dropped_total` 일별 10,000+ → ⚠️ 메트릭 손실 큼 → KafkaRest 인프라 점검

---

## 알람/대시보드 권장 패턴 (장기 — Phase 2.5 SelfMetrics 도입 시)

| 지표 | 임계 | 의미 |
|------|------|------|
| `dropped_total` 시간당 증가량 | > 1,000 | KafkaRest 지속 문제 |
| `bufferHighWaterMark` | > MaxBufferedRecords × 0.8 | cap 임박 — 증가 검토 |
| `bufferCount` 1시간 평균 | > MaxBufferedRecords × 0.5 | 평소 부하가 cap의 절반 이상 |
| ResourceAgent.exe RSS | 시작 + 50MB 이상 | enforcement 우회 의심 |

본 phase에는 자동 노출 없음. 운영 1주차에 위 명령으로 수동 체크 권장.

---

## 롤백 트리거

다음 중 하나라도 발생하면 이전 버전으로 롤백 또는 Hot-fix:

1. ResourceAgent.exe RSS가 24h 안에 시작 + 50MB 이상 증가 (enforcement 우회)
2. `BUFFER_DROP_OLDEST` 가 정상 환경(KafkaRest 정상)에서 발생 — 정상 부하 < cap 가정 위반
3. `panic: runtime error` (kafkarest 관련 stack)

### Hot-fix (롤백 없이 enforcement 비활성)

긴급 시 config의 `MaxBufferedRecords=0`으로 변경하면 enforcement 무력화 → 기존 동작 회귀:

```json
{
  "Batch": {
    "MaxBufferedRecords": 0
  }
}
```

단, 이 hot-fix는 **OOM 위험을 감수**한다는 의미. KafkaRest 단절 시 무한 buffer 누적 가능.

---

## FAQ

### Q. 매 5분마다 BUFFER_DROP_OLDEST 1건씩 보임. 정상인가?
A. 비정상. 매 cycle drop은 KafkaRest가 영구 단절되었다는 신호. cycle당 records가 cap을 초과하지 않으면 cap에 도달조차 안 함. → KafkaRest endpoint 점검 + ServiceDiscovery 재조회.

### Q. dropped_total은 누적값인가?
A. **프로세스 lifetime** 누적값. ResourceAgent 재시작 시 0으로 리셋. 일별/주별 추세는 24h 후 값에서 24h 전 값을 빼서 계산.

### Q. BasicSampler N=10 때문에 처음 9건 drop을 놓치지 않나?
A. 로그 message는 1/10만 기록되지만 atomic counter (`dropped_total`)는 모든 drop을 정확히 누적. 첫 로그가 보이는 시점의 `dropped_total` 값으로 그 시점까지의 정확한 drop 수 확인 가능.

### Q. cap을 늘리면 데이터 손실 0이 되나?
A. 아니요. cap 증가는 **회수 시간 연장**. KafkaRest가 영구 단절되면 cap × N 시간 후 drop 시작. 근본 해결은 KafkaRest 가용성 확보.

### Q. flush 실패 시에도 cap이 적용되나?
A. **현재 설계는 enqueue 경로에만 cap 적용**. flush 실패 시 records는 무조건 drop (기존 동작 유지, plan v2.3에서 정책 유지 결정). flush 실패는 별도 로그(`Buffered KafkaRest send failed after all retries, dropping batch`)로 추적.

### Q. MaxBufferedRecords=0으로 두면 어떻게 되나?
A. enforcement 비활성. 이전 (Phase 2-1 적용 전) 동작과 동일 — 무제한 buffer. 테스트 호환성 + 긴급 hot-fix 용도. 프로덕션에서는 권장 안 함.

---

## 참고 문서

- 메모리 누수 대응 전체 plan: `docs/plans/memory-leak-mitigation-plan.md` (v2.4.2)
- Phase 1-1 LhmProvider 가이드: `docs/runbooks/lhm-provider-timeout-monitoring.md`
- Phase 1-2 WMI Query 가이드: `docs/runbooks/wmi-query-monitoring.md`
- 코드: `internal/sender/kafkarest.go` (특히 `BufferedHTTPTransport`, `Deliver`, `BufferStats`)
- Config: `internal/config/config.go` (특히 `BatchConfig.MaxBufferedRecords`)
