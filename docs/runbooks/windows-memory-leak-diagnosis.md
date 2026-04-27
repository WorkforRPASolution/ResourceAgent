# Windows 메모리 누수 진단 가이드

> **목적**: Windows 공장 PC에서 발생하는 Paged/Nonpaged Pool / Driver Locked 메모리 증가 원인을 ResourceAgent vs 시스템(드라이버/AhnLab/EQP SW) 으로 분리 식별하기 위한 측정 절차 + 해석 기준.
>
> **배경**:
> - 2026-04-23 현장 Win7 PC에서 1.5GB In Use 증가 관찰. 초기 가설 "LhmHelper 원인" 기각.
> - 2026-04-27 ResourceAgent **미설치** PC에서도 Paged/Nonpaged 각 1GB 사용 확인. baseline 1GB 자체가 ResourceAgent 무관 가능성 대두.
> - ResourceAgent.exe Private Working Set 변화 없음(20MB 유지) + Paged/Nonpaged 증가 → 유저 메모리가 아닌 **커널 풀 누수**가 핵심 신호.
>
> **결과 활용**: `docs/plans/memory-leak-mitigation-plan.md` v2.3.1 의 plan 우선순위 재조정 입력.

---

## 0. 시작 전 준비

### 도구 다운로드 (10분, 한 번만)

| 도구 | 다운로드 위치 | 용도 |
|------|-------------|------|
| **RAMMap** | https://learn.microsoft.com/sysinternals/downloads/rammap | 시스템 전체 메모리 카테고리 분해 |
| **Process Explorer** | https://learn.microsoft.com/sysinternals/downloads/process-explorer | 프로세스별 핸들/메모리 시계열 |
| **poolmon.exe** | Windows Driver Kit (WDK) 설치 또는 https://learn.microsoft.com/windows-hardware/drivers/devtest/poolmon | 풀 태그별 분해 |
| **pooltag.txt** | WDK 설치 시 포함 (`C:\Program Files (x86)\Windows Kits\10\Debuggers\x64\triage\pooltag.txt`) | 풀 태그 → 드라이버 매핑 |

모든 도구 **관리자 권한 필요**.

### 측정 PC 분류

| 라벨 | 정의 | 용도 |
|------|------|------|
| **PC-A** | ResourceAgent **미설치** PC | baseline 측정 (시스템 자체 누수 분리) |
| **PC-B** | ResourceAgent **설치 + 서비스 OFF** (`net stop ResourceAgent`) | 설치 자체 영향 측정 |
| **PC-C** | ResourceAgent **설치 + 서비스 ON** (운영 상태) | 실제 운영 시 영향 + 누수율 |

**3개 시나리오 비교가 핵심**. 단일 PC만 보면 baseline과 ResourceAgent 영향 분리 불가.

---

## 1. 진단 항목 전체 목록

| # | 항목 | 도구 | 측정 PC | 시간 |
|---|------|------|--------|------|
| 1 | 시스템 메모리 카테고리 baseline | RAMMap | PC-A | 5분 |
| 2 | 풀 태그별 사용량 (정적 시점) | poolmon | PC-A | 10분 |
| 3 | 풀 태그별 추세 (시간 변화) | poolmon Diff | PC-A | 30분~1시간 |
| 4 | AhnLab 드라이버 로드 여부 | driverquery, sc | PC-A | 5분 |
| 5 | 시스템 메모리 baseline (서비스 OFF) | RAMMap | PC-B | 5분 |
| 6 | ResourceAgent ON 시 시스템 영향 | RAMMap | PC-C (시계열) | 1~2시간 |
| 7 | ResourceAgent.exe 자체 메모리 | Process Explorer | PC-C | 1시간 |
| 8 | ResourceAgent.exe 핸들 누적 | Process Explorer | PC-C | 1시간 |
| 9 | Collector 분리 측정 (원인 collector 특정) | Monitor.json + RAMMap | PC-C | 각 30분 |

---

## 2. 항목별 방법

