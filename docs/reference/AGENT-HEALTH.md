# AgentHealth — Watchdog 기반 Heartbeat

## 개요

`AgentHealth`는 ResourceAgent의 **application-level liveness**를 감지하기 위한 Redis 기반 heartbeat 메커니즘이다.

기존 `AgentRunning` 키는 순수 숫자(uptime 초)만 저장하여 "프로세스가 살아있는가"만 판단할 수 있었다. `AgentHealth`는 여기에 **상태(Status)**와 **사유(Reason)**를 추가하여 다음 질문에 답한다:

| 질문 | AgentRunning | AgentHealth |
|------|:---:|:---:|
| 프로세스가 살아있는가? | O | O |
| 메트릭 수집이 정상인가? | X | O |
| 정상 종료인가, 비정상 중단인가? | X | O |
| 프로세스 가동 시간은? | O | O |

## 아키텍처

### 컴포넌트 구성도

```
┌─────────────────────────────────────────────────────────────────┐
│                        ResourceAgent                            │
│                                                                 │
│  ┌──────────┐    collect() 성공 시     ┌──────────────────────┐  │
│  │Scheduler │─── lastActivityMs ────►│ atomic.Int64          │  │
│  │          │    .Store(now)          │ (UnixMilli)           │  │
│  │ collect()│                        └──────────┬───────────┘  │
│  │  ├ Collect()                                 │              │
│  │  ├ Send()  ←── 둘 다 성공해야 갱신            │              │
│  │  └ Store() ←── lastActivityMs               │              │
│  └──────────┘                                   │              │
│                                                 │ LastActivity()│
│                                                 ▼              │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Heartbeat Sender                                         │  │
│  │                                                          │  │
│  │  healthCheck func() (status, reason)                     │  │
│  │    │                                                     │  │
│  │    ├─ nil              → "OK", ""                        │  │
│  │    ├─ LastActivity=0   → "OK", ""  (첫 수집 전)          │  │
│  │    ├─ Since > 90s      → "WARN", "no_collection"         │  │
│  │    └─ 그 외            → "OK", ""                        │  │
│  │                                                          │  │
│  │  10초마다 ──► SETEX AgentHealth:{key} {value} 30         │  │
│  └──────────────────────────────────────────────────────────┘  │
│                          │                                      │
└──────────────────────────┼──────────────────────────────────────┘
                           │ SETEX (TTL 30s)
                           ▼
                    ┌─────────────┐
                    │    Redis    │
                    │             │
                    │ AgentHealth │
                    │ :ARS-M1-E1 │
                    │ = OK:3600   │
                    └─────────────┘
```

### 시작 순서 (Phase별)

`main.go`의 `run()` 함수에서 Phase별로 순차 초기화된다:

| Phase | 컴포넌트 | 설명 |
|-------|---------|------|
| **1** | Infrastructure | Redis EQP_INFO, ServiceDiscovery, TimeDiff |
| **1.5** | **Heartbeat Start** | `heartbeat.NewSender()` + `hb.Start(ctx)` |
| 2 | LHM Provider | Windows 온도 수집 데몬 |
| 3 | Collectors | Registry 구성 및 Configure |
| 4 | Sender | Kafka/KafkaRest/File |
| 4.5 | Address Refresher | ServiceDiscovery 주기 갱신 |
| **5** | **Scheduler Start** | `sched.Start(ctx)` — 메트릭 수집 시작 |
| **5 직후** | **SetHealthCheck()** | Scheduler → Heartbeat watchdog 연결 |
| 6 | Watchers | Monitor.json/Logging.json Hot Reload |

#### 지연 주입(Deferred Injection) 패턴

Heartbeat는 Phase 1.5에서 시작되지만, Scheduler는 Phase 5에서야 시작된다. 이 시차 동안 `healthCheck`는 `nil`이므로 항상 `OK`를 보고한다.

Phase 5 직후 `SetHealthCheck()`로 실제 watchdog 함수를 주입한다:

```go
// Phase 1.5: Heartbeat Start (healthCheck=nil → 항상 OK)
hb = heartbeat.NewSender(redisAddr, cfg.Redis, infra.dialFunc,
    cfg.EqpInfo.Process, cfg.EqpInfo.EqpModel, cfg.EqpInfo.EqpID)
hb.Start(ctx)

// ... Phase 2~5 ...

// Phase 5 직후: SetHealthCheck() 지연 주입
hb.SetHealthCheck(func() (string, string) {
    last := sched.LastActivity()
    if last.IsZero() {
        return "OK", "" // 아직 첫 수집 전
    }
    if time.Since(last) > heartbeat.StalenessThreshold {
        return "WARN", "no_collection"
    }
    return "OK", ""
})
```

이 패턴 덕분에:
- Heartbeat는 Scheduler 없이도 즉시 `OK` heartbeat를 보낼 수 있다 (Phase 1.5~5 구간)
- Scheduler가 준비된 후에야 실제 수집 상태를 감시한다
- `SetHealthCheck()`는 `sync.RWMutex`로 보호되어 thread-safe하다

