# WMI Query (StorageHealth) 모니터링 가이드

> **대상**: ResourceAgent 운영자 / 현장 담당자 / 개발자
> **연관 변경**: Phase 1-2 — `queryWMIDiskDrive` goroutine leak bounded fix (옵션 C — in-flight flag + stale-cache)
> **연관 plan**: `docs/plans/memory-leak-mitigation-plan.md`
> **자매 가이드**: `docs/runbooks/lhm-provider-timeout-monitoring.md` (Phase 1-1)

---

## TL;DR — "한 번에 봐야 할 한 줄"

```bash
grep "WMI_QUERY_" log/ResourceAgent/ResourceAgent.log | tail -50
```

이 결과가:
- **거의 비어있음** → 정상. 작업 끝.
- `WMI_QUERY_RECOVERED` 가끔 보임 → 일시적 hang 후 정상 회복. **이상적인 옵션 C 동작**.
- `WMI_QUERY_INFLIGHT` 누적 발생 → WMI 진짜 hang 중. 단, 추가 누수는 차단됨 (1 goroutine 한정).
- `WMI_QUERY_TIMEOUT` 빈발 → WMI 서비스 자체가 응답 안 함. 시스템 진단 필요.

---

## 배경 — 왜 옵션 C인가

`StorageHealthCollector`가 5분 cycle마다 `wmi.Query("SELECT ... FROM Win32_DiskDrive")`을 호출. WMI 서비스(wmiprvse.exe / svchost)가 hang하면 `wmi.Query`가 영원히 block됨.

### Phase 1-1과의 본질적 차이

| 항목 | Phase 1-1 (LhmHelper) | Phase 1-2 (WMI) |
|------|----------------------|-----------------|
| 통신 대상 | 우리가 spawn한 자식 프로세스 | Windows 시스템 서비스 (kill 불가) |
| 누수 회수 메커니즘 | Process.Kill → pipe close | **없음** — wmi.Query를 unblock할 방법 자체가 없음 |
| 채택 전략 | 옵션 B (Kill-on-timeout) | **옵션 C — in-flight flag + stale-cache** |
| 누수 보장 | 0 (drain으로 worker 종료 동기 대기) | **1 한정 (bounded)** — 추가 누적 차단 |

### 옵션 C 메커니즘

```
첫 호출:                    inFlight=false → CAS true → worker 생성 → wmi.Query 시작
   ↓ 응답 도착
   cache 업데이트 + inFlight=false → 정상 종료

첫 호출이 ctx timeout:      
   함수 리턴 (worker는 background에서 wmi.Query 계속 대기)
   inFlight=true 유지
   WMI_QUERY_TIMEOUT 로그

후속 호출들 (5분 cycle):    inFlight=true 감지 → 새 worker 안 만듦
   ↓
   cache 있음 → 즉시 stale 반환 (정상 데이터 흐름 유지)
   cache 없음 → 즉시 에러 반환
   WMI_QUERY_INFLIGHT 로그

결국 wmi.Query 응답:        worker가 cache 업데이트 + inFlight=false 해제
   ↓
   다음 호출은 새 worker 정상 spawn
   WMI_QUERY_RECOVERED 로그 (응답에 5초 이상 걸린 경우)

영원히 hang하는 worst case: 1 worker 영구 leak (추가 누적 없음 — bounded)
```

**핵심 보장**: 매 5분마다 1개씩 누적되던 게 → **최대 1개 한정**.

---

## 추가된 로그 prefix (총 4개)

전부 `[storage-health]` 컴포넌트에서 발생.

| Prefix | Level | 발생 조건 | 정상/비정상 | 함께 보이는 필드 |
|--------|-------|----------|:-----------:|----------------|
| `WMI_QUERY_INFLIGHT` | WARN | 이전 worker 진행 중 → stale cache (또는 에러) 반환 | bounded leak 신호 | `inflight_for` |
| `WMI_QUERY_TIMEOUT` | WARN | caller ctx timeout, worker는 background 잔류 | 일시적 OK | `err`, `inflight_for` |
| `WMI_QUERY_RECOVERED` | INFO | 5초+ 걸린 worker가 결국 응답 도착 | ✅ 좋은 신호 | `elapsed`, `query_error` |
| `WMI_QUERY_ERROR` | WARN | wmi.Query 자체 에러 (timeout 외) | 시스템 신호 | `err` |

