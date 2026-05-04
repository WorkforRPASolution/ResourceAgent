# LhmProvider Timeout 모니터링 가이드

> **대상**: ResourceAgent 운영자 / 현장 담당자 / 개발자
> **연관 변경**: Phase 1-1 — `doRequestWithTimeout` goroutine leak 수정 (옵션 B 단독 — Kill-on-timeout)
> **연관 plan**: `docs/plans/memory-leak-mitigation-plan.md` (v2.4.2 노트 참조)
> **연관 plan 파일**: `~/.claude/plans/phase-1-1-wise-cocoa.md`

---

## TL;DR — "한 번에 봐야 할 한 줄"

```bash
grep -E "LHM_TIMEOUT_KILL|LHM_KILL_FAILED|LHM_DRAIN_TIMEOUT|LHM_IO_ERROR|LHM_CTX_CANCELLED" log/ResourceAgent/ResourceAgent.log | tail -100
```

이 결과가:
- **거의 비어있음** → 정상. 작업 끝.
- `LHM_TIMEOUT_KILL` 가끔 발생 + 직후 `LhmHelper daemon started` → **옵션 B 정상 회수 중**. 끝.
- `LHM_KILL_FAILED` 또는 `LHM_DRAIN_TIMEOUT` 발생 → **위험 신호**. Q3, Q4 참조.
- `LHM_TIMEOUT_KILL` 빈발 (시간당 5회+) → LhmHelper 자체 문제 의심. Q5 참조.

---

## 배경 — 왜 옵션 B 단독인가

당초 plan은 **옵션 A (pipe deadline) + 옵션 B (Process.Kill) 하이브리드** 였으나, Win10/11 환경 검증에서 다음을 확인:

```
SetReadDeadline returned error: file type does not support deadline
SetWriteDeadline returned error: file type does not support deadline
```

→ **Go runtime이 Windows anonymous pipe (`os.Pipe()`)에 deadline 지원을 명시적으로 거부**. Win7만 의심한 게 아니라 **모든 Windows 버전에 해당**. 따라서 옵션 A는 silent fail이 아니라 **명시적 에러로 거부됨**, 우리 코드가 동작 자체를 못 함.

**옵션 B 단독으로 전환** (커밋 X에서):
1. `doRequestWithTimeout`이 worker goroutine을 spawn해서 `stdin.Write` + `stdout.ReadBytes` 수행
2. main goroutine은 `select { case result, case time.After(timeout) }`
3. timeout 시 `cmd.Process.Kill()` → pipe가 죽어서 worker의 I/O가 에러로 회귀 → worker 종료
4. main goroutine이 worker 종료를 동기 대기 (`<-ch`, 2초 hard cap) → **누수 0 보장**

비용: timeout 발생 시마다 LhmHelper 재시작 (~1초). 정상 환경(99%+)에서는 발생 안 함.

---

## 추가된 로그 prefix (총 5개)

전부 `[lhm-provider]` 컴포넌트에서 발생. 모든 prefix는 `LHM_` 으로 시작하므로 grep 1번에 모두 잡힘.

| Prefix | Level | 발생 조건 | 정상/비정상 | 함께 보이는 필드 |
|--------|-------|----------|:-----------:|----------------|
| `LHM_TIMEOUT_KILL` | ERROR | request가 `requestTimeout` (기본 10초) 초과 → Process.Kill 트리거 | 가끔 OK | `timeout`, `consecutive_timeouts` |
| `LHM_TIMEOUT_RECOVERED` | INFO | timeout 누적 후 정상 응답 도착 | ✅ 좋은 신호 | `prior_timeouts` |
| `LHM_KILL_FAILED` | ERROR | `Process.Kill()` 자체가 실패 | ❌ 위험 | `err`, `reason` |
| `LHM_DRAIN_TIMEOUT` | ERROR | Kill 후에도 worker goroutine이 2초 안에 unwind 안 됨 | ❌ 위험 (1 goroutine 누수) | `reason` |
| `LHM_IO_ERROR` | WARN | timeout 외 pipe 에러 (broken pipe, EOF, JSON parse 실패 등) | 재시작 트리거 | `err` |
| `LHM_CTX_CANCELLED` | WARN | caller context 취소 → Kill 트리거 | 정상 종료 시 발생 | `err` |