### 항목 1: 시스템 메모리 카테고리 baseline (RAMMap)

**방법**:
```
1. RAMMap.exe 실행 (관리자)
2. "Use Counts" 탭 선택
3. 화면 캡처 또는 File > Save... 로 .rmp 저장
```

**기록할 컬럼**:
- Process Private (Total)
- Mapped File
- Page Table
- **Paged Pool** ← 핵심
- **Nonpaged Pool** ← 핵심
- **Driver Locked** ← 핵심
- AWE
- Driver Locked Total

**해석 기준**:

| 카테고리 | 정상 범위 (공장 PC) | 비정상 시작점 |
|---------|------------------|-------------|
| Paged Pool | 100~600 MB | **800 MB+** |
| Nonpaged Pool | 100~400 MB | **600 MB+** |
| Driver Locked | 50~300 MB | **500 MB+** |
| Process Private | 1~3 GB | OS/SW 따라 다름 |

→ **Paged 1GB / Nonpaged 1GB는 정상 상한 근처 또는 약간 초과**. 절대값보다 "시간 추세"가 더 중요 (항목 3 참조).

---

### 항목 2: 풀 태그별 사용량 (정적, poolmon)

**방법**:
```cmd
REM 관리자 cmd
poolmon.exe -p

REM 화면 키:
REM   B = Bytes 정렬
REM   P = Paged/Nonpaged 토글
REM   E = 종료
```

**기록**: 상위 10개 태그 (Tag, Type, Bytes, Allocs, Frees, Diff).

**해석 — 알려진 태그**:

| Tag | 의미 | 정상/주의 |
|-----|------|---------|
| `MmSt`, `MmCa` | Memory Manager 자체 | 정상 (수백 MB 가능) |
| `Toke` | 보안 토큰 | 정상 |
| `File`, `FMfn` | 파일 시스템 | 정상 |
| `Thre` | 스레드 객체 | 정상 |
| `Wmi*`, `Wmip` | WMI 인프라 | **100MB+ 면 주의** (C2 후보) |
| `NDIS`, `Tcpt` | 네트워크 스택 | 200MB+ 면 주의 |
| `AhLb`, `V3*`, `ASDx` | AhnLab | 환경 따라 다름 |
| `Pwio` | PawnIO (LhmHelper) | LhmHelper 동작 시만 |
| `Nv*`, `iGdv` | 그래픽 드라이버 | 일부 누수 사례 알려짐 |

**알 수 없는 태그 추적**:
```cmd
findstr /S /M "<Tag>" C:\Windows\System32\drivers\*.sys
```
또는 `pooltag.txt` 검색.

---

### 항목 3: 풀 태그별 추세 (poolmon Diff 시계열)

**방법**:
```cmd
REM T0: 측정 시작
poolmon.exe -p
REM B 키 → 상위 10개 Bytes 캡처 (Excel)

REM 30분 후, 1시간 후 동일하게 재측정
```

**기록 표 예시**:

| Tag | T0 Bytes | T+30 Bytes | T+60 Bytes | 증가율/h |
|-----|---------|-----------|-----------|---------|
| Wmi* | 80MB | 95MB | 110MB | +30MB/h |
| Pwio | 0MB | 0MB | 0MB | 정적 |
| AhLb | 200MB | 200MB | 200MB | 정적 |

**해석**:
- **증가율 > 10MB/h** → 누수 후보. 24시간이면 240MB 누적
- **증가율 = 0** → 정상 (정적 baseline)
- **증가 후 감소** → GC/cleanup 동작 정상

**중요**: 시스템 idle 상태에서 측정 (사용자 작업 중이면 변동 큼).

---

### 항목 4: AhnLab 드라이버 로드 여부

**방법**:
```cmd
REM 4-A: 모든 드라이버 목록
driverquery /v > drivers.txt
findstr /i "ahn v3 asd ay" drivers.txt

REM 4-B: 서비스 상태 (드라이버 + 일반)
sc query type= driver | findstr /i "v3 ahn"
sc query | findstr /i "v3 ahn"

REM 4-C: 풀 태그로 직접 확인 (가장 결정적)
poolmon.exe -p
REM 화면에서 V3*, AhLb, ASDx, AhnL 등 태그 검색
```