### 로그 포맷 예시 (FixedFormatWriter)

```
2026-05-04 14:23:12.456 [WRN] [storage-health ] WMI_QUERY_TIMEOUT context expired; worker still running in background until WMI responds err="WMI query timed out: context deadline exceeded" inflight_for=30s
2026-05-04 14:28:12.567 [WRN] [storage-health ] WMI_QUERY_INFLIGHT prior WMI query still running; serving cached response to avoid goroutine accumulation inflight_for=5m1s
2026-05-04 14:33:12.678 [WRN] [storage-health ] WMI_QUERY_INFLIGHT prior WMI query still running; serving cached response to avoid goroutine accumulation inflight_for=10m1s
2026-05-04 14:35:42.123 [INF] [storage-health ] WMI_QUERY_RECOVERED long-running WMI query finally returned elapsed=12m30s query_error=false
2026-05-04 14:38:12.234 [INF] [storage-health ] (정상 데이터 수집 재개)
```

---

## Q1. "정상 동작 중인가?" — 가장 먼저 보는 것

### 명령

```bash
# Windows PowerShell
Get-Content "D:\EARS\EEGAgent\log\ResourceAgent\ResourceAgent.log" | Select-String "WMI_QUERY_" | Select-Object -Last 50

# Linux/macOS (수집 분석)
grep "WMI_QUERY_" log/ResourceAgent/ResourceAgent.log | tail -50
```

### 판정 기준

| 결과 | 해석 | 조치 |
|------|------|------|
| **결과가 비어있음** | WMI가 매번 timeout 안에 응답. 매우 정상. | 작업 끝. |
| `WMI_QUERY_TIMEOUT` 1~2건 + `WMI_QUERY_RECOVERED` 짝지어 | 일시적 hang → 옵션 C가 회수 차단 → 결국 회복. **이상적**. | 정상. |
| `WMI_QUERY_INFLIGHT` 시간당 1~2건 + 결국 `RECOVERED` 도착 | WMI가 매우 느리지만 결국 회복. | 정상이지만 WMI 부하 모니터링 권장. |
| `WMI_QUERY_INFLIGHT` 누적 (RECOVERED 안 옴) + `inflight_for` 시간 계속 증가 | WMI 영구 hang. **1 goroutine 영구 leak**. 추가 누적은 없음. | Q3 진단. |
| `WMI_QUERY_ERROR` 빈발 | WMI 서비스 자체 에러 | Q4 진단. |

---

## Q2. "1 goroutine 영구 leak이 정말 발생했나?" — bounded leak 검증

**핵심 질문**: `inflight_for` 시간이 30분 이상 누적되어도 신규 worker spawn은 안 되는가?

### 명령

```bash
# 가장 오래 걸린 inflight 이벤트
grep "WMI_QUERY_INFLIGHT" log/ResourceAgent/ResourceAgent.log | grep -oP 'inflight_for=\K[0-9hms.]+' | sort -V | tail -5

# RECOVERED 도착 여부
grep "WMI_QUERY_RECOVERED" log/ResourceAgent/ResourceAgent.log | tail -5
```

### 판정

- `inflight_for` 가 시간 따라 계속 증가 (예: 30s → 5m → 30m → 1h ...) AND `RECOVERED`가 안 옴 → WMI 영구 hang. 1 goroutine leak 확정.
  - **단, 신규 worker는 spawn 안 됨**. Task Manager의 ResourceAgent.exe Threads 수가 일정해야 정상 (Q5 참조).
- `RECOVERED` 가 결국 보임 → leak 회수됨. 정상.

### 영향 분석

1 goroutine leak의 메모리 비용: ~8KB (Go stack baseline). 운영상 무시 가능.
실질적 우려: ResourceAgent 자체가 고갈되는 게 아니라, **WMI 서비스가 hang한 시스템 자체의 문제** (Windows 재기동 또는 wmiprvse 재시작 필요).

---

## Q3. "WMI가 왜 hang하는가?" — 시스템 진단

### 가능한 원인