## Watchdog 메커니즘

### 상태 판단 흐름

```
healthCheck 호출
│
├─ healthCheck == nil ?
│   └─ YES → return "OK", ""
│
├─ sched.LastActivity().IsZero() ?
│   └─ YES → return "OK", ""       ← 첫 수집 전 (Scheduler 막 시작)
│
├─ time.Since(lastActivity) > 90s ?
│   └─ YES → return "WARN", "no_collection"  ← 수집 지연/실패
│
└─ return "OK", ""                  ← 정상
```

### lastActivity 갱신 조건

`Scheduler.collect()` 내부에서 `lastActivityMs`는 **Collect와 Send 모두 성공한 경우에만** 갱신된다:

```go
func (s *Scheduler) collect(ctx context.Context, c collector.Collector) {
    data, err := c.Collect(collectCtx)
    if err != nil {
        return  // ← Collect 실패: lastActivityMs 갱신 안 됨
    }
    if data == nil {
        return  // ← nil 데이터: lastActivityMs 갱신 안 됨
    }

    if err := s.sender.Send(sendCtx, data); err != nil {
        return  // ← Send 실패: lastActivityMs 갱신 안 됨
    }

    s.lastActivityMs.Store(time.Now().UnixMilli())  // ← 여기서만 갱신
}
```

이 설계 덕분에:
- Kafka/KafkaRest 연결 장애 → Send 실패 → `lastActivity` 정체 → 90초 후 `WARN:no_collection`
- Collector 내부 오류 → Collect 실패 → 동일하게 staleness 감지
- `atomic.Int64`를 사용하여 lock-free로 다중 collector goroutine에서 안전하게 갱신

### LastActivity() 메서드

```go
func (s *Scheduler) LastActivity() time.Time {
    ms := s.lastActivityMs.Load()
    if ms == 0 {
        return time.Time{}  // zero Time → IsZero() == true
    }
    return time.UnixMilli(ms)
}
```

- `lastActivityMs`는 `atomic.Int64` 타입 (zero value = 0)
- 한 번도 수집 성공하지 않았으면 `time.Time{}` (zero) 반환
- Heartbeat의 healthCheck에서 `last.IsZero()` 체크로 첫 수집 전 상태 판별

## Redis 키 형식

| 항목 | 값 |
|------|-----|
| 키 | `AgentHealth:{Process}-{EqpModel}-{EqpID}` |
| 값 형식 | `{Status}:{UptimeSeconds}` 또는 `{Status}:{UptimeSeconds}:{Reason}` |
| TTL | 30초 |
| 갱신 주기 | 10초 |

### 키 생성

```go
func BuildKey(process, eqpModel, eqpID string) string {
    return fmt.Sprintf("%s:%s-%s-%s", HealthKeyPrefix, process, eqpModel, eqpID)
}
// 예: AgentHealth:ARS-MODEL1-EQP001
```

`process`, `eqpModel`, `eqpID`는 Redis EQP_INFO에서 조회한 값으로, `main.go`에서 다음과 같이 전달된다:

```go
hb = heartbeat.NewSender(redisAddr, cfg.Redis, infra.dialFunc,
    cfg.EqpInfo.Process, cfg.EqpInfo.EqpModel, cfg.EqpInfo.EqpID)
```

### 값 예시

| 상태 | 값 | 의미 |
|------|-----|------|
| 정상 | `OK:3600` | 프로세스 정상, uptime 1시간 |
| 경고 | `WARN:3600:no_collection` | 프로세스 alive지만 90초 이상 수집 실패 |
| 종료 | `SHUTDOWN:3600` | 정상 종료 직후 (TTL 30초 내) |
| 키 없음 | - | 오래 전 종료 또는 비정상 중단 |

### 값 생성 로직

```go
// reason이 비어있으면 2-part, 있으면 3-part
if reason != "" {
    value = fmt.Sprintf("%s:%d:%s", status, uptimeSeconds, reason)
} else {
    value = fmt.Sprintf("%s:%d", status, uptimeSeconds)
}
```

`uptimeSeconds`는 `time.Since(s.startTime).Seconds()`로 계산되며, `startTime`은 `NewSender()` 호출 시점의 `time.Now()`이다.

## 상수

| 상수 | 값 | 근거 |
|------|-----|------|
| `DefaultInterval` | 10s | heartbeat 전송 주기 |
| `DefaultTTL` | 30s | 3회 연속 miss 후 키 자동 만료 (10s x 3) |
| `StalenessThreshold` | 90s | collector 간격(10s) x 3 + collect timeout(30s) x 2 |

### StalenessThreshold 산출 근거

```
기본 collector 간격:   10s
3회 연속 실패 여유:     × 3 = 30s
collect timeout:       30s (context.WithTimeout)
send timeout:          10s (context.WithTimeout)
safety margin:         ~20s
합계:                  ~90s
```