**해석**:

| 결과 | 의미 |
|------|------|
| driverquery에 V3 드라이버 RUNNING + 풀 태그 존재 | **AhnLab 정상 동작** (UI 프로세스 없어도 보호 중) |
| driverquery에 V3 드라이버 없음 + 풀 태그 없음 | **AhnLab 미설치 또는 비활성** — baseline 1GB 원인은 다른 곳 |
| 드라이버는 있는데 STOPPED | 일시 중지 상태 (재부팅 시 다시 RUNNING 가능) |

→ baseline 1GB 가 AhnLab인지 다른 드라이버인지 분기점.

---

### 항목 5: PC-B (설치 + 서비스 OFF) baseline

**방법**:
```cmd
net stop ResourceAgent
REM 5분 안정화 대기
```
이후 **항목 1, 2 동일하게 측정**.

**해석**:
- **PC-B Paged Pool ≈ PC-A Paged Pool** → ResourceAgent 설치만으로는 차이 없음 (정상 기대)
- **PC-B Paged Pool > PC-A Paged Pool** → 거의 발생 안 함. 만약 발생하면 **설치 시 등록한 드라이버 의심** (ResourceAgent 자체는 드라이버 등록 안 함, 단 설치 스크립트가 PawnIO 설치 시 영향 가능)

---

### 항목 6: PC-C (서비스 ON) 시계열

**방법** (각 시점에 RAMMap + poolmon 캡처):
```
T0: net stop ResourceAgent → 5분 대기 → 측정
T1: net start ResourceAgent → 즉시 측정
T2: 30분 후 측정
T3: 1시간 후 측정
T4: 2시간 후 측정
T5: net stop ResourceAgent → 5분 대기 → 측정 (회복 확인)
```

**기록 표**:

| 시점 | Paged Pool | Nonpaged Pool | Driver Locked | 상위 풀 태그 변화 |
|------|-----------|--------------|--------------|----------------|
| T0 (OFF) | ___ MB | ___ MB | ___ MB | baseline |
| T1 (ON 직후) | ___ MB | ___ MB | ___ MB | 시작 비용 |
| T2 (+30분) | ___ MB | ___ MB | ___ MB | 30분 운영 후 |
| T3 (+1h) | ___ MB | ___ MB | ___ MB | 누수율 측정 |
| T4 (+2h) | ___ MB | ___ MB | ___ MB | 선형 증가 확인 |
| T5 (OFF) | ___ MB | ___ MB | ___ MB | T0로 회복? |

**해석**:
- **T5에서 T0로 회복** → 정상 (할당/해제 균형)
- **T5에서 T0로 회복 안 됨** → ResourceAgent가 누수시킨 양이 OS에 영구 잡힘 = **누수 확정**
- **T2~T4 선형 증가** → 시간당 누수율 = (T4-T2) / 1.5h
- **T1 즉시 jump** → 시작 비용 (정상 가능, 단발성)

---

### 항목 7: ResourceAgent.exe 자체 메모리 (Process Explorer)

**방법**:
```
1. Process Explorer 실행 (관리자)
2. View > Select Columns... > Process Memory 탭
3. 체크: Private Bytes, Working Set Size, Virtual Size, Peak Working Set
4. ResourceAgent.exe 우클릭 > Properties > Performance Graph 탭
   → 그래프 시계열 자동 기록
```

**기록 컬럼**:
- **Private Bytes**: Go heap + stack + 기타. 누수 시 증가.
- **Working Set**: OS가 RAM에 잡고 있는 양. 변동 큼 (정상)
- **Virtual Size**: 가상 주소 공간. 핸들 누수 시 함께 증가하기도

