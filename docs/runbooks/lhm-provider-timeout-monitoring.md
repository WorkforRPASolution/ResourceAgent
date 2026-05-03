# LhmProvider Timeout 모니터링 가이드

> **대상**: ResourceAgent 운영자 / 현장 담당자 / 개발자
> **연관 변경**: Phase 1-1 — `doRequestWithTimeout` goroutine leak 수정 (옵션 A + 옵션 B fallback 하이브리드)
> **연관 plan**: `docs/plans/memory-leak-mitigation-plan.md`
> **연관 plan 파일**: `~/.claude/plans/phase-1-1-wise-cocoa.md`

---

## TL;DR — "한 번에 봐야 할 한 줄"

```bash
grep -E "LHM_TIMEOUT|LHM_KILL|LHM_DEADLINE|LHM_IO_ERROR" log/ResourceAgent/ResourceAgent.log | tail -100
```

이 결과가:
- **거의 비어있음** → 정상. 작업 끝.
- `LHM_TIMEOUT` 가끔 + `LHM_TIMEOUT_RECOVERED` 따라옴 → **옵션 A 정상 동작**. 끝.
- `LHM_KILL_FALLBACK` 자주 발생 → **옵션 B로 회수 중**. Q2~Q3로 원인 파악.
- `LHM_DEADLINE_UNSUPPORTED` 발생 → **이 OS는 deadline 미지원**. Q2 참조.

---

## 배경 — 왜 이 로그들이 추가되었나