| 원인 | 진단 방법 | 조치 |
|------|----------|------|
| `wmiprvse.exe` 프로세스 hang | `Get-Process wmiprvse` (CPU/Memory 비정상) | `Restart-Service Winmgmt` |
| WMI repository 손상 | Event Viewer → Applications and Services → Microsoft → Windows → WMI-Activity | `winmgmt /verifyrepository`, `winmgmt /salvagerepository` |
| WMI provider (디스크) 장애 | 다른 WMI 쿼리도 실패하는지 확인 | `Get-CimInstance Win32_DiskDrive` 직접 실행 |
| 디스크 자체가 응답 안 함 (failing HDD) | Event Viewer → System → "disk" 관련 에러 | 하드웨어 점검 |
| AV/EDR 간섭 (드물) | 보안 로그 확인 | EPS 화이트리스트 등록 |

### 직접 검증 명령 (Windows)

```powershell
# 같은 쿼리를 PowerShell에서 직접 실행
Get-CimInstance Win32_DiskDrive | Select Model, Status, Size, MediaType, InterfaceType

# 응답이 즉시 오면 → ResourceAgent의 문제 (드물지만 가능)
# 응답이 안 오면 → WMI 자체 hang 확정
```

---

## Q4. "WMI_QUERY_ERROR가 빈발한다" — 에러 진단

```bash
grep "WMI_QUERY_ERROR" log/ResourceAgent/ResourceAgent.log | tail -10
```

### 가능한 원인

| 에러 메시지 패턴 | 의미 | 조치 |
|----------------|------|------|
| `Access is denied` | ResourceAgent 서비스 권한 부족 | LocalSystem 계정 확인 |
| `class Win32_DiskDrive not found` | WMI repository 손상 | `winmgmt /verifyrepository` |
| `Failed to connect to namespace` | WMI 서비스 자체 죽음 | `Restart-Service Winmgmt` |
| `unmarshal` 또는 `reflect` 관련 | gopsutil/wmi 라이브러리 호환성 | Go 의존성 버전 확인 |

---

## Q5. "ResourceAgent 자체에 누수가 있나?" — Threads 카운트

옵션 C는 **bounded** 누수 보장 — Worst case 1 goroutine.

### Windows Task Manager 검증

1. Task Manager → Details 탭 → 우클릭 컬럼 → **Threads** 체크
2. `ResourceAgent.exe`의 Threads 수를 시간대별 비교

### 정상/비정상 판정

| 시간대 | 정상 범위 | 비정상 신호 |
|--------|----------|------------|
| 시작 직후 | 20~50 | - |
| 1시간 후 | ±5 (시작 대비) | +50 이상 → leak 의심 |
| 24시간 후 | ±10 | +100 이상 → leak 확정 |
| 1주일 후 | ±15 | 시간 따라 선형 증가 → 옵션 C 우회됨 |

**옵션 C가 정상 동작 시**: 24시간 후에도 시작 직후와 비슷. 단, WMI hang 케이스에서는 +1 (영구 leak 1개).

---

## Q6. "메트릭 수집 영향" — 회귀 검증

옵션 C는 inflight 동안 **stale cache**를 반환. 즉 WMI가 hang해도 StorageHealth 메트릭은 마지막 정상 값으로 계속 송신됨.

```bash
# StorageHealth 수집 자체 실패 (Collect 함수 에러)
grep "StorageHealth.*Failed to collect" log/ResourceAgent/ResourceAgent.log | wc -l

# inflight 동안 cache 반환 신호와 함께 보기
grep -E "WMI_QUERY_INFLIGHT|StorageHealth" log/ResourceAgent/ResourceAgent.log | tail -20
```

### 판정

- inflight 동안 `StorageHealth Failed to collect`가 0건 → 옵션 C가 정상 동작 (cache로 메트릭 흐름 유지)
- inflight 동안 `Failed to collect` 발생 → 첫 hang 케이스에서 cache가 없는 상황. 정상이지만 첫 hang은 1회 메트릭 손실.

---

## 운영 1주차 체크리스트 (배포 후 권장)

