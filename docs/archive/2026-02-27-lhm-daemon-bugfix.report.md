# Bugfix Report: LhmHelper Daemon 모드 — Kafka 데이터 미전송 + 동시성 버그

> **Summary**: LhmHelper daemon 모드 배포 후 LHM 데이터가 Kafka에 전송되지 않는 문제 조사 및 수정. 프로세스 관리 코드의 동시성 버그 7건 발견, 6건 수정.
>
> **Date**: 2026-02-27
> **Status**: Completed
> **Severity**: Critical (프로덕션 데이터 손실)

---

## 1. 증상

### 배포 환경
- Windows 10, ResourceAgent 서비스 모드
- sender_type: `kafka`
- LhmHelper daemon 모드 (`--daemon` 플래그)

### 관찰된 현상

| 항목 | 상태 | 비고 |
|------|------|------|
| daemon 시작 로그 | 정상 | `sensors=11, gpus=1, storages=2, mb_temps=1` |
| CPU, Memory, Disk, Network | 정상 전송 | LHM 미사용 collector |
| Temperature, Fan, GPU, Voltage, MB Temp, Storage | **전송 안 됨** | LHM 기반 collector 전체 |
| 에러 로그 | **없음** | 데드락으로 로그 출력 불가 |

### 핵심 단서
- daemon은 시작 시 유효한 데이터를 반환 (초기 수집 성공)
- 이후 **모든 LHM 데이터가 사라짐** — 에러 없이
- non-LHM collector는 정상 동작

---

## 2. 근본 원인 분석

### 데이터 흐름 추적

```
LhmHelper.exe --daemon
    ↓ stdin/stdout pipe
LhmProvider.GetData(ctx)          ← 여기서 데드락 발생
    ↓
각 LHM Collector (temperature, gpu, fan, ...)
    ↓
Scheduler.collect()
    ↓
KafkaSender.Send()
    ↓ WrapMetricDataJSON()
    ↓ ErrNoRows → return nil (조용히 무시)
Kafka
```

### 발견된 버그 (총 7건)

4명의 병렬 리뷰어(concurrency, dataflow, test, platform)가 코드 전체를 리뷰하여 발견.

#### CRITICAL — 프로덕션 데이터 손실 직접 원인

**Bug #1: `processErr` 채널 소비 문제 (1차 수정에서 해결)**

```go
// 수정 전: processErr chan error — 한 번만 읽을 수 있음
processErr chan error

// isProcessAlive()가 한 번 읽으면 채널이 비어서 이후 읽기가 블로킹
// stopProcess()도 같은 채널을 읽으려 해서 데드락

// 수정 후: close() 기반 시그널링 — 여러 번 읽기 가능
processExit chan struct{}
```

**Bug #2: `stopProcess()` 잠재적 데드락**

```go
// 수정 전: stderr를 processExit 대기 후에 닫음
func stopProcess() {
    p.stdin.Close()
    <-p.processExit        // cmd.Wait()가 drainStderr 완료를 기다릴 수 있음
    p.stderr.Close()       // ← 여기서 닫으면 이미 늦음
}

// 수정 후: stderr를 먼저 닫아 drainStderr 해제
func stopProcess() {
    p.stdin.Close()
    p.stderr.Close()       // ← drainStderr가 EOF 받아 종료
    <-p.processExit        // cmd.Wait() 정상 완료
}
```

**Bug #3: monitoring goroutine이 `processExit`를 참조로 캡처**

```go
// 수정 전: p.processExit를 클로저가 참조로 캡처
go func() {
    cmd.Wait()
    close(p.processExit)   // ← 재시작 후 NEW 채널을 close할 수 있음 → panic
}()

// 수정 후: 값으로 캡처
exitCh := p.processExit
go func() {
    cmd.Wait()
    close(exitCh)          // ← 항상 자신의 채널만 close
}()
```

**Bug #4: `restartWithBackoff()`가 최대 60초 mutex 보유**

