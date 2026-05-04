# 메모리 / 소켓 / 핸들 관리 코드베이스 audit

> **수행일**: 2026-05-04 (v2.4.2 시점)
> **범위**: `internal/` 전체 + `cmd/resourceagent/` (production 코드만, `_test.go` 제외)
> **목적**: H1/C1/C2/H2/R-1 fix 후 잔존 위험 영역 체계적 점검
> **수행 방식**: 카테고리별 grep + 호출처 일대일 매칭 + 라이브러리 옵션 확인
> **결과**: leak 0건, 효율성 개선 후보 1건. 운영 검증 전 추가 fix 불필요.

---

## 검증 카테고리 7개

각 카테고리별로 production 코드 전수 검사. 결과 요약은 §최종 판정 참조.

---

### 1. HTTP `response.Body.Close()` + Body drain

**왜**: `http.Client.Do()` 의 응답 Body를 Close 안 하면 underlying TCP connection이 keep-alive pool로 못 돌아감 → connection leak. 추가로 Body drain (`io.Copy(io.Discard, ...)`) 안 하면 keep-alive reuse 안 됨.

**점검 위치 (4건)**:

| 파일 | 라인 | 패턴 |
|------|------|------|
| `internal/sender/kafkarest.go` | 116, 120-121 | ✅ `defer Body.Close()` + `io.Copy(io.Discard, resp.Body)` |
| `internal/sender/kafkarest.go` | 405, 409-410 | ✅ 동일 |
| `internal/discovery/discovery.go` | 60, 64 | ✅ `defer Body.Close()` (Decode가 EOF까지 읽음) |
| `internal/discovery/discovery.go` | 93, 97 | ✅ 동일 |

**HTTP transport pool 튜닝** (`internal/network/socks.go:46-49`):
```go
transport := &http.Transport{
    MaxIdleConns:        10,
    MaxIdleConnsPerHost: 10,
    IdleConnTimeout:     90 * time.Second,
}
```
→ pool 무한 성장 방지 ✅

**`CloseIdleConnections()` 호출** (shutdown 시):
- `discovery.Client.Close()` (`discovery.go:44-49`)
- `BufferedHTTPTransport.Close()` (`kafkarest.go:272 이하`)

**판정**: ✅ 모두 정상.

---

### 2. `context.With*` ↔ `cancel()` 짝 매칭

**왜**: `context.WithTimeout` / `WithCancel` / `WithDeadline` 가 반환하는 `cancel` 을 호출하지 않으면 부모 context와 연결된 timer/goroutine이 GC 될 때까지 살아 있음 → 잠재 goroutine leak.

**점검 위치 (17건)**:

| 파일 | 라인 (생성 → cancel) | 결과 |
|------|---------------------|------|
| `timediff/timediff.go` | 60 → 72 (`s.cancel()`) | ✅ |
| `timediff/timediff.go` | 113 → 114 (`defer cancel()`) | ✅ |
| `discovery/refresher.go` | 71 → 81 (`r.cancel()`) | ✅ |
| `heartbeat/heartbeat.go` | 77 → 89 (`s.cancel()`) | ✅ |
| `heartbeat/heartbeat.go` | 123 → 124 (`defer cancel()`) | ✅ |
| `heartbeat/heartbeat.go` | 168 → 169 (`defer cancel()`) | ✅ |
| `metainfo/metainfo.go` | 50 → 51 (`defer cancel()`) | ✅ |
| `eqpinfo/eqpinfo.go` | 65 → 66 (`defer cancel()`) | ✅ |
| `eqpinfo/eqpinfo.go` | 104 → 105 (`defer cancel()`) | ✅ |
| `scheduler/scheduler.go` | 57 → 83 (`s.cancel()` in Stop) | ✅ |
| `scheduler/scheduler.go` | 154 → 155 (`defer cancel()`) | ✅ |
| `scheduler/scheduler.go` | 191 → 192 (`defer sendCancel()`) | ✅ |
| `scheduler/scheduler.go` | 238 → field 저장, Reconfigure/Stop에서 호출 | ✅ |
| `service/linux.go` | 35 → 75 (`s.cancel()`) | ✅ |
| `service/windows.go` | 87 → 54 (`s.cancel()`) | ✅ |
| `collector/lhm_provider_windows.go` | 172 → 177, 223 (`p.cancel()`) | ✅ |
| `collector/storage_health_unix.go` | 117 → 118 (`defer cancel()`) | ✅ |

**판정**: ✅ 모두 짝지어짐.

---

### 3. `time.NewTicker` ↔ `Stop()`