**해석**:
- 사용자 보고: **20MB 유지** → ResourceAgent 자체 유저 메모리 누수는 **없음**
- 그러나 Paged/Nonpaged Pool은 **커널 메모리** → Private에 안 잡힘
- 즉, **유저 메모리 정상 + 커널 풀 증가** = ResourceAgent가 호출한 OS API/드라이버 측 누수 (C2 WMI handle leak 패턴)

---

### 항목 8: ResourceAgent.exe 핸들 누적 (Process Explorer)

**방법**:
```
1. Process Explorer
2. ResourceAgent.exe 우클릭 > Properties > Performance 탭
3. "Handles" 카운트 확인
4. 1시간 시계열로 5번 캡처 (15분 간격)
5. View > Lower Pane View > Handles 로 어떤 종류의 핸들인지 분해
```

**기록 표**:

| 시점 | Handles 총수 | 증가량 |
|------|------------|-------|
| T0 (시작 직후) | ___ | - |
| T+15분 | ___ | +___ |
| T+30분 | ___ | +___ |
| T+1h | ___ | +___ |
| T+2h | ___ | +___ |

**해석**:
- **시간에 따라 핸들 단조 증가** → **핸들 누수 확정** (close 누락)
- **Lower Pane**에서 핸들 종류 보이는 패턴 → 코드 위치 추론:

| 핸들 종류 폭증 | 추정 원인 |
|--------------|---------|
| `Section` | LhmHelper 또는 mmap 의심 |
| `File` | log/metric 파일 close 누락 |
| `Event` | goroutine + sync 객체 누수 |
| `Thread` | goroutine leak (C1) |
| `Mutant`(Mutex) | sync 패턴 문제 |
| `Key` | Registry 핸들 close 누락 |

---

### 항목 9: Collector 분리 측정 (원인 특정)

**방법**: `Monitor.json` 에서 collector 하나씩 비활성화하고 항목 6/8 재측정.

**시나리오**:

| 시나리오 | Monitor.json 설정 | 목적 |
|---------|------------------|------|
| S1: 전체 ON | 모두 Enabled=true | baseline 누수율 |
| S2: StorageHealth OFF | StorageHealth.Enabled=false | **C2 (WMI handle) 검증** |
| S3: Sender만 ON | 모든 collector OFF, Sender만 동작 | M-1/M-2 (HTTP transport) 검증 |
| S4: CPU/Memory만 ON | gopsutil 기반만 | gopsutil 측 누수 검증 |
| S5: ProcessWatch OFF | ProcessWatch.Enabled=false | process API 누수 검증 |

**각 시나리오 측정**: 1시간 운영 → Paged/Nonpaged 증가량 + 풀 태그 + Handles 증가량

**해석**:
- **S2에서 누수 멈춤 + 핸들 증가율 0** → **C2 (WMI) 확정**
- **S3에서 누수 지속** → **M-1/M-2 (HTTP/Kafka) 후보**
- **S5에서 누수 멈춤** → process snapshot 관련 누수
- **모든 시나리오에서 누수 동일** → ResourceAgent 외부 원인 (다른 프로세스/드라이버)

---

## 3. 종합 의사결정 트리

```
[항목 1, 2, 4 측정] (PC-A baseline)
    │
    ├─ baseline Paged/Nonpaged 1GB + V3 드라이버/태그 없음
    │   → AhnLab 외 다른 드라이버가 원인. EQP SW / OEM 드라이버 조사
    │
    ├─ V3 드라이버 RUNNING + V3* 풀 태그 큼
    │   → AhnLab이 baseline 일부 또는 전부. 정상 (운영팀 협의)
    │
    └─ 다른 의심 태그 큼 (예: Tcpt 500MB)
        → 해당 드라이버 조사
    │
    ▼
[항목 3] 1시간 추세 측정 (PC-A)
    │
    ├─ PC-A 단독으로도 시간당 10MB+ 증가
    │   → ResourceAgent 무관, 시스템/AhnLab 자체 누수
    │   → 본 plan(코드 수정)으로 해결 안 됨. 운영팀 사안
    │
    └─ PC-A 정적 (증가 없음)
        → baseline은 정적. ResourceAgent 영향 분리 측정 의미 있음
    │
    ▼
[항목 6] PC-C 시계열 (1~2시간)
    │
    ├─ T5에서 T0로 완전 회복
    │   → ResourceAgent 누수 없음. 본 plan 우선순위 강등
    │
    ├─ T5 회복 미흡 + 풀 태그 Wmi* 증가
    │   → C2 (WMI handle leak) 확정 → 항목 9 S2로 재확인 → Step 4 진행
    │
    ├─ T5 회복 미흡 + 핸들 카운트 증가 (항목 8)
    │   → C1 (goroutine + sync 객체) 또는 다른 핸들 누수 → Step 3 진행
    │
    └─ T5 회복 미흡 + Sender만 ON에서도 누수 (S3)
        → M-1/M-2 → Step 5 진행
```