```go
// 수정 전: sleep 중 mutex 보유 → Stop()이 lock 획득 불가
func restartWithBackoff() {
    // p.mu 이미 잠김 (GetData에서 호출)
    select {
    case <-time.After(wait):    // 최대 60초 대기 중 mutex 보유
    }
    p.startProcess()
}

// 수정 후: sleep 전 unlock, 후 lock + 상태 재검증
func restartWithBackoff() {
    p.mu.Unlock()               // sleep 전 해제
    select {
    case <-time.After(wait):
    case <-p.ctx.Done(): ...
    }
    p.mu.Lock()                 // sleep 후 재획득
    if !p.started { return }    // Stop()이 호출됐을 수 있음
    p.startProcess()
}
```

#### IMPORTANT — 운영/디버깅 영향

**Bug #5: `doRequestWithTimeout()` 파이프 데이터 레이스 (1차 수정에서 해결)**

```go
// 수정 전: goroutine이 p.stdin/p.stdout 직접 접근
go func() {
    p.stdin.Write(...)     // stopProcess가 p.stdin = nil 설정 가능 → nil deref
    p.stdout.ReadBytes()
}()

// 수정 후: 파이프 참조를 로컬 변수로 캡처
stdin := p.stdin
stdout := p.stdout
go func() {
    stdin.Write(...)       // 로컬 참조 사용, nil deref 방지
    stdout.ReadBytes()
}()
```

**Bug #6: `findLhmHelper()`에서 `exec.LookPath("ResourceAgent.exe")` 사용**

```go
// 수정 전: PATH에서 검색 — 서비스 모드에서 실패
exec.LookPath("ResourceAgent.exe")    // cwd가 C:\Windows\System32

// 수정 후: 실행 중인 바이너리의 절대 경로 사용
os.Executable()    // 항상 올바른 경로 반환
```

**Bug #7: `ErrNoRows` 조용한 데이터 드롭**

```go
// 수정 전: 빈 데이터를 로그 없이 무시
if errors.Is(err, ErrNoRows) {
    return nil    // 로그 없음 → 디버깅 불가
}

// 수정 후: Debug 레벨 로그 추가
if errors.Is(err, ErrNoRows) {
    log.Debug().Str("type", data.Type).Msg("Skipping empty metric data (no rows)")
    return nil
}
```

---

## 3. 프로덕션 장애 시나리오 재현

### 가장 가능성 높은 장애 시퀀스

```
1. ResourceAgent 시작
2. LhmProvider.Start() → startProcess() → 초기 수집 성공 (sensors=11)
3. 캐시 TTL(5초) 이내: GetData() → 캐시 반환 (정상)
4. 캐시 만료 후: GetData() → doRequestWithTimeout() 호출
5. [Bug #1] 만약 이전에 isProcessAlive()가 processErr를 소비했다면:
   → stopProcess()가 같은 채널 읽기에서 영원히 블로킹
   → mutex 영원히 보유
   → 모든 LHM collector가 GetData()에서 mutex 대기
   → 데이터 전송 0건
6. 에러 로그 없음 (데드락 발생 지점에 로그가 없었음)
```

### 왜 에러 로그가 없었는가