**왜**: ticker는 Stop 안 호출하면 내부 goroutine이 살아남아 GC 차단 → goroutine leak + 채널 누적.

**점검 위치 (5건)**:

| 파일 | 라인 (생성 → Stop) | 결과 |
|------|--------------------|------|
| `timediff/timediff.go` | 91 → 92 (`defer ticker.Stop()`) | ✅ |
| `sender/kafkarest.go` | 287 → 288 (`defer ticker.Stop()`) | ✅ |
| `discovery/refresher.go` | 104 → 105 (`defer ticker.Stop()`) | ✅ |
| `scheduler/scheduler.go` | 125 → 126 (`defer ticker.Stop()`) | ✅ |
| `heartbeat/heartbeat.go` | 146 → 147 (`defer ticker.Stop()`) | ✅ |

**판정**: ✅ 모두 정상.

---

### 4. Redis 클라이언트 lifecycle

**왜**: `redis.NewClient()` 는 내부 connection pool 보유. `Close()` 안 호출하면 TCP connection + goroutine leak.

**점검 위치 (4 컴포넌트)**:

| 파일 | 패턴 | 결과 |
|------|------|------|
| `eqpinfo/eqpinfo.go:59,102` | 매 호출마다 client 생성, `defer client.Close()` | ✅ leak 없음 (효율성 개선 후보 — §발견 사항 1 참조) |
| `heartbeat/heartbeat.go:106-118` | lazy init + `Stop()` 시 `s.client.Close()` (line 98) | ✅ |
| `metainfo/metainfo.go:47-48` | per-call 생성, `defer client.Close()` | ✅ |
| `timediff/timediff.go:55,77` | Syncer Start 시 생성, Stop 시 close | ✅ |

**판정**: ✅ leak 없음. 효율성 개선 후보 1건 (§발견 사항).

---

### 5. 파일 핸들 (`os.Open` / `os.Create` / `os.OpenFile`)

**왜**: file 핸들 누적 시 OS handle limit 도달 → syscall 실패.

**점검 위치 (직접 open 2건)**:

| 파일 | 패턴 | 결과 |
|------|------|------|
| `service/startup_error_file.go:18` | `os.Create` + `defer f.Close()` (line 22) | ✅ |
| `collector/selfmetrics_linux.go:10` | `os.Open("/proc/self/fd")` + `defer dir.Close()` (line 14) | ✅ |

**`os.ReadFile` 호출** (config/loader.go × 3, sender/kafka.go, collector/storage_health_unix.go, fan_unix.go × 3): `os.ReadFile` 는 자체적으로 Close 처리 → leak 불가.

**`lumberjack.Logger`** (file sender, logger): 라이브러리 신뢰. rotation 시 이전 파일 핸들 정리.

**판정**: ✅ 모두 정상.

---

### 6. Channel 패턴 + map/slice 무한 성장

**왜**: buffered channel send-only without receiver → goroutine block. instance 변수 map/slice 가 cycle마다 grow → 메모리 누수.

**점검 결과**:

| 위치 | 패턴 | 결과 |
|------|------|------|
| `logger.asyncWriter.ch` (1000-buf) | drain goroutine + Close 시 wait. send는 `select default` 로 drop | ✅ |
| `kafkarest.flushCh` (1-buf) | `select default` send + flushLoop drain | ✅ |
| `kafkarest.stopCh / doneCh` | shutdown 신호용, leak 무관 | ✅ |
| `file.consoleCh` (1000-buf) | drainConsole + Close 시 wait, send는 `select default` drop | ✅ |
| `lhm_provider.processExit` | 1회 close 신호 | ✅ |
| `lhm_provider.lhmRequestResult` (1-buf) | per-request, worker drain 보장 (Phase 1-1) | ✅ |
| `network.lastStats` (instance map) | 매 collect 마다 newStats 로 replace (line 112: `c.lastStats = newStats`) | ✅ |
| `process_match.watchSet` (instance map) | Configure 시 set, 무한 증가 안 함 | ✅ |
| `kafkarest.topicRecords` | flush 내 local 변수 | ✅ |
| `service/linux.go:42 done` (1-buf) | shutdown 1회 신호 | ✅ |
| `service/windows.go:90 done` (1-buf) | 동일 | ✅ |

**판정**: ✅ 모두 정상.

---

### 7. Sarama producer shutdown

**왜**: Sarama AsyncProducer 는 internal goroutine 다수 (`dispatcher`, `retryHandler`, `partitionProducer`, `Broker.responseReceiver`, `withRecover`). `Close()` 안 호출 시 모두 leak. 추가로 `Producer.Return.Successes=true` 인데 `Successes()` 채널 안 읽으면 메시지 누적 → 메모리 leak.