### 로그 포맷 예시 (FixedFormatWriter, 패키지 zerolog)

```
2026-05-04 14:23:12.456 [ERR] [lhm-provider   ] LHM_TIMEOUT_KILL request timed out; killing LhmHelper to release blocked goroutine timeout=10s consecutive_timeouts=1
2026-05-04 14:23:12.567 [INF] [lhm-provider   ] LhmHelper daemon stopped pid=4521
2026-05-04 14:23:13.789 [INF] [lhm-provider   ] LhmHelper daemon started pid=4567 path="..."
2026-05-04 14:23:13.890 [INF] [lhm-provider   ] LhmHelper daemon ready sensors=12 fans=2 ...
2026-05-04 14:28:13.123 [INF] [lhm-provider   ] LHM_TIMEOUT_RECOVERED daemon responded after prior timeout streak prior_timeouts=1
```

---

## Q1. "정상 동작 중인가?" — 가장 먼저 보는 것

**핵심 질문**: timeout 자체가 거의 안 발생하는가?

### 명령

```bash
# Windows PowerShell (현장 PC)
Get-Content "D:\EARS\EEGAgent\log\ResourceAgent\ResourceAgent.log" | Select-String "LHM_" | Select-Object -Last 50

# Linux/macOS (수집 분석)
grep "LHM_" log/ResourceAgent/ResourceAgent.log | tail -50
```

### 판정 기준

| 결과 | 해석 | 조치 |
|------|------|------|
| **결과가 비어있음** | LhmHelper가 timeout 안에 모두 응답. 매우 정상. | 작업 끝. |
| `LHM_TIMEOUT_KILL` 1~2건 + 후속 `LhmHelper daemon started` | 일시적 hang → Kill로 회수 → 정상 재시작. **이상적인 옵션 B 동작**. | 정상. 추가 작업 불필요. |
| `LHM_TIMEOUT_RECOVERED` 출현 | timeout 후 정상 응답. 매우 좋은 신호. | 정상. |
| `LHM_TIMEOUT_KILL` 빈발 (시간당 5회+) | LhmHelper가 자주 hang. 옵션 B는 정상 회수 중이지만 비용 누적. | Q5 참조 (LhmHelper 자체 문제 진단). |
| `LHM_KILL_FAILED` 또는 `LHM_DRAIN_TIMEOUT` | **위험**. Process.Kill 자체가 동작 안 하거나 OS가 pipe I/O를 unblock 안 함. | Q3/Q4 즉시 진단. |

---

## Q2. "Kill로 정말 회수가 되었는가?" — 옵션 B 검증

**핵심 질문**: Kill 후 LhmHelper가 정상 종료 + 재시작되었는가?

### 명령

```bash
# 한 번의 timeout cycle을 시간순으로 본다
grep "$(date +%Y-%m-%d).*\(LHM_TIMEOUT_KILL\|LhmHelper daemon (stopped\|started\|ready)\)" log/ResourceAgent/ResourceAgent.log | tail -20
```

### 판정 — 정상 패턴

```
14:23:12.456 [ERR] LHM_TIMEOUT_KILL ... timeout=10s
14:23:12.567 [INF] LhmHelper daemon stopped pid=4521        ← Kill 직후
14:23:13.789 [INF] LhmHelper daemon started pid=4567        ← 재시작 (1~2초)
14:23:13.890 [INF] LhmHelper daemon ready sensors=12 ...    ← 초기 데이터 OK
```

### 판정 — 비정상 패턴