---

## 4. 측정 결과 기록 템플릿

각 PC별로 다음 표 채우기 (Excel 또는 별도 .md 파일).

### PC-A (ResourceAgent 미설치) — baseline

```
측정일: ____-__-__
PC 정보: Windows __, RAM __GB, CPU __
설치 SW: AhnLab __, EQP SW __, ...

[항목 1] RAMMap Use Counts
- Paged Pool: ___ MB
- Nonpaged Pool: ___ MB
- Driver Locked: ___ MB
- Process Private Total: ___ MB

[항목 2] poolmon 상위 10 태그
1. Tag=____  Type=___  Bytes=___MB  Diff=___
2. ...
10. ...

[항목 3] 1시간 후 변화
- Paged Pool: ___ MB → ___ MB (Δ ___ MB)
- Nonpaged Pool: ___ MB → ___ MB (Δ ___ MB)
- 증가한 태그: ____, ____

[항목 4] AhnLab 드라이버
- driverquery 결과: [있음 / 없음]
- 풀 태그 V3*/AhLb 등: [있음 / 없음]
- 풀 태그가 있다면 Bytes: ___ MB
```

### PC-B (설치 + 서비스 OFF)

```
측정일: ____-__-__
[항목 1] Paged Pool ___ MB / Nonpaged ___ MB / Driver Locked ___ MB
[항목 2] 상위 10 태그 (PC-A와 비교, 차이 강조)
[항목 4] AhnLab/PawnIO 드라이버 상태 (PC-A 와 동일/차이)
```

### PC-C (서비스 ON) — 시계열

```
[항목 6] RAMMap 시계열 (Paged / Nonpaged / Driver Locked)
T0 (OFF):     ___ / ___ / ___
T1 (ON 직후): ___ / ___ / ___
T2 (+30분):   ___ / ___ / ___
T3 (+1h):     ___ / ___ / ___
T4 (+2h):     ___ / ___ / ___
T5 (OFF, 5분): ___ / ___ / ___ ← T0로 회복?

[항목 7] ResourceAgent.exe Private Bytes
T0: ___ MB → T4: ___ MB (Δ ___)

[항목 8] ResourceAgent.exe Handles
T0: ___ → T1: ___ → T2: ___ → T3: ___ → T4: ___
종류별 (Lower Pane, T4 시점):
- File: ___
- Event: ___
- Thread: ___
- Section: ___
- Mutant: ___
- Key: ___

[항목 9] Collector 분리 측정 (각 1시간 운영 후 Paged Pool 증가량)
S1 전체 ON: +___ MB/h
S2 StorageHealth OFF: +___ MB/h ← S1과 비교
S3 Sender만 ON: +___ MB/h
S4 CPU/Memory만 ON: +___ MB/h
S5 ProcessWatch OFF: +___ MB/h
```

---

## 5. 작업 순서 (현실적 일정)