| 단계 | 로그 레벨 | 상태 |
|------|-----------|------|
| daemon 시작 | Info | 출력됨 (정상) |
| 초기 수집 | Info | 출력됨 (sensors=11) |
| 데드락 발생 | — | **로그 출력 불가** (mutex 보유 상태) |
| Scheduler | Debug | "Collection completed" (Send가 nil 반환이므로 성공으로 보임) |
| KafkaSender | — | ErrNoRows → return nil (Bug #7: 로그 없음) |

---

## 4. 수정 사항

### 수정 파일 목록

| 파일 | 변경 내용 | 버그 # |
|------|-----------|--------|
| `internal/collector/lhm_provider_windows.go` | import에 `"os"` 추가 | #6 |
| 동일 | stopProcess: stderr close 순서 변경 | #2 |
| 동일 | startProcess: exitCh 값 캡처 | #3 |
| 동일 | restartWithBackoff: mutex 해제 during sleep | #4 |
| 동일 | findLhmHelper: `os.Executable()` 사용 | #6 |
| `internal/collector/testdata/fake_daemon.go` | crash 모드: 첫 응답 후 즉시 exit | 테스트 |
| `internal/collector/lhm_provider_daemon_test.go` | sleep → processExit 채널 대기 | 테스트 |
| `internal/sender/kafka.go` | ErrNoRows Debug 로그 추가 | #7 |

### 변경하지 않은 파일

- 6개 LHM collector (temperature, fan, gpu, voltage, motherboard_temp, storage_smart) — `GetData()` 인터페이스 변경 없음
- `lhm_provider_unix.go` — Unix stub, 실질적 영향 없음
- `cmd/resourceagent/main.go` — 변경 불필요

### 수정 전후 비교

| 항목 | 수정 전 | 수정 후 |
|------|---------|---------|
| processExit 시그널링 | `chan error` (1회 읽기) | `chan struct{}` + `close()` (다중 읽기) |
| stopProcess stderr | processExit 대기 후 close | processExit 대기 전 close |
| goroutine 채널 캡처 | `p.processExit` 참조 | `exitCh` 값 캡처 |
| backoff sleep 중 mutex | 보유 (최대 60초) | 해제 → sleep → 재획득 |
| doRequestWithTimeout 파이프 | `p.stdin` 직접 접근 | 로컬 변수로 캡처 |
| LhmHelper 경로 탐색 | `exec.LookPath` (PATH 검색) | `os.Executable()` (절대 경로) |
| ErrNoRows 로깅 | 없음 | Debug 레벨 로그 |

---

## 5. 검증

### 빌드

```
GOOS=linux  go build ./...   → PASS
GOOS=windows go build ./...  → PASS
```

### 테스트

```
go test ./...  → 전체 PASS

resourceagent/internal/collector   ok
resourceagent/internal/config      ok (cached)
resourceagent/internal/discovery   ok (cached)
resourceagent/internal/eqpinfo     ok (cached)
resourceagent/internal/sender      ok
resourceagent/internal/scheduler   ok
```

### 프로덕션 검증 (TODO)

- [ ] Windows 10 서비스 모드에서 LHM 데이터 Kafka 전송 확인
- [ ] 3시간+ 지속 운영 시 temperature 수집 지속 확인
- [ ] LhmHelper.exe 수동 kill → 자동 재시작 확인
- [ ] 로그에서 "Skipping empty metric data" Debug 메시지 확인 (빈 데이터 시)

---

## 6. 교훈

### Go 동시성 패턴

1. **채널 시그널링은 `close()` 패턴 사용**: 단일 이벤트를 여러 goroutine에 알릴 때 `close(chan struct{})`가 안전. `chan error`에 값을 보내면 하나의 reader만 받음.

2. **goroutine 클로저에서 채널을 값으로 캡처**: `go func() { close(p.ch) }()`는 `p.ch`가 재할당되면 잘못된 채널을 닫음. `ch := p.ch; go func() { close(ch) }()`가 안전.

3. **mutex 보유 중 blocking I/O 금지**: sleep, 네트워크 I/O 등 오래 걸리는 작업은 mutex 해제 후 수행. 재획득 후 반드시 상태 재검증.

4. **goroutine에서 공유 필드 접근 시 로컬 캡처**: `p.stdin`, `p.stdout` 같은 필드가 다른 goroutine에서 nil로 설정될 수 있으면, 사용 전에 로컬 변수에 복사.

### 디버깅 가시성

5. **조용한 에러 경로에 로그 추가**: `return nil`로 에러를 삼키는 곳에는 최소 Debug 레벨 로그가 있어야 프로덕션 디버깅이 가능.

6. **서비스 모드에서는 `os.Executable()` 사용**: `exec.LookPath()`는 PATH 환경변수에 의존하므로, Windows 서비스(cwd=`C:\Windows\System32`)에서 실패할 수 있음.

### 리뷰 프로세스

7. **병렬 전문 리뷰어 패턴 효과적**: 동시성/데이터흐름/테스트/플랫폼 4개 관점으로 분리 리뷰하면 단일 리뷰어가 놓치는 문제를 발견할 수 있음.