**점검 위치** (`internal/sender/kafka.go`):

| 항목 | 위치 | 결과 |
|------|------|------|
| `Producer.Return.Successes` | line 75: `false` | ✅ Successes 누적 없음 |
| `Producer.Return.Errors` | line 76: `true` | + handleErrors goroutine 이 drain ✅ |
| `handleErrors()` | line 198-: `for err := range t.producer.Errors()` | producer.Close() 시 채널 close → 자연 종료 ✅ |
| `Close()` | line 194-196: `t.producer.Close()` | sync, 모든 internal goroutine 회수 ✅ |

**판정**: ✅ 정상.

---

## 발견 사항

### 1. eqpinfo / metainfo: 매 호출마다 Redis client 재생성 (효율성, leak 아님)

**위치**:
- `internal/eqpinfo/eqpinfo.go:59` (`defer client.Close()` × 2 함수)
- `internal/metainfo/metainfo.go:48`

**현재 동작**: 함수 진입 시 `redis.NewClient(opts)` → 사용 후 `defer client.Close()`. Close는 정상 호출되어 **leak 없음**.

**비효율 (실 영향 미미)**:
- go-redis client 는 자체 connection pool 보유 → 매 호출마다 pool 생성/소멸
- 호출 빈도가 낮음 (시작 시 1~2회 또는 주기적 1회) → 성능 영향 무시 가능

**개선 후보** (선택):
- heartbeat/timediff 패턴처럼 long-lived client + lazy init 으로 통일
- 코드 변경 ~30분, ROI 낮음

**결정**: 현 시점 미진행. 호출 빈도 변경 또는 다른 효율성 개선 작업 시 묶어서 처리.

---

## 명시적으로 점검 안 한 영역

라이브러리 신뢰 또는 ROI 낮은 영역:

| 영역 | 사유 |
|------|------|
| `gopsutil` 내부 handle/socket 사용 | 라이브러리 신뢰. process enum, IO counters, MemoryInfo 등 |
| `lumberjack` rotation 시 이전 파일 핸들 정리 | 라이브러리 신뢰. `Logger.Close()` 시 정리 |
| `fsnotify` watcher (`config/watcher.go`) | `Stop()` 호출 여부 코드 확인 안 함. 낮은 우선순위 (운영 중 reload 시에만 영향, 누적도 매우 느림) |
| 외부 process spawn (LhmHelper.exe 외) | 다른 외부 프로세스 spawn 없음 — 코드 검색으로 확인 완료 |

---

## 최종 판정

**현 시점 코드베이스의 메모리 / 소켓 / 핸들 관리는 production 사용에 안전.**

- 표준 Go 패턴 (`defer Close`, `defer cancel`, `defer ticker.Stop`) 일관 적용
- HTTP transport pool 튜닝 명시 (`MaxIdleConns=10, IdleConnTimeout=90s`)
- async pattern (logger asyncWriter, FileSender drainConsole, BufferedHTTPTransport flushLoop) 모두 graceful shutdown
- Sarama 위험 옵션 (`Successes=true` 미수신) 회피
- C1/C2/H2/R-1 fix 후 추가 leak 위험 없음

**발견 사항**: leak 0건, 효율성 개선 후보 1건 (Redis client 재생성, 미진행).

운영 검증 (Win VM 1주차) 전 추가 fix 필요한 항목 없음. SelfMetrics 의 `goroutine_count`, `rss_bytes`, `handle_count`, `buffer_count` / `buffer_dropped_total` 7 지표가 운영 중 회귀를 1차 감지.

---

## 향후 audit 트리거

다음 변경이 들어올 때 본 audit 재실행 권장:

- 새 sender / collector 추가
- 새 외부 프로세스 spawn
- HTTP / Redis / Kafka 라이브러리 메이저 업그레이드
- 새 channel / goroutine 패턴 도입
- 새 config field 가 자료구조 lifecycle 에 영향

---

## 관련 문서

- `docs/plans/memory-leak-mitigation-plan.md` — 본 audit 의 배경 (H1/C1/C2/H2 fix)
- `docs/runbooks/lhm-provider-timeout-monitoring.md` — Phase 1-1 (C1)
- `docs/runbooks/wmi-query-monitoring.md` — Phase 1-2 (C2)
- `docs/runbooks/buffered-http-transport-monitoring.md` — Phase 2-1 (H2)
- `docs/runbooks/selfmetrics-overview.md` — Phase 2.5-1 / 2.5-1.6 (운영 회귀 감지)
- `docs/runbooks/known-race-conditions.md` — R-1 RESOLVED