| 일자 | 작업 | 소요 |
|------|------|------|
| Day 1 (오전) | 도구 설치 + PC-A 측정 (항목 1, 2, 4) | 1시간 |
| Day 1 (오후) | PC-A 항목 3 (1시간 추세) | 대기 1h, 작업 30분 |
| Day 2 (오전) | PC-B + PC-C 항목 1, 2, 4 비교 측정 | 1시간 |
| Day 2 (오후) | PC-C 항목 6 (2시간 시계열) + 항목 7, 8 동시 | 대기 2h, 작업 30분 |
| Day 3 | 항목 9 collector 분리 측정 (S2 우선) | 각 1h × 2~3 시나리오 |
| Day 4 | 결과 종합 → 본 plan 우선순위 재조정 | 30분 |

---

## 6. 결과에 따른 plan 분기

| 측정 결과 | plan 결정 |
|----------|----------|
| ResourceAgent 기여분 < 50MB/h, baseline 정적 | plan 폐기 또는 강등 (코드 품질 개선 수준) |
| ResourceAgent 기여분 > 100MB/h, S2에서 멈춤 | **C2 (Step 4) 최우선**. Step 1 후순위 |
| baseline 자체 증가 (PC-A 추세) | plan과 별개 **운영팀 트랙** 추가 |
| ResourceAgent 핸들 단조 증가 | **C1 (Step 3) 최우선** |
| 모든 collector OFF인데 Sender만으로 누수 | **M-1/M-2 (Step 5) 최우선** |

---

## 7. 풀 태그 → 드라이버 매핑 빠른 참조

자주 만나는 풀 태그 + 의미. 알 수 없는 태그는 `findstr /S /M "<Tag>" C:\Windows\System32\drivers\*.sys` 또는 `pooltag.txt` 검색.

| Tag | 컴포넌트 | 정상/주의 |
|-----|--------|---------|
| `MmSt`, `MmCa`, `MmAc` | Memory Manager | 정상 |
| `Toke`, `TokP` | 보안 토큰 | 정상 |
| `Proc`, `Pro3` | Process 객체 | 정상 |
| `Thre`, `ThNm` | 스레드 객체 | 정상 |
| `Even`, `EvtR` | Event/ETW | 정상 |
| `File`, `Fil*`, `FMfn` | 파일 시스템 (NTFS, FltMgr) | 정상 |
| `Wmi*`, `Wmip`, `WmiE` | WMI 인프라 | **C2 후보** (100MB+ 시 의심) |
| `NDIS`, `Tcpt`, `TCPT`, `Tcpc` | 네트워크 스택 | 200MB+ 시 의심 |
| `Pwio` | PawnIO | LhmHelper 동작 시만 |
| `AhLb`, `V3*`, `ASDx`, `AhnL` | AhnLab | 환경 따라 다름 |
| `Nv*`, `iGdv`, `Amdg` | 그래픽 드라이버 (NVIDIA/Intel/AMD) | 일부 누수 사례 |
| `Disk`, `Stor`, `ATAp` | 디스크/스토리지 | 정상 |
| `Prn*`, `Splr` | Print Spooler | 누수 사례 있음 |
| `EaseFilter` 관련 | 파일 후킹 (백업/DLP SW) | 환경 따라 |

---

## 8. ResourceAgent Collector 메모리 누수 관련성 매핑

항목 9 (Collector 분리 측정) 시 어떤 collector를 어떤 누수 가설에 매핑하는지 정리. 측정 결과를 plan의 어느 Step과 연결할지 결정 입력.

### 8.1 메모리 이상 의심 항목 분류 (코드 분석 기반)

| 코드 | 위치 | 종류 | 영향 범위 |
|------|------|------|----------|
| **C1** | `lhm_provider_windows.go:451` doRequestWithTimeout | goroutine leak | LhmHelper 사용 collector 전체 |
| **C2** | `storage_health_windows.go:73` WMI query | 핸들 / Paged Pool 누수 | StorageHealth |
| **H1** | `lhm_provider_windows.go:319` drainStderr | panic 시 goroutine 종료 누락 | LhmHelper 사용 collector 전체 |
| **H2** | `cpu_process.go`, `memory_process.go`, `process_watch.go` | process snapshot 빈번 호출, gopsutil 핸들 누적 | Process 계열 3개 |
| **M-1/M-2** | `sender/kafkarest.go` BufferedHTTPTransport | flushLoop / HTTP idle conn | Sender 단 (모든 collector 간접) |