LhmHelper(LibreHardwareMonitor wrapper, C# 프로세스)와 stdin/stdout 파이프로 통신할 때 LhmHelper가 hang하면 ResourceAgent 측 goroutine이 영구 누수되던 문제를 수정함. 전략:

1. **옵션 A (1차 방어)**: pipe에 `SetReadDeadline` / `SetWriteDeadline` 설정 → I/O가 timeout으로 자연스럽게 회수
2. **옵션 B (2차 방어)**: 옵션 A가 silent fail하는 환경(Win7 corner case 등)을 대비, **연속 N회 timeout 시 LhmHelper 프로세스 강제 종료** → 다음 호출에서 재시작

**왜 본 log 가이드가 중요한가**: Win7 PoC 환경이 없어서 "옵션 A가 진짜 동작하는지"를 코드 단위 테스트로 사전 검증할 수 없음. 따라서 **배포 후 로그로 사후 검증**해야 함.

---

## 추가된 로그 prefix (총 6개)

전부 `[lhm-provider]` 컴포넌트에서 발생. 모든 prefix는 `LHM_` 으로 시작하므로 grep 1번에 모두 잡힘.

| Prefix | Level | 발생 조건 | 정상/비정상 | 함께 보이는 필드 |
|--------|-------|----------|:-----------:|----------------|
| `LHM_TIMEOUT` | WARN | pipe I/O가 deadline 초과 | 가끔 OK | `op`, `consecutive_timeouts`, `threshold`, `err` |
| `LHM_TIMEOUT_RECOVERED` | INFO | timeout 누적 후 첫 정상 응답 | ✅ 좋은 신호 | `prior_timeouts` |
| `LHM_KILL_FALLBACK` | ERROR | `consecutive_timeouts >= threshold` (기본 3) → Process.Kill 트리거 | 드물게 OK | `consecutive_timeouts` |
| `LHM_KILL_FAILED` | ERROR | `Process.Kill()` 자체가 실패 | ❌ 위험 | `err` |
| `LHM_DEADLINE_UNSUPPORTED` | WARN | `SetReadDeadline`/`SetWriteDeadline` 호출 자체가 에러 반환 | ⚠️ OS 지원 안 함 | `op`, `err` |
| `LHM_IO_ERROR` | WARN | timeout 외 pipe 에러 (broken pipe, EOF 등) | 재시작 트리거 | `op`, `err` |

### 로그 포맷 예시 (FixedFormatWriter, 패키지 zerolog)

```
2026-05-04 14:23:12.456 [WRN] [lhm-provider   ] LHM_TIMEOUT pipe deadline exceeded op="stdout_read" consecutive_timeouts=1 threshold=3 err="read |0: i/o timeout"
2026-05-04 14:24:42.789 [WRN] [lhm-provider   ] LHM_TIMEOUT pipe deadline exceeded op="stdout_read" consecutive_timeouts=2 threshold=3 err="read |0: i/o timeout"
2026-05-04 14:26:12.123 [WRN] [lhm-provider   ] LHM_TIMEOUT pipe deadline exceeded op="stdout_read" consecutive_timeouts=3 threshold=3 err="read |0: i/o timeout"
2026-05-04 14:26:12.124 [ERR] [lhm-provider   ] LHM_KILL_FALLBACK consecutive timeouts exceeded threshold, killing LhmHelper for restart consecutive_timeouts=3
2026-05-04 14:26:13.567 [INF] [lhm-provider   ] LhmHelper daemon started pid=4521 path="..."
2026-05-04 14:26:13.890 [INF] [lhm-provider   ] LhmHelper daemon ready sensors=12 fans=2 ...
```

---

## Q1. "옵션 A가 동작하는가?" — 정상 환경 확인

**핵심 질문**: pipe deadline이 의도대로 발화하는가?

### 명령

```bash
# Windows PowerShell (현장 PC)
Get-Content "D:\EARS\EEGAgent\log\ResourceAgent\ResourceAgent.log" | Select-String "LHM_TIMEOUT|LHM_KILL" | Select-Object -Last 50

# Linux/macOS (수집한 로그를 분석할 때)
grep -E "LHM_TIMEOUT|LHM_KILL" log/ResourceAgent/ResourceAgent.log | tail -50
```

### 판정 기준

| 결과 | 해석 | 조치 |
|------|------|------|
| **결과가 비어있음** | LhmHelper가 timeout 안에 모두 응답. 옵션 A가 발화할 일조차 없음. | 정상. 추가 작업 불필요. |
| `LHM_TIMEOUT` 1~2건 후 `LHM_TIMEOUT_RECOVERED` | 일시적 hang → 옵션 A 발화 → goroutine 회수 → 다음 요청에서 정상 복구. **이상적인 패턴**. | 정상. 옵션 A 동작 확인됨. |
| `LHM_TIMEOUT` 만 누적 (RECOVERED 없음) | timeout이 발화는 하는데 LhmHelper가 응답 자체를 못 줌. LhmHelper가 진짜 hang. | Q3 참조 (LhmHelper 자체 문제 진단). |
| `LHM_KILL_FALLBACK` 자주 발생 | 옵션 A가 동작 안 하거나 LhmHelper가 진짜 hang → 옵션 B가 회수 중. **동작은 정상이지만 비용 발생** (LhmHelper 재시작). | Q2 + Q3 참조. |

---

## Q2. "Win7에서 옵션 A가 silent fail하는가?" — Hard Gate 사후 검증

**핵심 질문**: pipe deadline 자체가 OS에서 지원되는가?

### 명령

```bash
# DEADLINE이 지원 안 되는 신호 (가장 확실한 증거)
grep "LHM_DEADLINE_UNSUPPORTED" log/ResourceAgent/ResourceAgent.log

# KILL_FALLBACK 빈도 (시간당)
grep "LHM_KILL_FALLBACK" log/ResourceAgent/ResourceAgent.log | wc -l
```

### 판정 기준

| 결과 | 해석 | 조치 |
|------|------|------|
| `LHM_DEADLINE_UNSUPPORTED` 발생 | 해당 OS는 `SetReadDeadline`/`SetWriteDeadline` 자체를 미지원. 옵션 A 무력화. **그러나 옵션 B가 정상 회수 중**. | OS/환경 정보 수집 (Win 버전, 서비스 계정 등). 장기적으론 옵션 B만 동작하는 것을 인지. |
| `LHM_DEADLINE_UNSUPPORTED` 없는데 `LHM_KILL_FALLBACK` 자주 (시간당 5회+) | **silent fail 의심**. SetXxxDeadline 자체는 에러 안 나지만 실제 발화 안 함. 또는 LhmHelper 진짜 hang. | Q3로 구분 진단. |
| `LHM_DEADLINE_UNSUPPORTED` 없고 `LHM_KILL_FALLBACK`도 거의 없음 | **옵션 A 정상 동작**. Win7도 deadline 지원. | 끝. 좋은 결과. |

### 추가 컨텍스트

ResourceAgent의 OS 정보는 별도 로그 또는 다음 명령으로:
```powershell
# Windows에서 OS 버전 확인
[System.Environment]::OSVersion
(Get-CimInstance Win32_OperatingSystem).Caption
```

---

## Q3. "LhmHelper가 진짜 hang인가, 아니면 deadline silent fail인가?"

**핵심 질문**: timeout의 원인이 LhmHelper 자체(C# 프로세스 문제)인가, ResourceAgent 측 deadline 문제인가?

### 명령

```bash
# LHM_TIMEOUT 직전/직후 LhmHelper stderr 메시지 확인
# drainStderr가 LhmHelper의 stderr를 [lhm-helper] 컴포넌트로 forward함
grep -B 3 -A 1 "LHM_TIMEOUT" log/ResourceAgent/ResourceAgent.log | grep -E "lhm-helper|LHM_TIMEOUT"
```

또는 시간순으로 같이 보기:
```bash
# 특정 시간대 ±5분
grep -E "lhm-helper|lhm-provider" log/ResourceAgent/ResourceAgent.log | grep "2026-05-04 14:2"
```

### 판정 기준

| 직전에 보이는 것 | 해석 | 조치 |
|-----------------|------|------|
| `[lhm-helper]` stderr 메시지 (.NET 예외, "Object reference null" 등) | **LhmHelper 자체 문제**. .NET runtime 이슈 / 하드웨어 센서 접근 실패 / PawnIO 드라이버 문제 등. | LhmHelper 단독 실행해서 재현: `LhmHelper.exe --daemon` 직접 실행 후 stdin에 `collect\n` 입력해서 응답 확인. |
| `[lhm-helper]` 메시지 전혀 없음 | 두 가지 가능성: ① LhmHelper 응답이 10초 timeout보다 느림 (정상이지만 느림) ② 옵션 A silent fail | 옵션 A silent fail 검증 → Q4. |
| `[lhm-helper] LhmHelper daemon stopped` | LhmHelper가 외부 요인(EPS / AV / OOM)으로 종료됨 | EPS 화이트리스트 등록 확인 (`docs/runbooks/eps-whitelist-request.md`). 시스템 이벤트 로그 확인. |

---

## Q4. "goroutine 누수가 사라졌는가?" — 핵심 검증

**현재 ResourceAgent에는 goroutine 카운트 자체 로그가 없음**. 우회 검증:

### 방법 1: Windows Task Manager (현장)

1. Task Manager 열기 → Details 탭 → 우클릭 컬럼 추가 → **"Threads"** 체크
2. `ResourceAgent.exe` 프로세스의 Threads 컬럼을 **시작 직후 vs 24시간 운영 후** 비교
3. **정상**: 변동 ±5 이내 (Go runtime 스케줄링 변동)
4. **비정상**: 시간이 갈수록 증가 (예: 시작 30 → 24h 후 200+)

> Go goroutine과 OS thread는 1:1이 아니지만, 본 leak처럼 **pipe-blocked goroutine**은 OS thread를 점유하므로 Threads 카운트가 증가함.

### 방법 2: pprof endpoint (추가 작업 필요)

향후 별도 작업으로 `net/http/pprof` debug endpoint 추가 시:
```bash
curl http://localhost:6060/debug/pprof/goroutine?debug=1 | head -100
```
`internal/poll.runtime_pollWait` 스택의 goroutine 수가 비정상으로 많은지 확인.

### 방법 3: 간접 증거 — Memory baseline

```powershell
# RSS 추이 (5분 주기 샘플링)
while ($true) {
    $p = Get-Process ResourceAgent -ErrorAction SilentlyContinue
    if ($p) { "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss'),$($p.WorkingSet64)" | Out-File -Append C:\temp\ra-memory.csv }
    Start-Sleep 300
}
```

**판정**: 24h 운영 후 RSS가 시작 직후의 1.5배 이상이면 누수 의심. 단, gopsutil의 캐시 등 다른 요인 가능 → 다른 신호와 종합.

---

## Q5. "이 변경으로 메트릭 수집 실패율이 늘었는가?" — 회귀 검증

**핵심 질문**: 옵션 A/B 도입이 정상 케이스에 부작용을 일으켰는가?

### 명령

```bash
# 메트릭 수집 실패 (initial collection failed, error in data 등)
grep -E "LhmHelper (initial collection failed|returned error|request failed)" log/ResourceAgent/ResourceAgent.log | wc -l

# 일별 분포
grep "LhmHelper request failed" log/ResourceAgent/ResourceAgent.log | awk '{print $1}' | sort | uniq -c
```

### 비교 기준

- **변경 전 1주일치 baseline 대비 +20% 이내**: 정상 노이즈
- **+50% 이상**: 회귀 의심 → 롤백 검토
- **0건 또는 극소수**: 변경 영향 없음

### 회귀 가능 시나리오

| 증상 | 가능 원인 |
|------|----------|
| 정상 환경에서 갑자기 `LHM_KILL_FALLBACK` 빈발 | `timeoutFallbackThreshold=3`이 너무 낮음. 원래 LhmHelper가 가끔 5~7초 걸리는 환경이면 이걸 timeout으로 잘못 판정. requestTimeout 늘리거나 threshold 늘리는 검토. |
| `failed to parse LhmHelper response (N bytes)` | bufio 부분 읽기 후 timeout이 끼어들어 다음 응답이 데이터 오염. 옵션 B fallback이 새 daemon으로 회수해야 하는 케이스. fallback이 발화하지 않으면 threshold 검토. |

---

## Q6. "LhmHelper 재시작 빈도는 정상인가?" — 옵션 B 비용 측정

**핵심 질문**: 옵션 B (Process.Kill)가 너무 자주 발화하면 LhmHelper 재시작 비용(시작에 1~2초 + .NET runtime 로딩)이 누적됨.

### 명령

```bash
# LhmHelper 시작 빈도 (시간당)
grep "LhmHelper daemon started" log/ResourceAgent/ResourceAgent.log | awk '{print $1, substr($2, 1, 2)}' | sort | uniq -c | tail -24
```

### 판정 기준

| 시간당 시작 횟수 | 해석 | 조치 |
|----------------|------|------|
| 0~1회 | 정상 (정상 종료 또는 일시적 회복) | 작업 끝 |
| 2~5회 | 옵션 B가 일정 부담으로 회수 중 | LhmHelper 자체 안정화 검토. EPS 화이트리스트 미등록일 가능성. |
| 5회+ | 비정상 부하. LhmHelper나 옵션 A에 큰 문제. | 즉시 진단 필요 — Q2 + Q3 + 시스템 이벤트 로그. |

---

## 알람/대시보드 권장 패턴 (장기)

ResourceAgent 메트릭 자체가 KafkaRest로 수집되므로, 향후 별도 SelfMetrics(Phase 2.5) 도입 시 다음을 alert 대상으로:

| 지표 | 임계 | 의미 |
|------|------|------|
| `LHM_KILL_FALLBACK` count (시간당) | > 3 | 옵션 A silent fail 의심 또는 LhmHelper 만성 hang |
| `LHM_KILL_FAILED` count (일별) | > 0 | Process.Kill 자체 실패 — Windows 권한/AV 간섭 의심 |
| `LhmHelper request failed` count (일별) | baseline × 5 | 회귀 또는 환경 변화 |

현 단계(Phase 1-1)에서는 이 지표를 자동 수집하는 인프라가 없음. **운영 1주차에 위 명령어로 수동 1일 1회 점검**을 권장.

---

## 운영 1주차 체크리스트 (배포 후 권장)

배포 직후 1주일간 매일 1회 다음 체크:

```bash
DATE=$(date +%Y-%m-%d)
LOG=log/ResourceAgent/ResourceAgent.log

echo "=== $DATE LhmProvider Daily Check ==="
echo "--- LHM_DEADLINE_UNSUPPORTED (한 번이라도 보이면 옵션 A 미지원 환경) ---"
grep "LHM_DEADLINE_UNSUPPORTED" $LOG | wc -l

echo "--- LHM_TIMEOUT (오늘) ---"
grep "$DATE.*LHM_TIMEOUT" $LOG | wc -l

echo "--- LHM_TIMEOUT_RECOVERED (오늘, 옵션 A 동작 신호) ---"
grep "$DATE.*LHM_TIMEOUT_RECOVERED" $LOG | wc -l

echo "--- LHM_KILL_FALLBACK (오늘, 옵션 B 회수) ---"
grep "$DATE.*LHM_KILL_FALLBACK" $LOG | wc -l

echo "--- LHM_KILL_FAILED (오늘, 위험 신호) ---"
grep "$DATE.*LHM_KILL_FAILED" $LOG | wc -l

echo "--- LhmHelper daemon started (오늘 재시작 횟수) ---"
grep "$DATE.*LhmHelper daemon started" $LOG | wc -l

echo "--- 수집 실패 (오늘) ---"
grep "$DATE.*LhmHelper request failed" $LOG | wc -l
```

### Day 7 종합 판정

위 7개 카운트를 보고:
- 모두 0~소수 → ✅ Phase 1-1 작업 정상 완료
- `LHM_DEADLINE_UNSUPPORTED` 있음 + 그 외 정상 → ✅ 옵션 B로 동작 (계획대로)
- `LHM_KILL_FALLBACK` 폭증 → ⚠️ 진단 필요 (Q2 + Q3 + Q6)
- `LHM_KILL_FAILED` 0 초과 → ❌ 위험. 즉시 분석 + 필요 시 롤백

---

## 롤백 트리거

다음 중 하나라도 발생하면 이전 버전으로 롤백 (ManagerAgent FTP를 통한 ResourceAgent.exe 재배포):

1. `LHM_KILL_FAILED` 발생 (Process.Kill 자체가 실패하는 환경 — Windows 권한 문제 가능)
2. `LhmHelper request failed` 빈도 평소 baseline × 10 이상
3. ResourceAgent.exe RSS가 24h 안에 시작 직후의 3배 이상으로 증가
4. `panic: runtime error` (lhm_provider 관련 stack)

롤백 절차는 `docs/runbooks/` 의 일반 배포 가이드 참조.

---

## FAQ

### Q. `LHM_TIMEOUT` 1~2건이 매일 보이는데 정상인가?
A. 정상. LhmHelper의 응답 시간이 가끔 10초를 넘는 케이스는 흔함 (특히 LHM이 SMART 데이터 갱신 중). `LHM_TIMEOUT_RECOVERED`가 짝지어 보이면 옵션 A가 정상 회수한 것.

### Q. `LHM_KILL_FALLBACK`이 한 번도 안 보이는 게 좋은 건가?
A. 네. 옵션 A만으로 충분히 회수되고 있다는 신호. 단, `LHM_TIMEOUT`도 함께 안 보여야 진짜 정상 (LhmHelper 자체가 안정적).

### Q. `LHM_DEADLINE_UNSUPPORTED`가 보이면 즉시 롤백해야 하나?
A. 아니요. 옵션 B fallback이 정상 회수 중이라면 동작 자체는 OK. 단, 옵션 B는 LhmHelper 재시작 비용이 있으므로 장기적으로 다른 방안(예: LhmHelper 자체 timeout 처리 강화) 검토 가치 있음.

### Q. 메트릭 수집 자체에 영향 있나?
A. 정상 케이스(99%)에는 영향 없음. timeout 발생 시 해당 1회 메트릭 수집은 실패하지만, 다음 cycle(5분 후)에는 정상 회복. LhmHelper kill되면 1~2초 지연 후 재시작.

---

## 참고 문서

- 본 작업 plan: `~/.claude/plans/phase-1-1-wise-cocoa.md`
- 메모리 누수 대응 전체 plan: `docs/plans/memory-leak-mitigation-plan.md`
- EPS 화이트리스트 협의: `docs/runbooks/eps-whitelist-request.md`
- Windows 메모리 진단 가이드: `docs/runbooks/windows-memory-leak-diagnosis.md`
- 알려진 race conditions: `docs/runbooks/known-race-conditions.md`
- 코드: `internal/collector/lhm_provider_windows.go` (특히 `doRequestWithTimeout`, `handleIOError`)