```
14:23:12.456 [ERR] LHM_TIMEOUT_KILL ...
14:23:14.456 [ERR] LHM_DRAIN_TIMEOUT worker goroutine did not unwind within 2s   ← 위험!
```

→ Process.Kill이 발화는 했지만 worker의 I/O가 unblock 안 됨. 1 goroutine 영구 누수. Windows 권한 문제 또는 AV 간섭 가능성.

---

## Q3. "Process.Kill 자체가 실패하는 환경?" — 권한/AV 문제

**핵심 질문**: `LHM_KILL_FAILED`가 보이는가?

### 명령

```bash
grep "LHM_KILL_FAILED" log/ResourceAgent/ResourceAgent.log
```

### 가능한 원인 + 조치

| 원인 | 신호 | 조치 |
|------|------|------|
| ResourceAgent 서비스 권한 부족 | `Access is denied` 같은 에러 | LocalSystem 계정으로 실행되는지 확인 (`sc qc ResourceAgent`) |
| AV/EDR가 Process.Kill 차단 | `The system cannot terminate the process` | EPS 화이트리스트 등록 (`docs/runbooks/eps-whitelist-request.md`) |
| LhmHelper가 보호 모드 (PROTECTED_LIGHT 등) | 매우 드묾 | LhmHelper 빌드 옵션 확인 |

`LHM_KILL_FAILED`가 한 번이라도 발생하면 **롤백 트리거**. 즉시 분석.

---

## Q4. "Drain timeout — Kill해도 worker가 안 죽는다?"

**핵심 질문**: `LHM_DRAIN_TIMEOUT`이 보이는가?

이론적으로 Process.Kill 후 자식 프로세스의 모든 handle이 close되면 부모측 pipe read가 EOF로 회귀해야 함. 그게 2초 안에 안 일어나면:
- AV/EDR가 pipe handle을 잡고 있음 (드물지만 가능)
- Windows pipe에 미해결 IRP가 stuck (커널 버그 — 매우 드묾)

### 영향 + 조치

- 1회 발생 = 1 goroutine 영구 누수 (다음 LhmHelper 재시작까지)
- 시간당 5회 이상 = LhmHelper 매번 재시작되니 누적은 제한적이지만 RSS 증가 가능
- 발생 시 **Sysinternals Handle.exe로 ResourceAgent 핸들 추적** 권장:
  ```powershell
  handle.exe -p ResourceAgent.exe -a
  ```

---

## Q5. "LhmHelper가 자주 hang한다" — 자체 문제 진단

**핵심 질문**: `LHM_TIMEOUT_KILL` 시간당 5회 이상 빈발 + `LHM_TIMEOUT_RECOVERED`도 안 보임

### 가능한 원인

| 원인 | 진단 신호 | 조치 |
|------|----------|------|
| .NET Framework 4.7 미설치/손상 | startProcess 시 `pipe is being closed` 또는 `broken pipe` | `reg query "HKLM\SOFTWARE\Microsoft\NET Framework Setup\NDP\v4\Full" /v Release` (≥460798 필요) |
| PawnIO 드라이버 문제 | LhmHelper stderr에 driver 관련 에러 | LhmHelper.exe 단독 실행해서 재현 |
| 하드웨어 센서 응답 지연 (특정 메인보드) | 항상 같은 sensor enumeration에서 timeout | Monitor.json에서 해당 collector 비활성화 검토 |
| EPS / V3 등 AV 간섭 | LhmHelper 프로세스가 의심 SW로 분류됨 | 화이트리스트 등록 |

### 검증 명령

```bash
# LhmHelper stderr (drainStderr가 forward) 확인
grep "lhm-helper" log/ResourceAgent/ResourceAgent.log | tail -30

# LhmHelper 시작 빈도 (시간당)
grep "LhmHelper daemon started" log/ResourceAgent/ResourceAgent.log | awk '{print $1, substr($2, 1, 2)}' | sort | uniq -c | tail -24
```