### 8.2 Collector × 누수 의심 매핑 통합 표

| # | 수집 항목 | Monitor.json 키 | 기본 Interval | LhmHelper 필요? | 데이터 소스 (Windows) | **메모리 이상 관련성** |
|---|---------|---------------|--------------|---------------|---------------------|---------------------|
| 1 | CPU 사용률 | `CPU` | 30s | ❌ | gopsutil | M-1/M-2 (Sender 간접) |
| 2 | 메모리 사용률 | `Memory` | 30s | ❌ | gopsutil | M-1/M-2 (Sender 간접) |
| 3 | 디스크 사용률 | `Disk` | 60s | ❌ | gopsutil | M-1/M-2 (Sender 간접) |
| 4 | 네트워크 트래픽 | `Network` | 30s | ❌ | gopsutil | M-1/M-2 (Sender 간접) |
| 5 | **CPU 온도** | `Temperature` | 60s | ✅ **필수** | LhmHelper (LHM) | ⚠️ **C1, H1** + M-1/M-2 |
| 6 | **프로세스별 CPU TopN** | `CPUProcess` | 60s | ❌ | gopsutil | ⚠️ **H2** + M-1/M-2 |
| 7 | **프로세스별 메모리 TopN** | `MemoryProcess` | 60s | ❌ | gopsutil | ⚠️ **H2** + M-1/M-2 |
| 8 | **팬 RPM** | `Fan` | 60s | ✅ **필수** | LhmHelper (LHM) | ⚠️ **C1, H1** + M-1/M-2 |
| 9 | **GPU 온도/사용률** | `GPU` | 60s | ✅ **필수** | LhmHelper (LHM) | ⚠️ **C1, H1** + M-1/M-2 |
| 10 | **SMART 정보** | `StorageSmart` | 300s | ✅ **필수** | LhmHelper (LHM) | ⚠️ **C1, H1** + M-1/M-2 |
| 11 | **전압 (CPU/MB)** | `Voltage` | 60s | ✅ **필수** | LhmHelper (LHM) | ⚠️ **C1, H1** + M-1/M-2 |
| 12 | **메인보드 온도** | `MotherboardTemp` | 60s | ✅ **필수** | LhmHelper (LHM) | ⚠️ **C1, H1** + M-1/M-2 |
| 13 | 시스템 가동 시간 | `Uptime` | 300s | ❌ | gopsutil | M-1/M-2 (Sender 간접) |
| 14 | **프로세스 감시** | `ProcessWatch` | 60s | ❌ | gopsutil | ⚠️ **H2** + M-1/M-2 |
| 15 | **디스크 헬스 상태** | `StorageHealth` | 300s | ❌ | **WMI** | ⚠️ **C2 (최우선 의심)** + M-1/M-2 |

### 8.3 누수 의심 우선순위별 collector 그룹

| 우선순위 | 의심 누수 | 연관 collector | Monitor.json 끄는 키 | 진단 시 확인 풀 태그 / 핸들 |
|---------|---------|--------------|------------------|--------------------------|
| **P0** (최상) | **C2 — WMI handle leak** | `StorageHealth` | `StorageHealth.Enabled=false` | `Wmi*`, `Wmip` 증가 |
| **P1** | **C1, H1 — LhmHelper 경로** | Temperature, Fan, GPU, StorageSmart, Voltage, MotherboardTemp (6개) | 6개 모두 `Enabled=false` | `Pwio` (PawnIO), Process Explorer Thread 핸들 |
| **P2** | **H2 — process snapshot** | CPUProcess, MemoryProcess, ProcessWatch (3개) | 3개 모두 `Enabled=false` | gopsutil 핸들 (File/Process), Process Explorer Handles |
| **P3** | **M-1/M-2 — Sender HTTP transport** | (collector 무관, Sender 단) | 모든 collector OFF + Sender만 동작 | `Tcpt`, idle conn 추세 |
| **무관** | (메모리 누수 의심 없음) | CPU, Memory, Disk, Network, Uptime | — | gopsutil 단순 read만 |