90초 동안 어떤 collector도 Collect+Send에 성공하지 못하면, 시스템에 실질적인 문제가 있다고 판단한다.

## SHUTDOWN 처리

### 종료 시퀀스

```
ctx.Done() (shutdown signal)
    │
    ▼
sched.Stop()         ← 모든 collector goroutine 종료 대기
    │
    ▼
hb.Stop()            ← defer로 호출됨
    ├─ s.cancel()    ← ticker goroutine 중지
    ├─ s.wg.Wait()   ← goroutine 종료 대기
    └─ s.sendShutdown()  ← SHUTDOWN heartbeat 전송
```

### sendShutdown() 구현 특징

```go
func (s *Sender) sendShutdown() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    client, err := createRedisClient(s.redisAddr, s.redisCfg, s.dialFunc)
    if err != nil {
        log.Warn().Err(err).Msg("failed to create Redis client for shutdown heartbeat")
        return  // best-effort
    }
    defer client.Close()

    uptimeSeconds := int64(time.Since(s.startTime).Seconds())
    value := fmt.Sprintf("SHUTDOWN:%d", uptimeSeconds)

    if err := client.SetEx(ctx, s.key, value, s.ttl).Err(); err != nil {
        log.Debug().Err(err).Str("key", s.key).Msg("shutdown heartbeat SETEX failed")
        return  // best-effort
    }
}
```

핵심 사항:
- **`context.Background()` 사용**: `hb.Stop()` 시점에서 원래 `ctx`는 이미 cancel된 상태이므로, 새로운 background context를 사용한다
- **5초 timeout**: SHUTDOWN 전송에 최대 5초까지 대기
- **best-effort**: Redis 연결 실패 시 에러를 로깅하고 무시한다. 종료 과정을 차단하지 않는다
- **TTL 30초**: SHUTDOWN 후 30초간 키가 유지되어, WebManager가 "정상 종료" 상태를 확인할 수 있다. 이후 자동 만료

### 상태 전이 다이어그램

```
[프로세스 시작] ──► OK:N ──(10s 주기)──► OK:N+10
                     │
                     ├──(수집 90s 실패)──► WARN:N:no_collection
                     │
                     └──(정상 종료)──► SHUTDOWN:N ──(30s TTL)──► [키 만료]

[비정상 중단] ──► (마지막 OK/WARN 유지) ──(30s TTL)──► [키 만료]
```

## 테스트

### 단위 테스트

```bash
go test ./internal/heartbeat/... -v
```

miniredis를 사용한 단위 테스트 목록:

| 테스트 | 검증 내용 |
|--------|---------|
| `TestBuildKey` | 키 형식 `AgentHealth:ARS-M1-EQP01` |
| `TestSender_SendOnce` | 단발 전송 시 `OK:N` 형식, TTL 30초 |
| `TestSender_UptimeIncreases` | 시간 경과에 따라 uptime 증가 |
| `TestSender_Stop` | Stop 후 `SHUTDOWN:N` 기록 |
| `TestSender_SetHealthCheck_WARN` | healthCheck가 WARN 반환 시 `WARN:N:no_collection` |
| `TestSender_SetHealthCheck_OK` | healthCheck가 OK 반환 시 `OK:N` (reason 없음) |
| `TestSender_NilHealthCheck` | healthCheck nil일 때 `OK:N` |
| `TestSender_RedisDown` | Redis 미연결 시 panic 없이 Start/Stop 정상 동작 |

### E2E 테스트 (실제 Redis)

```bash
# 실제 Redis 인스턴스 대상
REDIS_E2E=localhost:6379 go test ./internal/heartbeat/... -v -run TestE2E

# 전체 E2E 스크립트
./scripts/e2e_test.sh
```

E2E 테스트(`TestE2E_RealRedis`)는 실제 Redis에 대해 다음을 검증한다:
- 키 prefix가 `AgentHealth:`인지 확인
- Start 후 `OK:N` (N >= 1) 기록 확인
- TTL이 (0, 30s] 범위인지 확인
- Stop 후 `SHUTDOWN:N` 전환 확인

`REDIS_E2E` 환경 변수가 없으면 자동으로 skip된다.

## WebManager 연동

WebManager는 클라이언트 상태 조회 시 다음 순서로 Redis를 조회한다:

1. **`AgentHealth:{Process}-{EqpModel}-{EqpID}`** 키 우선 조회
2. 키가 없으면 **`AgentRunning` 계열 키**로 fallback (기존 순수 숫자 형식)

프론트엔드의 `parseAliveValue()` 함수가 두 형식 모두 파싱한다:
- `"3600"` → 순수 숫자 (AgentRunning 레거시)
- `"OK:3600"` → AgentHealth 정상
- `"WARN:3600:no_collection"` → AgentHealth 경고
- `"SHUTDOWN:3600"` → AgentHealth 정상 종료