판정:
- 시간당 0~1회 시작: 정상
- 시간당 2~5회: 옵션 B가 일정 부담으로 회수 중. 운영 가능하나 LhmHelper 안정화 권장.
- 시간당 5회+: 비정상 부하. 즉시 진단.

---

## Q6. "goroutine 누수가 사라졌는가?" — 핵심 검증

옵션 B 단독에서는 timeout마다 worker가 Kill로 회수되므로 **이론적으로 누수 0**. 단, `LHM_DRAIN_TIMEOUT`이 발생하지 않는 한 보장됨.

### 우회 검증 (현장 환경)

#### 방법 1: Windows Task Manager
1. Task Manager → Details 탭 → 우클릭 컬럼 → "Threads" 체크
2. `ResourceAgent.exe`의 Threads 컬럼을 시작 직후 vs 24h 후 비교
3. **정상**: ±5 이내 (Go scheduler 변동)
4. **비정상**: 시간 따라 증가 (시작 30 → 24h 후 200+)

> Go goroutine과 OS thread는 1:1이 아니지만 pipe-blocked goroutine은 OS thread를 점유 → Threads 카운트가 증가.

#### 방법 2: pprof endpoint (선택사항, 별도 작업)

debug 빌드에 `net/http/pprof` 추가 시:
```bash
curl http://localhost:6060/debug/pprof/goroutine?debug=1 | head -100
```
`internal/poll.runtime_pollWait` 또는 `syscall.SyscallN` 스택의 goroutine 수가 비정상으로 많은지.

---

## Q7. "이 변경으로 메트릭 수집 실패율이 늘었는가?" — 회귀 검증

```bash
grep -E "LhmHelper (initial collection failed|returned error|request failed)" log/ResourceAgent/ResourceAgent.log | wc -l

# 일별 분포
grep "LhmHelper request failed" log/ResourceAgent/ResourceAgent.log | awk '{print $1}' | sort | uniq -c
```

### 비교 기준

- 변경 전 1주일치 baseline 대비 +20% 이내: 정상 노이즈
- +50% 이상: 회귀 의심 → 롤백 검토
- 0건 또는 극소수: 변경 영향 없음

---

## 운영 1주차 체크리스트 (배포 후 권장)

배포 직후 1주일간 매일 1회:

```bash
DATE=$(date +%Y-%m-%d)
LOG=log/ResourceAgent/ResourceAgent.log

echo "=== $DATE LhmProvider Daily Check ==="

echo "--- LHM_TIMEOUT_KILL (오늘 timeout으로 인한 강제 종료) ---"
grep "$DATE.*LHM_TIMEOUT_KILL" $LOG | wc -l

echo "--- LHM_TIMEOUT_RECOVERED (오늘 timeout 후 정상 복구 확인) ---"
grep "$DATE.*LHM_TIMEOUT_RECOVERED" $LOG | wc -l

echo "--- LHM_KILL_FAILED (오늘 위험 신호) ---"
grep "$DATE.*LHM_KILL_FAILED" $LOG | wc -l

echo "--- LHM_DRAIN_TIMEOUT (오늘 위험 신호 — goroutine 누수) ---"
grep "$DATE.*LHM_DRAIN_TIMEOUT" $LOG | wc -l

echo "--- LHM_IO_ERROR (오늘 pipe I/O 에러) ---"
grep "$DATE.*LHM_IO_ERROR" $LOG | wc -l

echo "--- LhmHelper daemon started (오늘 재시작 횟수) ---"
grep "$DATE.*LhmHelper daemon started" $LOG | wc -l

echo "--- 수집 실패 (오늘) ---"
grep "$DATE.*LhmHelper request failed" $LOG | wc -l
```

### Day 7 종합 판정