### 8.4 진단 시나리오별 Monitor.json 설정 (항목 9 상세)

#### S2: C2 (WMI) 검증
```json
"StorageHealth": { "Enabled": false }
```
→ `Wmi*` 풀 태그 증가 멈추면 **C2 확정** (Step 4 진행 근거)

#### LHM 경로 (C1/H1) 검증
```json
"Temperature":     { "Enabled": false },
"Fan":             { "Enabled": false },
"GPU":             { "Enabled": false },
"StorageSmart":    { "Enabled": false },
"Voltage":         { "Enabled": false },
"MotherboardTemp": { "Enabled": false }
```
→ LhmHelper.exe 자체 spawn 안 됨. `Pwio` 풀 + ResourceAgent.exe Thread 핸들 추세 변화 확인

#### S3: Sender만 ON (M-1/M-2 검증)
모든 collector `Enabled=false` 로 설정 → collector 0개. Sender의 flushLoop / HTTP idle conn만 동작. 누수 지속 시 **M-1/M-2 후보** (Step 5)

### 8.5 핵심 관찰

- **15개 중 11개**가 메모리 이상과 관련 의심 (CPU/Memory/Disk/Network/Uptime 4개만 "Sender 간접" 외 무관)
- **LhmHelper 의존 6개는 모두 C1/H1 영향권** — 단, LhmHelper가 .NET Framework 부재로 실행 안 되면 자동 무관 (현재 Win7 현장 일부 PC 케이스)
- **StorageHealth 단독으로 C2 의심** — WMI 호출은 Windows에서만 발생, Linux PC는 무관
- **Process 계열 3개의 H2**는 gopsutil 측 잠재 누수 (정량 분석 미완)

### 8.6 Win7 현장 PC (LhmHelper 미실행) 의 경우

LhmHelper 경로 전체 무효화 → P1 (C1/H1) 자동 제거. 남은 의심:
- **P0**: StorageHealth (C2) — **가장 강력한 후보**
- **P2**: Process 계열 (H2)
- **P3**: Sender (M-1/M-2)

→ baseline 1GB 의 ResourceAgent 기여분이 있다면, **C2 WMI handle leak** 또는 **gopsutil process snapshot** 또는 **Sender HTTP** 중 하나. 항목 9 의 **S2 시나리오가 1순위**.

### 8.7 32-bit Windows 빌드 시 주의

`scripts/package.sh --arch 386` 빌드는 **LhmHelper 자동 제외** (32-bit 환경 미지원). 32-bit PC는 LHM 의존 6개 collector 모두 빈 데이터 반환 → 메모리 누수 진단 시 **LhmHelper 변수 자동 제거됨** (P1 평가 불필요).

---

## 9. 참고 자료

- **본 plan 문서**: `docs/plans/memory-leak-mitigation-plan.md` (v2.3.1)
- **Win7 현장 관찰 기록**: `docs/issues/` (별도)
- **MS poolmon 가이드**: https://learn.microsoft.com/windows-hardware/drivers/devtest/poolmon
- **MS RAMMap 사용**: https://learn.microsoft.com/sysinternals/downloads/rammap
- **Process Explorer 핸들 분석**: https://learn.microsoft.com/sysinternals/downloads/process-explorer

---

## 10. 변경 이력

| 일자 | 변경 |
|------|------|
| 2026-04-27 | 초안 작성. PC-A baseline 1GB 관찰 + Private 변화 없음 + 커널 풀 증가 신호 기반. |
| 2026-04-27 | §8 추가: Collector × 누수 의심 매핑 (C1/C2/H1/H2/M-1/M-2 코드별 영향 범위), 우선순위 그룹, 시나리오별 Monitor.json 설정. |