```bash
DATE=$(date +%Y-%m-%d)
LOG=log/ResourceAgent/ResourceAgent.log

echo "=== $DATE WMI Query Daily Check ==="

echo "--- WMI_QUERY_TIMEOUT (오늘 ctx timeout, 일시적 OK) ---"
grep "$DATE.*WMI_QUERY_TIMEOUT" $LOG | wc -l

echo "--- WMI_QUERY_INFLIGHT (오늘 stale cache 반환, bounded leak 신호) ---"
grep "$DATE.*WMI_QUERY_INFLIGHT" $LOG | wc -l

echo "--- WMI_QUERY_RECOVERED (오늘 회복, 좋은 신호) ---"
grep "$DATE.*WMI_QUERY_RECOVERED" $LOG | wc -l

echo "--- WMI_QUERY_ERROR (오늘 WMI 에러) ---"
grep "$DATE.*WMI_QUERY_ERROR" $LOG | wc -l

echo "--- 가장 오래 걸린 inflight (오늘) ---"
grep "$DATE.*WMI_QUERY_INFLIGHT" $LOG | grep -oP 'inflight_for=\K[0-9hms.]+' | sort -V | tail -3
```

### Day 7 종합 판정

- 모두 0~소수 → ✅ Phase 1-2 작업 정상 완료
- `WMI_QUERY_TIMEOUT` 일별 0~3건 + 모두 `RECOVERED`로 이어짐 → ✅ 정상 (옵션 C로 회복)
- `WMI_QUERY_INFLIGHT` 누적 (RECOVERED 안 옴) → ⚠️ WMI 영구 hang. 1 goroutine leak 발생 중. **시스템 차원 진단** (Q3).
- ResourceAgent.exe Threads 수가 시간 따라 선형 증가 → ❌ 옵션 C 우회됨. 즉시 분석.

---

## 알람/대시보드 권장 패턴 (장기)

| 지표 | 임계 | 의미 |
|------|------|------|
| `WMI_QUERY_INFLIGHT` 의 `inflight_for` | > 1h | WMI 영구 hang 의심 → 시스템 점검 필요 |
| `WMI_QUERY_ERROR` count (일별) | > 5 | WMI 서비스 자체 문제 |
| ResourceAgent.exe Threads 추이 | 24h 내 +50 | 옵션 C 우회 의심 — 즉시 분석 |

---

## 롤백 트리거

다음 중 하나라도 발생하면 이전 버전으로 롤백:

1. ResourceAgent.exe Threads 수가 24h 내 50+ 증가 (옵션 C 우회 의심)
2. StorageHealth 수집 실패가 평소 baseline × 10 이상
3. `panic: runtime error` (storage_health 관련 stack)
4. `WMI_QUERY_RECOVERED` 가 한 번도 안 보이는데 `WMI_QUERY_INFLIGHT` 가 매일 누적 (cache가 깨졌을 가능성)

---

## FAQ

### Q. WMI hang 시 메트릭 수집은?
A. inflight 동안 stale cache 반환 → StorageHealth는 마지막 정상 값으로 계속 송신. 첫 hang 시점에는 cache가 없어 1회 빈 데이터. 그 이후 hang 회복 전까지 동일 데이터 반복.

### Q. 옵션 C가 우회되는 시나리오?
A. 이론적으로 거의 없음. atomic CAS로 spawn 차단. 단, 패키지 변수가 mock으로 swap되거나 `resetWMIQueryStateForTest`가 production에서 호출되면 우회됨 (둘 다 테스트 전용).

### Q. WMI 서비스 재시작 후에는?
A. 재시작 후 첫 호출에서 `wmi.Query`가 즉시 응답 → 새 worker가 cache 업데이트 + `WMI_QUERY_RECOVERED` 로그 + `inFlight=false` 해제 → 다음 호출부터 정상.

### Q. 영구 leaked goroutine이 신경 쓰인다.
A. ~8KB stack 비용 + 1개 thread (locked to syscall). ResourceAgent 자체 운영에 영향 없음. 실제 우려는 **WMI 서비스가 영구 hang한 시스템 자체** — Windows 재시작 외 해결책 없음. ResourceAgent를 재시작해도 leaked goroutine은 사라지지만 (프로세스 종료) 다시 시작 후 같은 hang에 빠질 가능성 동일.

---

## 참고 문서

- 메모리 누수 대응 전체 plan: `docs/plans/memory-leak-mitigation-plan.md`
- Phase 1-1 LhmProvider 가이드: `docs/runbooks/lhm-provider-timeout-monitoring.md`
- EPS 화이트리스트 협의: `docs/runbooks/eps-whitelist-request.md`
- Windows 메모리 진단 가이드: `docs/runbooks/windows-memory-leak-diagnosis.md`
- 코드: `internal/collector/storage_health_windows.go` (특히 `queryWMIDiskDrive`, `wmiQueryStateData`)