- 모두 0~소수 → ✅ Phase 1-1 작업 정상 완료
- `LHM_TIMEOUT_KILL` 일별 0~3건, `LHM_KILL_FAILED`/`LHM_DRAIN_TIMEOUT` 모두 0 → ✅ 정상 (옵션 B로 회수 중)
- `LHM_KILL_FAILED` 0 초과 또는 `LHM_DRAIN_TIMEOUT` 0 초과 → ❌ 즉시 분석 + 필요 시 롤백
- `LhmHelper daemon started` 시간당 5회+ → ⚠️ Q5 (LhmHelper 자체 문제) 진단

---

## 알람/대시보드 권장 패턴 (장기)

| 지표 | 임계 | 의미 |
|------|------|------|
| `LHM_TIMEOUT_KILL` count (시간당) | > 5 | LhmHelper 만성 hang 의심 → Q5 진단 |
| `LHM_KILL_FAILED` count (일별) | > 0 | 즉시 알람 — 권한/AV 간섭 의심 |
| `LHM_DRAIN_TIMEOUT` count (일별) | > 0 | 즉시 알람 — goroutine 누수 발생 중 |

현 단계(Phase 1-1)에서는 이 지표를 자동 수집하는 인프라 없음. **운영 1주차에 위 명령으로 수동 1일 1회 점검** 권장.

---

## 롤백 트리거

다음 중 하나라도 발생하면 이전 버전으로 롤백:

1. `LHM_KILL_FAILED` 1회라도 발생
2. `LHM_DRAIN_TIMEOUT` 1회라도 발생
3. `LhmHelper request failed` 빈도 평소 baseline × 10 이상
4. ResourceAgent.exe RSS가 24h 안에 시작 직후의 3배 이상 증가
5. `panic: runtime error` (lhm_provider 관련 stack)

롤백 절차는 `docs/runbooks/`의 일반 배포 가이드 참조.

---

## FAQ

### Q. `LHM_TIMEOUT_KILL`이 매일 1~2건 보이는데 정상인가?
A. 정상. LhmHelper는 가끔 SMART 데이터 갱신이나 센서 enumeration에 10초+ 걸리는 경우가 있음. Kill로 회수되고 다음 cycle에 정상 복구.

### Q. timeout 시 LhmHelper 재시작 비용이 부담스럽지 않나?
A. 재시작은 ~1초 (LhmHelper 프로세스 + .NET Framework 로딩 + LibreHardwareMonitor 초기화). 정상 환경에서 timeout은 거의 발생 안 하므로 평소 부담 없음. timeout 빈발 시에는 Q5 진단으로 근본 원인 해결.

### Q. 옵션 A 미지원이 향후 Go 버전 업데이트로 해결될 가능성?
A. Go 표준 라이브러리는 Windows anonymous pipe IOCP 통합을 명시적으로 거부 (`file type does not support deadline`). 해결되려면 named pipe로 마이그레이션 또는 Go runtime 패치 필요. 본 작업 시점(2026-05)까지 Go의 known limitation. 변경되면 plan 재검토.

### Q. 메트릭 수집 자체에 영향 있나?
A. 정상 케이스(99%)에는 영향 없음. timeout 발생 시 해당 1회 메트릭 수집은 실패하지만, 다음 cycle (5분 후)에는 정상 회복. LhmHelper kill되면 1~2초 지연 후 재시작.

---

## 참고 문서

- 본 작업 plan: `~/.claude/plans/phase-1-1-wise-cocoa.md`
- 메모리 누수 대응 전체 plan: `docs/plans/memory-leak-mitigation-plan.md` (v2.4.2)
- EPS 화이트리스트 협의: `docs/runbooks/eps-whitelist-request.md`
- Windows 메모리 진단 가이드: `docs/runbooks/windows-memory-leak-diagnosis.md`
- 알려진 race conditions: `docs/runbooks/known-race-conditions.md`
- 코드: `internal/collector/lhm_provider_windows.go` (특히 `doRequestWithTimeout`, `killAndDrain`)
