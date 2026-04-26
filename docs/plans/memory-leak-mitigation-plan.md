# ResourceAgent 버그 수정 & 메모리 누수 대응 계획 (v2.3.1)

> **변경 이력**:
> - v1 → v2 (2026-04-22): 전문가 1차 리뷰 반영 — Appendix A
> - v2 → v2.1 (2026-04-22): 전문가 2차 교차 검증 반영 — Appendix B
> - v2.1 → v2.2 (2026-04-22): 전문가 3차 교차 검증 반영 — Appendix C
> - 작업량 추정 — Appendix D
> - 2026-04-24: 전문가 4차 최종 검증 — Appendix E (12건 실행 중 해결 TODO)
> - v2.2 → v2.3 (2026-04-26): Codex 5차 교차 검증 반영 — Appendix G
>   - **핵심 원칙 재확립**: 증상 감추기(배포 설정 disable) 제거. 코드 버그는 근본 수정, 외부 요인(AV)은 진단 절차 + 사용자 대응 매뉴얼(Appendix F)로 분리.
>   - Phase 0 축소: 배포 Monitor.json 영구 변경 제거, drainStderr H1 fix만 유지
>   - Phase 1-2 재정의: C2(WMI goroutine leak)를 **코드 수정**으로 (수집기 유지)
>   - Phase A 매트릭스 확장: LHM-all-off, StorageSmart-only-off **진단용 일시 실험** 추가 (끝나면 원복)
>   - M-1 atomic race 제거: enforcement는 mu critical section 내부, atomic은 SelfMetrics 관측용으로만
>   - M-2 MaxBufferedRecords config 경로 확장 (loader.go, validate.go, 샘플 JSON 포함)
>   - Appendix F 신설: AV 원인 확정 시 사용자 대응 매뉴얼 (예외 등록 / 대체 API / 수집기 삭제 기준)
> - **v2.3 → v2.3.1 (2026-04-26): GitHub Actions 미사용 환경 반영 — Appendix G.5 갱신**
>   - W0-4 재정의: GitHub Actions 조직 승인 → "사내 CI 인프라 조사 (없으면 로컬 스크립트 + CSV 이력 운영)"
>   - Phase 4-0 변경: `.github/workflows/ci.yml` 매트릭스 → 로컬 `scripts/ci.sh` + `scripts/ci.ps1` + `docs/runbooks/ci-history.csv` (자동 append)
>   - PR 본문 결과 첨부 정책 폐지 → CSV 이력 통합 추적
>   - git pre-push hook은 도입하지 않음 (`--no-verify` 우회 + setup 마찰 + 정책 중복)
>   - **Coverage baseline 실측 (Day 1)**: Overall 58.5%, internal/collector 46.4% (function avg, macOS 측정, Windows-only 제외)
>   - **Step 0 race detector 발견**: R-1 (FileSender.drainConsole vs 테스트 stdout swap) — `docs/runbooks/known-race-conditions.md` 별도 PR로 fix

---

## Context

### 현장 관찰 사실 (확정)

공장 현장 Windows 7 PC 여러 대에서 ResourceAgent 설치 이후 시스템 메모리가 약 800MB 증가하는 현상이 관찰됨:

- ResourceAgent.exe **프로세스 Private Working Set은 증가 없음**
- RAMMap 기준 **Paged Pool 약 1.05GB + Nonpaged Pool 약 500MB** (평시 baseline 미확정, Phase A-1 확보)
- Resource Monitor의 **Modified 영역은 증가 없음**
- **ResourceAgent 서비스 중지 및 `sc delete` 후에도 메모리 유지**
- **여러 동종 PC에서 재현** → ResourceAgent 설치와 상관관계

### 가설 및 불확실성

**1순위 가설 (미확정)**: ResourceAgent의 반복 시스템 API 호출(raw device open, WMI 쿼리, 프로세스 enumeration)이 시스템 상주 커널 필터/추적 드라이버의 커널 객체 할당을 유발, ResourceAgent 수명과 무관하게 드라이버 메모리에 축적 → 재부팅 전까지 해소 안 됨.

**경합 가설 (Phase A 배제 대상)**:
- AhnLab V3 **미니필터(FltMgr) + Ob callback 중심** 커널 컴포넌트 (v2.2 정정: 리버스 분석 커뮤니티 보고 기준 Win7 x64에서 IRP/Ob callback이 주경로, SSDT 후킹은 20~30% 확률)
- AhnLab의 **IRP 후킹 드라이버** (`AhnMDrv.sys`, `AhnFlt.sys` 등 `IRP_MJ_DEVICE_CONTROL` 후킹)
- Windows Defender (Win7 Security Essentials) 실시간 스캔
- Superfetch(SysMain) / Prefetcher 자연 메모리 보유
- 자산관리(SCCM/`ccmexec`) / DLP / 백업 에이전트의 커널 필터
- ETW 세션 consumer paged buffer
- Win7 x64 정상 Nonpaged Pool 범위 (동적 상한)

**이전 버전 정정 (Appendix A, B, C 참조)**:
- WMI 쿼리 → 커널 Pool 누적 주장 부정확 (WmiPrvSE user-mode)
- `\\.\C:` 볼륨 open → AV 미니필터 전체 트리거 과장 (`FO_VOLUME_OPEN` early-return 일반적)
- IOCTL_STORAGE vs WMI AV 트리거 비교 근거 부족
- Phase 1-1 bufio.Reader race 오진 (Lock 직렬화로 방지)
- **AhnLab V3 실제 주경로 (v2.2)**: SSDT 후킹이 아닌 **IRP 후킹 + 커널 콜백 + 미니필터 조합**. v2.1이 SSDT 시나리오에 과도 의존했던 부분 조정.
- **RAMMap `-Rt` 옵션 허구** — `rammap.exe -o snapshot.rmp` 덤프로 대체

### 병렬 발견 코드 버그

- **Critical-C1**: `lhm_provider_windows.go:434-481` — `doRequestWithTimeout` goroutine leak
- **Critical-C2**: `storage_health_windows.go:66-85` — WMI 타임아웃 goroutine leak (30s bounded)
- **High-H1**: `lhm_provider_windows.go:396-403` — `drainStderr` 에러/panic 처리 부재
- **High-H2**: `kafkarest.go:176-191` — `BufferedHTTPTransport` 버퍼 상한 부재

### 목표

1. Week 0 + Phase A로 원인 가설 확정/배제 + 인프라 실사
2. 코드 레벨 버그 C1/C2/H1/H2 **근본 수정** (수집기 기능 유지)
3. noisy API 호출 경로 대체 (AV 트리거 감소)
4. 배포·관측·롤백 인프라 확보 (10,000대 규모 안전 롤아웃)
5. SLO 기반 배포 종결 판단
6. **(v2.3 신설) AV 원인 확정 시 사용자 대응 매뉴얼 제공** (Appendix F) — 코드로 못 고치는 외부 요인은 운영 절차로 분리

### 핵심 원칙 (v2.3 명시)

**증상 감추기 금지**:
- "수집기 off → 메모리 안 늘어남" 으로 끝내지 않는다. 그게 원인이면 **코드를 고치거나(코드 버그), 기능을 삭제하거나(불필요한 수집기), 외부 요인을 문서화한다(AV 등)**.
- 배포 Monitor.json의 영구 disable은 채택하지 않는다. Phase A에서의 일시 off는 **진단 실험**일 뿐, 끝나면 원복한다.
- 현장에서 안 보이게 한다고 문제가 사라지는 게 아님 — 재발/확산 위험 + 진단 능력 영구 손실.

---

## 서비스 수준 목표 (SLO)

| SLO | 지표 | 임계 | 측정 | 평가 기간 |
|-----|------|------|------|----------|
| **SLO-1 커널 Pool 증가** | Paged Pool 72h 증가량 | ≤ baseline × **[Phase A 확보 후 0.3~0.7 범위 확정, 잠정 0.5]** | **외부 typeperf/RAMMap snapshot = 판정** + SelfMetrics `paged_pool_bytes` 고빈도 보조 | 72h × 카나리 |
| **SLO-2 프로세스 안정성** | RSS / Goroutine / 외부 ping | RSS < 60MB, goroutine drift < ±10 (상대 drift 기반), ping 성공률 ≥ 99% | SelfMetrics + 외부 heartbeat (Dead-man's switch) | **45일 연속** |
| **SLO-3 파이프라인 건강성** | 성공률 / 드롭률 / 롤백 건수 | 성공률 ≥ 99.5%, 드롭 < 1/hour/PC, 월 롤백 < 사이트당 0.1% | SelfMetrics + ManagerAgent 명령 로그 | 45일 |

**SLO-2 "상대 drift" 주의 (v2.2 정정)**: `runtime.NumGoroutine()` 절대값은 goroutine 생성/소멸 변동으로 flaky → **자신의 baseline(프로세스 시작 후 60초 평균) 대비 상대값** 으로 변경. QA 전문가 지적 반영.

**SLO-1 source of truth 이중화 (v2.3 신설)**:
- **판정 (source of truth)**: 외부 도구 측정 — `typeperf "\Memory\Pool Paged Bytes"` (10s 주기 파일 수집) + RAMMap snapshot (대표 시점 3회). 이 값이 SLO 판정 기준.
- **고빈도 보조**: SelfMetrics `paged_pool_bytes` (1분 주기, agent hang/dead 시 공백 발생). 추세 모니터링 용도.
- **이중화 이유**: agent rollback/dead 상태에서도 외부 측정으로 carry-over baseline 유지. agent 자체의 self-report만 믿으면 hang 시 관측 공백.

---

## 계획 개요 (Phase 지도)

```
Week 0   (선결, 1주)         : 인프라 실사 Spike — Alerting/워크플로우 엔진 + RACI 확정
Phase A   (병렬, 사용자 수행) : 현장 진단
Phase 0   (즉시)             : 설정 기본값 변경 + drainStderr fix
Phase 1   (단기)             : Critical C1 수정
Phase 2   (단기)             : BufferedHTTPTransport 상한
Phase 2.5 (단기)             : 관측성 & 배포 인프라 + Auto-rollback 회로 차단기
Phase 3   (중기)             : AV Trigger 감소 리팩토링
Phase 4   (검증)             : goleak + CI + 통합 테스트
Phase 5   (배포)             : SLO 기반 카나리 → 점진 확대
```

**Week 0는 Phase A와 병행 개시** — 인프라 존재 여부 확정 없이는 Phase 2.5-3(자동 트리거 Chain)이 공중누각.

---

## 진행 현황 대시보드

### Week 0 — 인프라 실사 (v2.2 신설)
- [ ] W0-1. Alerting 시스템 실사 (Prometheus Alertmanager / Grafana Alerts / 기타)
- [ ] W0-2. ManagerAgent 워크플로우 엔진 존재 확인
- [ ] W0-3. RACI 실명 확정 (SRE on-call / 개발 리드 / 보안팀 / 사이트 관리자)
- [ ] W0-4. 사내 CI 인프라 조사 (Jenkins/GitLab CI/기타). 부재 시 로컬 스크립트 + CSV 이력 운영 (v2.3.1: GitHub Actions 사용 불가 확정)
- [ ] W0-5. FTP 스토리지 용량 산정 (N-1 artifact × 90일 × 사이트 수)

### Phase A — 현장 진단
- [ ] A-0. 대체 가설 전수 조사 (fltmc/driverquery/logman/SSDT 진단)
- [ ] A-1. 깨끗한 baseline (RAMMap `-o` + `/accepteula`)
- [ ] A-2. With-ResourceAgent 측정
- [ ] A-3. Pool 분석
- [ ] A-4. 선택적 비활성화 실험 (병렬 5대)
- [ ] A-5. AV 예외 등록 실험
- [ ] A-6. 보조 확증
- [ ] Phase A 산출물

### Phase 0 — 저위험 코드 수정 (v2.3 축소)
- [ ] 0-1. drainStderr 에러 핸들링 + panic 복구 (H1)
- ~~0-0. Downstream 영향 사전 조사~~ → **삭제** (배포 설정 영구 변경 안 함)
- ~~0-1. Disk / StorageHealth 기본값 disabled~~ → **삭제** (증상 감추기, v2.3 원칙 위배)

### Phase 1 — Critical Goroutine Leak
- [ ] 1-1-spike. bufio.Reader 해체 + stdin SetWriteDeadline 설계 + PoC
- [ ] 1-1. LhmProvider doRequestWithTimeout 수정 (C1)
- [ ] 1-2. WMI Query goroutine leak **코드 수정** (C2, v2.3: 수집기 유지 + hang 방지)

### Phase 2 — 견고성 개선
- [ ] 2-1. BufferedHTTPTransport 버퍼 상한 + atomic 카운터 구현 계약
- [ ] 2-2. Scheduler 30s timeout 주석 보강

### Phase 2.5 — 관측성 & 배포 인프라
- [ ] 2.5-0. ManagerAgent 계약 Kickoff (Week 0 RACI 기반)
- [ ] 2.5-1. SelfMetrics + Dead-man's switch + runtimeStats seam
- [ ] 2.5-2. Feature flag / kill-switch
- [ ] 2.5-3. 롤백 Runbook + 자동 트리거 Chain + 회로 차단기
- [ ] 2.5-4. ManagerAgent 원격 롤백 명령 구현
- [ ] 2.5-5. 로그/메트릭 중앙 수집 경로

### Phase 3 — AV Trigger 감소 리팩토링
- [ ] 3-0. 대체 접근 검토 (ETW consumer API / PerfLib v2 / Job Object)
- [ ] 3-1. NtQuerySystemInformation 기반 enumeration (Go 구현 6건 주의)
- [ ] 3-2. ProcessSnapshot + singleflight + (PID, CreateTime) tuple + FILETIME 정규화
- [ ] 3-3. 세 collector 전환 (gopsutil seam 감사 선행)
- [ ] 3-4. (conditional) Disk PDH 기반 구현
- [ ] 3-5. (conditional, 기술 제약) StorageHealth 대안

### Phase 4 — 테스트 & 검증
- [ ] 4-0. 선결: goleak + CI + clock + coverage baseline (Day 1 실측치 기록) + mockable seam 감사
- [ ] 4-1. 단위 테스트 전면 통과
- [ ] 4-2. goleak 기반 leak 회귀 방지
- [ ] 4-3. 플랫폼별 통합 테스트
- [ ] 4-4. 벤치마크 (옵션 B 결과별 의사결정 트리 포함)
- [ ] 4-5. Golden test (mockable seam 기반)
- [ ] 4-6. fake_daemon 하네스 단위 테스트 (1-1 선결)

### Phase 5 — 배포 & 모니터링
- [ ] 5-1. 카나리 (자동 롤백 Chain + 회로 차단기 연결)
- [ ] 5-2. 단계적 확장
- [ ] 5-3. 모니터링 지표 운영
- [ ] 5-4. SLO 달성 (45일)
- [ ] 5-5. 장기 운영 정책

---

## Week 0 — 인프라 실사 Spike (v2.2 신설)

**목표**: Phase 2.5-3 자동 트리거 Chain과 Phase 4-0 CI의 **전제 인프라 존재 여부를 Phase 본 착수 전 확정**. 없으면 해당 Phase가 공중누각이 되어 Phase 5 수개월 추가 지연 가능.

**기간**: 1주, Phase A와 병행 개시.

### W0-1. Alerting 시스템 실사

- [ ] 사내 기존 Alerting 인프라 조사:
  - Prometheus Alertmanager 있는가?
  - Grafana Alerts 활성화?
  - 자체 alerting 시스템 (예: 기존 Kafka consumer 기반 경고)?
- [ ] 없으면 Phase 2.5-3 범위 확대 재산정
- [ ] Kafka consumer 측 heartbeat-miss 감지 로직을 기존 인프라 어디에 구현할지 결정

### W0-2. ManagerAgent 워크플로우 엔진 존재 확인

- [ ] ManagerAgent 팀에 기존 워크플로우 엔진(자동 롤백/hold 명령 체인 실행) 보유 여부 질의
- [ ] 없으면 Phase 2.5-4 스코프에 엔진 자체 구현 추가 필요
- [ ] 대체재: 간단한 cron 기반 체크 + `sc.exe stop/delete` 스크립트 조합

### W0-3. RACI 실명 확정

- [ ] **Responsible (SRE on-call 1차)**: 실명 [TBD → W0 종료 시 확정]
- [ ] **Accountable (개발 리드)**: 실명 [TBD → W0 종료 시 확정]
- [ ] **Consulted**:
  - AV/보안팀 담당자 실명
  - 사이트 관리자 대표 실명
- [ ] **Informed**: 운영팀 전체 (메일링 리스트 확정)
- [ ] On-call 15분 응답 정책의 현실성 확인 (주말/야간 대응 가능 여부)

### W0-4. 사내 CI 인프라 조사 (v2.3.1 재정의)

**v2.3.1 변경 사유**: GitHub Actions 사용 불가 확정. 대안 인프라 조사 + 부재 시 로컬 스크립트 운영.

- [ ] 사내 Jenkins / GitLab CI / TeamCity / 기타 CI 인프라 존재 여부 확인
- [ ] 존재 시: ResourceAgent 빌드/테스트 통합 가능 여부 + 신청 절차 + 리드타임
- [ ] 부재 시: 로컬 검증 스크립트 + CSV 이력 운영 정책으로 대체 (Step 0 PR에서 도입 완료)
  - `scripts/ci.sh` + `scripts/ci.ps1`
  - `docs/runbooks/ci-history.csv` (자동 append)
  - `docs/runbooks/ci-history-policy.md`
- [ ] CI 인프라 발견 시 ci.sh를 entrypoint로 통합 (CSV 형식 유지)

### W0-5. FTP 스토리지 용량 산정

- [ ] N-1 artifact 보관: ResourceAgent.exe 약 20MB × 90일 (보관 기간) × 사이트 수 (예: 50개) = **90GB** 수준
- [ ] Monitor.json 백업도 포함: +1GB
- [ ] 기존 FTP 스토리지 여유 공간 확인, 부족 시 증설 요청

**Week 0 산출물**: 인프라 실사 리포트 (`docs/runbooks/infrastructure-audit.md`). 이 문서가 Phase 2.5-3 설계 조정의 근거가 됨.

---

## Phase A — 현장 진단 (병렬, 사용자 수행)

**목표**: 1순위 가설 확정/배제 + baseline 확보.

**표본**: 3사업장 × 2라인 × 3PC = 18대 stratified. AhnLab 버전, Win7 SP1/SP2, 32/64bit, RAM, 동시 에이전트 메타데이터 기록. `experimental` 태그로 격리.

### A-0. 대체 가설 전수 조사

```cmd
REM 1. 미니필터
fltmc instances

REM 2. 드라이버
driverquery /v /fo csv > drivers.csv

REM 3. ETW 세션
logman query -ets

REM 4. AV/DLP/백업 서비스
sc.exe query type= service state= all | findstr /i "ahnlab v3 defender mcafee symantec trellix ccmexec veeam acronis"

REM 5. SSDT 후킹 탐지 (v2.2 신설)
REM 주의: GMER/PCHunter는 별도 도구 필요. Win10 관리자 PC에서 원격 분석.
REM 또는 WinDbg 로컬: kd -kl
REM   !chkimg -d nt   (SSDT hooked entries)
REM   !analyze -v
```

**판정 기준 (v2.2 재조정)**:

- `fltmc instances` **Altitude 컬럼 해석**:
  - 280000~289999: Anti-Virus FSFilter
  - 320000~329999: Activity Monitor  
  - 360000~389999: Replication
  - Windows 기본 미니필터 화이트리스트: `FileInfo`, `Wof`, `MsSecFlt` 등
  - AhnLab 관련: `V3*`, `Ahn*`, `ALYac*` 이름 포함이면 active 여부 확인

- **Altitude 값 해석**: Frame 컬럼 값이 0이면 기본 Filter Manager 계층

- **v2.2 정량 임계 (통계 기반)**:
  - baseline 사이트 평균 대비 AV 미니필터 수가 **+2σ** 이상 → 커널 필터 누적 가설 가중치 상향
  - ETW non-MS 세션 수 평균 +2σ 이상 → ETW consumer 누적 가설
  - 절대 임계 (변별력 약해서 보조 용도): AV 3개+, ETW non-MS 5개+

- **`logman query -ets` non-MS 세션 필터**:
  - 화이트리스트: `EventLog-*`, `Circular Kernel Context Logger`, `NT Kernel Logger`, `DiagTrack-Listener`, `Microsoft-Windows-*`
  - 나머지 중 ResourceAgent 관련 이벤트 구독 세션 확인

- **AhnLab V3 실제 후킹 메커니즘 참고 (v2.2)**:
  - Win7 x64에서 V3는 주로 **FltMgr 미니필터 + `PsSetCreateProcessNotifyRoutineEx` + `ObRegisterCallbacks`** 사용
  - `AhnMDrv.sys`, `AhnFlt.sys` 등은 **IRP_MJ_DEVICE_CONTROL 후킹** 가능
  - SSDT 후킹은 PatchGuard 회피 어려워 주경로 아님 (20~30% 확률)
  - 따라서 Phase A 진단은 **IRP 후킹/Ob callback에 우선 초점**

### A-1. 깨끗한 baseline 확보

```cmd
REM 1. 대표 현장 PC 재부팅
REM 2. 부팅 완료 10분 대기
REM 3. RAMMap 덤프 (v2.2: /accepteula 추가, 서비스 계정 실행 대비)
rammap.exe -accepteula -o baseline.rmp

REM 4. perfmon baseline
typeperf "\Memory\Pool Paged Bytes" "\Memory\Pool Nonpaged Bytes" -sc 60 -si 1 -o baseline_perfmon.csv
```

### A-2. With-ResourceAgent 측정

(v2.1과 동일 — ResourceAgent 설치 후 24~72h 후 재측정, 델타 기록)

### A-3. Pool 분석 (v2.2 정정)

**v2.2 정정 사항**:
- RAMMap `-Rt` 옵션은 존재하지 않음. `rammap.exe -accepteula -o snapshot.rmp` 덤프 후 다른 PC GUI 분석.
- **PoolMon 원격 (v2.2 정정)**: PoolMon 자체는 로컬 전용. `PsExec \\win7pc -c poolmon.exe -n pool.txt` 로 **PsExec으로 Win7에 poolmon.exe 임시 복사 + 실행**.
- `xperf -on POOL` WPT 단일 zip 배포: **라이선스 경계 회색지대**. 사내 개발·테스트 목적 허용이지만 공식 최소 파일셋 미문서화. 파일셋 조사 선행 필요.

**실사용 권장 절차**:
```
1. 현장 Win7 PC에서:
   rammap.exe -accepteula -o snapshot.rmp
   
2. 관리자 PC (Win10)로 snapshot.rmp 복사
3. 관리자 PC에서 RAMMap GUI로 열어서 Use Counts 탭 분석
4. Pool Tag → 드라이버 매핑:
   - Microsoft 공식 pooltag.txt 참조 (MS 드라이버용)
   - 서드파티: strings <driver>.sys | findstr /i "<tag>"
5. (선택) PoolMon 원격: PsExec으로 임시 복사 실행
6. (선택) xperf -on POOL: WPT 파일셋 확정 후 배포
```

### A-4. 선택적 비활성화 실험 (병렬 8대, v2.3 확장)

**중요 (v2.3)**: 이 실험의 일시 off는 **진단 목적**. 24시간 후 원복하여 변수 분리. 영구 off 아님.

**LhmProvider 공유 의존성**: Temperature/Fan/GPU/Voltage/MotherboardTemp/StorageSmart는 모두 LhmProvider 경유. 개별 collector만 off하면 LhmProvider 자체는 살아 있어 원인 분리 불가. → LHM-all-off variant로 LhmProvider+PawnIO 영향 통째로 평가.

| PC | 일시 비활성화 | 기간 | 원인 분리 목적 |
|----|------------|-----|------------|
| PC-A4-1 | Disk | 24h | gopsutil raw device open 영향 |
| PC-A4-2 | CPUProcess | 24h | gopsutil 프로세스 enum 영향 |
| PC-A4-3 | MemoryProcess | 24h | (위와 동일, double-check) |
| PC-A4-4 | ProcessWatch | 24h | gopsutil + WatchProcess 영향 |
| PC-A4-5 | StorageHealth (WMI) | 24h | WMI 호출 경로 영향 |
| **PC-A4-6** | **LhmHelper + LHM 기반 collector 전부** (v2.3 신설) | 24h | **LhmProvider/PawnIO 드라이버 상주 영향** (E-Win-3 통합) |
| **PC-A4-7** | **StorageSmart only** (v2.3 신설) | 24h | LHM S.M.A.R.T raw device open 단독 영향 |
| **PC-A4-8** | **모든 collector off (heartbeat만)** (v2.3 신설) | 24h | 잔여 baseline (ResourceAgent 본체 영향) |

**판정**:
- A4-6 vs baseline에서 큰 차이 → LhmProvider/PawnIO 원인. Phase 3-5 + Appendix F 처리.
- A4-7 vs A4-5 비교 → WMI vs LHM IOCTL 어느 쪽이 AV 트리거 큰지 분리.
- A4-8 vs baseline 차이 = ResourceAgent 본체(Go runtime + heartbeat) 잔여 영향. 매우 작아야 정상.

**일시 off 방법** (배포 변경 아님):
- 진단용 Monitor.json은 `experimental` 태그 PC에만 적용
- 실험 종료 후 즉시 원본 Monitor.json 복원 (자동 스크립트)
- 배포본 Monitor.json/DefaultConfig는 **변경하지 않음**

### A-5. AV 예외 등록 실험

(v2.1과 동일)

### A-6. 보조 확증

(v2.1과 동일 + SSDT 후킹 확인 도구 결과 포함)

### Phase A 산출물

- `docs/issues/win7-kernel-pool-growth.md`
- 1순위 가설 확정/기각
- Phase 3 우선순위 재조정
- **SLO-1 최종 계수 확정**

### Phase A 체크리스트

- [ ] **A-0**: 대체 가설 전수 조사
  - [ ] `fltmc instances` + Altitude/Frame 해석
  - [ ] `driverquery /v /fo csv`
  - [ ] `logman query -ets` + non-MS 필터
  - [ ] AV/DLP 서비스 식별
  - [ ] **SSDT 후킹 탐지** (WinDbg `!chkimg` 또는 GMER 원격)
  - [ ] 통계 기반 가설 가중치 (±2σ)
- [ ] **A-1**: baseline (`rammap.exe -accepteula -o` + typeperf)
- [ ] **A-2**: With-ResourceAgent 측정
- [ ] **A-3**: Pool 분석 (RAMMap GUI + PsExec PoolMon + (선택) xperf)
- [ ] **A-4**: 선택적 비활성화 (병렬 8대, v2.3 확장)
  - [ ] PC-A4-1~5 (개별 collector)
  - [ ] PC-A4-6 (LHM-all-off, E-Win-3 통합)
  - [ ] PC-A4-7 (StorageSmart only)
  - [ ] PC-A4-8 (heartbeat-only baseline)
  - [ ] 실험 종료 후 Monitor.json 원복 자동화 검증
- [ ] **A-5**: AV 예외 등록
- [ ] **A-6**: 보조 확증
- [ ] **산출물**: 원인 특정 보고서 + SLO-1 계수 확정

---

## Phase 0 — 저위험 코드 수정 (v2.3 축소)

**v2.3 원칙 적용**: "수집기 영구 disable + 배포 Monitor.json 변경" 전략은 **삭제**. 이유:
- 영구 disable은 증상 감추기. 원인 미해결 + 진단 능력 손실.
- 영구 안 쓸 수집기라면 **코드에서 삭제**가 맞음 (현재 그 결정 못 함 — 기능 가치 평가 미완)
- 따라서 Phase 0는 **확실히 안전한 코드 버그 fix**(H1)만 남김

**삭제된 항목 (v2.3)**:
- ~~0-0. Downstream 영향 사전 조사~~ — 배포 설정 변경 안 함 → 불필요
- ~~0-1. Disk/StorageHealth 기본값 disabled~~ — 증상 감추기, 원칙 위배

**Phase A 진단 중 일시 off는 별개**: A-4 매트릭스 참조. `experimental` 태그 PC에만 적용, 24h 후 자동 원복.

### 0-1. drainStderr 에러 핸들링 + panic 복구 (H1)

**수정 파일**: `lhm_provider_windows.go:396-403`

(v2.1 코드 예시와 동일)

**테스트**: `testing/iotest.ErrReader` + panic 유발 reader helper. Phase 4-0 선결 전이라도 local 검증 가능.

**근거**: 이건 코드 버그 fix이지 수집기 disable 아님. drainStderr가 panic하면 LhmProvider 전체가 죽는 위험을 잡는 것. 수집기 기능은 그대로 유지.

### Phase 0 체크리스트

- [ ] **0-1**: drainStderr 수정 + `iotest.ErrReader` 테스트 + panic reader 테스트

---

## Phase 1 — Critical Goroutine Leak 수정

### 1-1-spike. bufio.Reader 해체 + stdin SetWriteDeadline 설계 + PoC (v2.2 확장)

**배경 (v2.2)**: v2에서 `stdout *bufio.Reader` 래핑 문제 지적, v2.1에서 spike 신설. v2.2에서 **stdin SetWriteDeadline까지 포함하여 완결**.

**v2.2 추가 배경**: Go 전문가 지적 — stdin write도 block 가능 (LhmHelper가 stdin 읽지 않는 경우). `SetReadDeadline`만 처리하면 stdin write block 시 여전히 goroutine freeze.

**작업**:
- [ ] **산출물 형태 명시**:
  - (a) 설계 문서: `docs/designs/lhm-provider-deadline-redesign.md`
  - (b) PoC 코드: 별도 spike 브랜치에 SetReadDeadline + SetWriteDeadline 단일 테스트 통과 확인
- [ ] 기존 `stdout` 필드 참조 **전수 조사** (8곳 예상):
  - `lhm_provider_windows.go:98`, 305, 393, 411-412, 421, 440-442, 458
  - 결과물: 참조 경로 diff 리스트
- [ ] **필드 분리 설계**:
  - `stdoutFile *os.File` (원본)
  - `stdoutReader *bufio.Reader` (버퍼)
  - `stdinFile *os.File` (v2.2 추가)
- [ ] **타임아웃 복구 로직**:
  - 타임아웃 발생 시 `stdoutFile.SetReadDeadline(time.Time{})` 리셋
  - 동시에 `stdoutReader.Reset(stdoutFile)` — bufio 내부 상태(부분 읽기 offset) 초기화
  - stdin도 마찬가지: `stdinFile.SetWriteDeadline(time.Time{})` + flush
- [ ] **startup `doRequest` 경로 일관성** (v2.2 신설):
  - `Start()` 내부의 초기 `doRequest` 호출(line 327)도 freeze 가능성
  - `doRequest`도 `doRequestWithTimeout`과 동일 deadline 경로 사용하도록 통합
  - 또는 startup 전용 짧은 timeout (예: 5초) 별도 적용
- [ ] **Timeboxing 룰 (v2.2 신설)**:
  - Day 1 종료 시점: 잔여 작업량 재산정
  - Day 2 종료 시: 잔여 3일 이상이면 **옵션 B 전환 판단**
  - Day 3 완료 시: 실패 시 옵션 B 강제 전환

- [ ] **PoC 합격 조건 (Hard Gate, v2.3 신설)**:
  옵션 A (`SetReadDeadline` + `SetWriteDeadline`) 채택의 모든 조건 충족 필요:
  1. **Win7 x64 + 서비스 계정** (`LocalSystem`)에서 LhmHelper hang 모드 (stdin 안 읽음)에 대해 `SetWriteDeadline` 정상 발화
  2. **Win7 x86** 동일 시나리오 통과
  3. anonymous pipe (`os/exec` 기본) 환경에서 deadline 동작 확인 (named pipe 아님)
  4. `bufio.Reader` reset 후 재사용 시 다음 정상 응답 정상 파싱 (offset 오염 없음)
  5. 1분간 100회 timeout 반복에도 goroutine 누적 0 (goleak 검증)
  
  **하나라도 실패 시 옵션 B(`Process.Kill()`) 강제 전환**. Phase 4-4 KillPath 벤치 결과로 의사결정 트리 적용.
  
  **PoC 검증 환경 명시**:
  - 실기 Win7 SP1 PC (32/64 각각) — 사이트 협조 필요
  - VM 검증은 보조 (`os/exec` pipe 동작이 hardware/driver와 무관해야 함)
  - 검증 산출물: `docs/designs/lhm-provider-deadline-poc-results.md` — 5개 조건 각각 PASS/FAIL 표

### 1-1. LhmProvider.doRequestWithTimeout 수정

**옵션 A (권장, spike 성공 시)**:
- `stdinFile.SetWriteDeadline` + `stdoutFile.SetReadDeadline` 설정
- 타임아웃 감지 + bufio.Reset + deadline 리셋
- 별도 goroutine 불필요

**옵션 B (fallback)**: `p.cmd.Process.Kill()`
- 주의: `cmd.Wait()` 대기는 **수십~수백 ms** (v2.1 정정 유지)
- Phase 4-4 `BenchmarkDoRequestTimeout_KillPath` 실측

**옵션 B 벤치 결과 의사결정 트리 (v2.2 신설)**:

| 측정 lock hold 시간 | 판단 | 조치 |
|-------------------|------|------|
| < 100ms | 안전 — scheduler 30s 대비 충분 margin | 옵션 B 채택 |
| 100~300ms | 경계 — 다른 collector 지연 가능성 | 옵션 B 채택 + scheduler 주석에 경합 기록 |
| 300ms~1s | 위험 — cascading block 가능성 | 옵션 A 강제 재시도 (추가 spike 3일) |
| > 1s | 차단 — 프로덕션 불가 | 옵션 A 필수, 또는 scheduler timeout 60s 연장 검토 |

### 1-2. WMI Query goroutine leak **코드 수정** (C2, v2.3 재정의)

**v2.3 변경 사유**: v2.2는 "Phase A 결과 대기"였음 — 즉 StorageHealth가 원인이면 그때 worker 전환, 아니면 현행 유지. 그러나 **goroutine leak은 코드 버그 그 자체**. 원인 가설 결과와 무관하게 다음 조건이면 누적됨:
- WMI 쿼리가 30s timeout 안에 응답 안 하면 goroutine 회수 불가 (영구 누적)
- WMI 자체가 hang 빈도 높지 않더라도, Win7 환경에서 WMI 서비스 일시 hang 발생 시 매 interval(5분)마다 1개씩 leak

**수정 파일**: `storage_health_windows.go:64-85`

**구현 방향 (3안 중 선택)**:

**옵션 A — Worker goroutine 풀 + 단일 처리 직렬화**:
```go
type wmiWorker struct {
    requests chan wmiRequest
    done     chan struct{}
}

// 단일 worker가 wmi.Query 직렬 처리
// timeout 시 상위는 즉시 반환하지만 worker는 계속 살아 응답을 drain
// 새 요청이 들어와도 이전 요청 진행 중이면 대기 (timeout 시 대체 응답)
```
- 장점: goroutine 수 1개로 고정. 누적 없음.
- 단점: WMI 영구 hang 시 새 요청 차단 → 별도 watchdog 필요.

**옵션 B — `Ole32 CoInitializeEx` + `IWbemServices::CancelAsyncCall`**:
- WMI COM API 직접 호출하면서 `IWbemServices::ExecQueryAsync` 사용
- timeout 시 `CancelAsyncCall`로 진행 중 쿼리 취소 가능
- 단점: gopsutil/yusufpapurcu/wmi 의존 깨짐, CGO/syscall 직접 호출 필요. 구현 비용 높음.

**옵션 C — 이미 진행 중인 쿼리에 대한 referenced count + force-skip**:
- 현재 goroutine 채널 패턴 유지하되, 쿼리 진행 중 플래그 추가
- 진행 중이면 새 호출은 즉시 stale-cache 반환 + 로깅
- timeout으로 새 goroutine 생성 안 됨 → 누적 방지
- 장점: 최소 변경. 단점: stale 데이터 윈도우 발생.

**권장**: 옵션 C (최소 변경 + 즉시 leak 방지). 옵션 A는 hang 빈도 높으면 검토. 옵션 B는 비용 대비 효과 낮음.

**수집기 기능 유지**: WMI hang 자체는 못 막지만 goroutine 누적은 차단. StorageHealth 정상 응답 경로는 영향 없음.

**테스트**:
- [ ] Mock `wmi.Query`로 hang 시나리오 재현 (10s sleep 모사)
- [ ] 100회 timeout 발생 후 `runtime.NumGoroutine` baseline ±2 이내 (goleak)
- [ ] 정상 응답 경로 회귀 (기존 통합 테스트)

### Phase 1 체크리스트

- [ ] **1-1-spike** (v2.2 확장)
  - [ ] 산출물: 설계 문서 + PoC 브랜치
  - [ ] `stdout` 필드 참조 8곳 전수 조사
  - [ ] `stdoutFile` / `stdoutReader` / `stdinFile` 필드 분리 설계
  - [ ] 타임아웃 복구 로직 (deadline 리셋 + bufio.Reset)
  - [ ] startup `doRequest` 경로 통합 설계
  - [ ] Timeboxing 룰 적용 (3일 상한)
- [ ] **1-1**: doRequestWithTimeout 수정
  - [ ] 옵션 A 또는 B 확정 (spike 결과 + 벤치 기반)
  - [ ] fake_daemon slow 모드 pipe-block 계약 확인 (Phase 4-6 **선행 필수**)
  - [ ] goleak 회귀 테스트 (10회 연속 타임아웃)
  - [ ] 옵션 B 선택 시 벤치 결과에 따른 후속 조치
  - [ ] 옵션 B 선택 시 Scheduler 경합 통합 테스트
- [ ] **1-2** (v2.3 재정의): WMI goroutine leak 코드 수정
  - [ ] 옵션 A/B/C 선정 (권장 C — 최소 변경)
  - [ ] Mock wmi.Query hang 시나리오 단위 테스트
  - [ ] goleak: 100회 timeout 후 goroutine baseline ±2 이내
  - [ ] 정상 응답 경로 회귀
  - [ ] StorageHealth 수집기 기능 유지 검증 (E2E)

---

## Phase 2 — 견고성 개선

### 2-1. BufferedHTTPTransport 버퍼 상한 (v2.3 재설계)

**v2.3 변경 사유 (Codex M-1 + E-Go-1 통합)**: v2.2의 atomic 카운터로 enforcement하는 설계는 race-prone:
- `Store(0)` vs 동시 `Add(+n)` 순서 race로 underflow/overflow
- atomic load로 상한 체크 후 Lock 획득 사이에 다른 Deliver가 끼어들면 상한 초과 가능
- atomic은 lock-free 관측에는 좋지만, **상한 enforcement 같은 일관성 필요한 로직에 부적절**

**v2.3 재설계 원칙**:
- **Enforcement는 mu critical section 내부**: append와 drop을 같은 Lock 안에서 처리. 상한 검사도 Lock 안.
- **atomic은 관측용으로만**: SelfMetrics가 lock 없이 빠르게 읽기 위한 hint. 정확성은 mu가 보장.
- `bufferCount` 필드는 `int` (lock 보호) + 별도 `bufferCountObs atomic.Int64` (관측용 best-effort 미러)

**구현 계약**:
```go
type BufferedHTTPTransport struct {
    mu             sync.Mutex
    buffer         []bufferedEntry
    bufferCount    int                // mu 보호. enforcement source of truth
    bufferCountObs atomic.Int64       // 관측용 (SelfMetrics, lock-free 읽기)
    // ...
}

func (t *BufferedHTTPTransport) Deliver(_ context.Context, topic string, records []KafkaRecord) error {
    t.mu.Lock()
    
    // 1. append
    t.buffer = append(t.buffer, bufferedEntry{topic: topic, records: records})
    t.bufferCount += len(records)
    
    // 2. 상한 enforcement (같은 critical section)
    droppedCount := 0
    for t.bufferCount > t.batchCfg.MaxBufferedRecords && len(t.buffer) > 0 {
        droppedCount += len(t.buffer[0].records)
        t.bufferCount -= len(t.buffer[0].records)
        t.buffer = t.buffer[1:]
    }
    
    // 3. flush 트리거 판단
    needFlush := t.bufferCount >= t.batchCfg.FlushMessages
    countSnapshot := t.bufferCount
    
    t.mu.Unlock()
    
    // 4. 관측값 update (관측용 best-effort)
    t.bufferCountObs.Store(int64(countSnapshot))
    
    if droppedCount > 0 {
        // sampled drop 로그
        t.logDropSampled(droppedCount)
    }
    
    if needFlush {
        select {
        case t.flushCh <- struct{}{}:
        default:
        }
    }
    return nil
}

func (t *BufferedHTTPTransport) flush(trigger string) {
    t.mu.Lock()
    if len(t.buffer) == 0 {
        t.mu.Unlock()
        return
    }
    entries := t.buffer
    flushed := t.bufferCount
    t.buffer = nil
    t.bufferCount = 0
    t.mu.Unlock()
    
    // 관측값 update
    t.bufferCountObs.Store(0)
    
    // ... (기존 flush 로직: HTTP POST 등)
    // 실패 시 재append는 별도 정책 (현재는 drop)
}
```

**중요 변경점 (v2.3)**:
- ~~`bufferCount.Store(0)` + `bufferCount.Add(+n)` race~~ → `mu` 보호하 정수 연산
- ~~`Load()` 후 상한 체크~~ → Lock 안에서 append+drop 한 번에
- `bufferCountObs`는 SelfMetrics 노출용 hint. SLO-3 drop count 등 정확한 값은 별도 metric에서 (Lock 안에서 atomic.Add).

**정책**: "oldest drop" (v2.1 근거 유지)

**MaxBufferedRecords config 경로 확장 (Codex M-2)**:

수정 파일 (v2.3 확장):
- `internal/config/config.go` — `BatchConfig.MaxBufferedRecords int` 필드 추가
- `internal/config/loader.go` — `rawBatchConfig.MaxBufferedRecords int` + `convertRawBatch`에서 매핑
- `internal/config/loader.go` — Merge 로직 (있다면) 확장
- `internal/config/validate.go` — 음수/0 검증 + 기본값 (`FlushMessages * 10`)
- `internal/config/defaults.go` — DefaultConfig에 기본값
- `configs/ResourceAgent.json` — 샘플 추가 (예: `"MaxBufferedRecords": 10000`)
- `conf/ResourceAgent/ResourceAgent.json` — 배포본 샘플 추가
- `internal/sender/kafkarest.go` — Deliver/flush enforcement 구현
- 단위 테스트: config load + validate + Deliver 상한 검증

**Drop 로그**: zerolog `BasicSampler{N: 10}` (분당 1회 샘플 수준)

**clock injection**: `benbjohnson/clock v1.3.5` (Phase 4-0 migration 완료 후)

**Race 테스트 (v2.3 강화)**:
- [ ] 동시 Deliver(10 goroutine × 1000회) + flush(별도 goroutine) → `bufferCount`와 실제 `len(buffer)` 항상 일치 (Lock으로 자동 보장됨)
- [ ] `go test -race -count=20`로 flaky 배제
- [ ] MaxBufferedRecords 도달 시 정확한 drop count 검증 (overflow 0, underflow 0)
- [ ] Lock 경합 벤치마크 (Phase 4-4): Deliver 처리 시간 baseline + 5% 이내

### 2-2. Scheduler 주석 보강

(v2.1과 동일)

### Phase 2 체크리스트

- [ ] **2-1** (v2.3 재설계): BufferedHTTPTransport 상한 (mu critical section enforcement)
  - [ ] `BatchConfig.MaxBufferedRecords` config 경로 확장:
    - [ ] `config.go`, `loader.go` (raw + convert), `validate.go`, `defaults.go`
    - [ ] `configs/ResourceAgent.json`, `conf/ResourceAgent/ResourceAgent.json` 샘플
  - [ ] Deliver 안에서 append + drop + 상한 enforcement (Lock 단일 critical section)
  - [ ] `bufferCount int` (mu 보호) + `bufferCountObs atomic.Int64` (SelfMetrics 관측용)
  - [ ] flush 시 bufferCount = 0 (Lock 안)
  - [ ] `zerolog.BasicSampler{N: 10}` drop 로그
  - [ ] clock injection (Phase 4-0 후)
  - [ ] **Race 테스트** (동시 Deliver 10×1000 + flush, `-race -count=20`)
  - [ ] MaxBufferedRecords overflow/underflow 0 검증
  - [ ] Lock 경합 벤치 (baseline + 5% 이내)
- [ ] **2-2**: Scheduler 주석 보강

---

## Phase 2.5 — 관측성 & 배포 인프라

### 2.5-0. ManagerAgent 계약 Kickoff

- Week 0 W0-3에서 확정된 RACI 기반 Kickoff
- M1~M4 마일스톤 (v2.1과 동일)
- **Kafka topic 결정 (M1)**: `eag.selfmetric` 별도 권장 (feedback loop 방지)
- Fallback: 수동 FTP 6~9대 한정 (카나리까지만 유효)

### 2.5-1. SelfMetrics + Dead-man's switch + runtimeStats seam (v2.2 강화)

**기본 지표**:
- `process_rss_bytes`, `process_vms_bytes`
- `goroutine_count` (seam 기반)
- `handle_count` (Windows)
- `buffered_records`, `buffer_drop_count`
- `collector_success_rate`
- `paged_pool_bytes`, `nonpaged_pool_bytes` — **`lxn/win` 또는 `pdh.dll` syscall** 경유 (v2.2: 라이브러리 의존성 명시)

**`runtimeStats` Seam 추상화 (v2.2 신설)**:
```go
type RuntimeStatsProvider interface {
    NumGoroutine() int
    NumCgoCall() int64
    MemStats() *runtime.MemStats
    ProcessHandleCount() (uint32, error)  // Windows only
    PagedPoolBytes() (uint64, error)      // Windows only
}

type SelfMetricsCollector struct {
    BaseCollector
    stats RuntimeStatsProvider   // 테스트에서 mock 주입 가능
    // ...
}
```
- 프로덕션 구현: `runtime.NumGoroutine()` 등 직접 호출
- 테스트 구현: 값을 임의 설정 가능한 mock
- goroutine drift 시나리오 의도적 재현 가능

**SLO-2 상대 drift 계산 (v2.2 정정)**:
- 프로세스 시작 후 60초간 baseline 평균 계산
- 이후 절대값이 아닌 **baseline 대비 drift ±10**으로 평가

**Dead-man's Switch 2중 방어**:
- heartbeat-miss: Kafka consumer 측에서 `last_emit_ts` 120s stale 감지 → alarm
- 외부 ping: ManagerAgent가 `sc.exe query ResourceAgent` 주기적 polling (1분, 3회 연속 실패 시 alarm)
- **Ping 실패 원인 구분 (v2.2 신설)**:
  - 같은 사이트 다른 PC 2~3대 상태 집계
  - 사이트 전체 실패 → 네트워크 문제로 판정, auto-hold만 (auto-rollback 안 함)
  - 특정 PC만 실패 → ResourceAgent 문제로 판정, 해당 PC rollback 고려

**자기 교란 방어**:
- `bufferRecordCount()` atomic (Phase 2-1에서 이미 구현)
- Kafka topic 분리 (`eag.selfmetric`)
- 별도 우선순위 큐 (feedback loop 시 SelfMetrics 자체가 drop되지 않도록)

### 2.5-2. Feature flag / kill-switch

(v2.1과 동일)

### 2.5-3. 롤백 Runbook + 자동 트리거 Chain + **회로 차단기** (v2.2 신설)

**산출물**: `docs/runbooks/resourceagent-rollback.md`

**Artifact 보관**: N-1 ResourceAgent.exe + Monitor.json, FTP `rollback/`, 90일 (W0-5 용량 확보)

**원격 실행**: ManagerAgent 경유 `install_ResourceAgent.bat /rollback`

**자동 트리거 Chain**:
```
SelfMetrics → Kafka → Alerting(W0-1 확정 인프라)
                    ↓
              SLO 위반 감지 (5분 내 N대 이상)
                    ↓
              회로 차단기 체크 (v2.2 신설)  ← NEW
                    ↓
              자동 hold (ManagerAgent 워크플로우 엔진)
                    ↓
              15분 on-call 무응답 → auto /rollback
```

**회로 차단기 (v2.2 신설)**:
- **Rate limit**: 1시간 내 rollback 발동 전체 PC의 **5% 상한** (10,000대면 500대)
- **Cooldown**: rollback 1회당 30분 cooldown, 연속 발동 방지
- **Sitewide lock**: 사이트 전체 ping 실패(네트워크 장애)는 rollback 금지 — W0-1 ping 실패 원인 구분 로직과 연동
- **Kill-switch**: ManagerAgent 관리 콘솔에서 수동 비활성화 가능 (긴급 상황용)

**RACI**: Week 0 W0-3 확정

### 2.5-4. ManagerAgent 원격 롤백 명령 구현

- W0-3 계약 기반
- `/rollback` 옵션 + 부분 롤백 필터

### 2.5-5. 로그/메트릭 중앙 수집 경로

(v2.1과 동일)

### Phase 2.5 체크리스트

- [ ] **2.5-0**: ManagerAgent Kickoff (W0 RACI 기반)
- [ ] **2.5-1**: SelfMetrics + Dead-man's switch + seam
  - [ ] `RuntimeStatsProvider` interface + mock
  - [ ] 8개 지표 + `paged_pool_bytes` (pdh.dll 경유)
  - [ ] heartbeat-miss + 외부 ping
  - [ ] Ping 실패 원인 구분 (사이트 집계)
  - [ ] Kafka topic 분리
  - [ ] SLO-2 상대 drift 계산
  - [ ] goleak + 자기 교란 방어 테스트
- [ ] **2.5-2**: Feature flag
- [ ] **2.5-3**: 롤백 Runbook + 자동 Chain + **회로 차단기**
  - [ ] 5% 상한 + 30분 cooldown + sitewide lock + kill-switch
- [ ] **2.5-4**: ManagerAgent 원격 명령 구현
- [ ] **2.5-5**: 로그/메트릭 중앙 수집

---

## Phase 3 — AV Trigger 감소 리팩토링

### 3-0. 대체 접근 검토 (v2.2 신설)

**v2.2 추가 배경**: Windows 전문가 지적 — ETW consumer API / PerfLib v2 / Job Object 등 유력 대안 미검토.

**작업**:
- [ ] **ETW consumer API 직접 호출 검토**:
  - `OpenTrace` / `ProcessTrace`로 `Microsoft-Windows-Kernel-Process` 이벤트 수신
  - 프로세스 enumeration을 snapshot이 아닌 event stream으로 대체
  - AV가 ETW 필터할 가능성은 상대적으로 낮음
  - 구현 난이도: 중~상 (CGO 또는 syscall 직접 호출)
- [ ] **PerfLib v2 (`PerfStartProviderEx`) 검토**:
  - 신형 counter API, 커널 경유 안 함
  - Win7 지원 제한 (부분)
- [ ] **`NtQuerySystemInformation(SystemProcessorPerformanceInformation)` 검토**:
  - CPU per-core 사용률을 한 번의 syscall로 획득
  - 현재 gopsutil CPU collector가 여러 번 호출하는 부담 감소
- [ ] **Job Object 기반 ProcessWatch 검토**:
  - `NtQueryInformationJobObject`로 프로세스 생성/종료 이벤트 수신
  - polling 제거 가능
  - 구현 난이도: 상 (큰 리팩토링)

**결정**: 각 대안의 구현 비용 vs AV 트리거 감소 효과 평가 후 Phase 3-1/3-2/3-3 우선순위 재조정. Phase A 결과와 교차 검증.

### 3-1. NtQuerySystemInformation 기반 enumeration

**Go 구현 주의사항 (v2.2 확장 — 6건)**:

1. **`runtime.KeepAlive(buf)`** — `unsafe.Pointer` 캐스팅 후 GC 방어
2. **`NextEntryOffset == 0` 가드** — 링크드 리스트 순회 종료
3. **`STATUS_INFO_LENGTH_MISMATCH` 루프** + `sync.Pool` + 최대 5회 재시도
4. **32/64 포인터 크기 분기 (v2.2 신설)**:
   - `SYSTEM_PROCESS_INFORMATION.UniqueProcessId`는 `HANDLE` (포인터 크기)
   - 386 빌드에서 `uint32`, amd64에서 `uint64`
   - `uintptr` 사용 + `GOARCH` 빌드 태그로 구조체 분기
5. **UNICODE_STRING `ImageName` 처리 (v2.2 신설)**:
   - `ImageName.Buffer`는 UTF-16 포인터, Length/MaximumLength 바이트 단위
   - `windows.UTF16PtrToString` + null 포인터 가드
6. **구조체 alignment 검증 (v2.2 신설)**:
   - `CreateTime` (LARGE_INTEGER = int64) 8-byte aligned
   - 32-bit 빌드에서 Go 구조체 패딩이 Windows 네이티브 레이아웃과 불일치 가능성
   - `unsafe.Sizeof` cross-check + 필요 시 수동 패딩

**Win7 세대 고려**:
- PatchGuard 이전 세대로 SSDT 후킹 가능 (확률 20~30%)
- OpenProcess 경로의 Ob callback은 완전 회피, SSDT는 여전히 가능
- Phase A-0 결과 SSDT 후킹 확인되면 NtQuery 효과 제한적

### 3-2. ProcessSnapshot + singleflight + (PID, CreateTime) tuple + FILETIME 정규화 (v2.2 확장)

**FILETIME 정규화 레이어 (v2.2 신설)**:
- gopsutil `(*Process).CreateTimeWithContext` 는 **Unix ms** 반환
- NtQuery `SYSTEM_PROCESS_INFORMATION.CreateTime` 은 **FILETIME (100ns since 1601)**
- 두 경로가 동일 tuple key로 비교되려면 정규화 필요:
  ```go
  // FILETIME → Unix ms 변환
  // FILETIME epoch: 1601-01-01, Unix epoch: 1970-01-01
  // Difference: 11644473600 seconds = 116444736000000000 * 100ns
  const FiletimeToUnixMs = 116444736000000
  
  func filetimeToUnixMs(ft int64) int64 {
      return (ft - FiletimeToUnixMs * 10000) / 10000
  }
  ```

**singleflight + `ttl_epoch` key**:
- `ttl_epoch = floor(now.UnixNano() / ttl.Nanoseconds())`
- TTL 경계 race 방지

**(PID, CreateTime) tuple 키**:
- stale 감지: 저장 시점 CreateTime과 사용 시점 CreateTime 비교
- 불일치 시 PID 재사용 판단, stale 처리

**Race 테스트 (v2.2 신설)**:
- [ ] singleflight 동시 N=100 호출 → enumerator call count == 1
- [ ] tuple stale 감지: mock으로 PID 재사용 시나리오 → detect 확인
- [ ] `ttl_epoch` 경계 race: time mock으로 재현

### 3-3. 세 collector 전환 (gopsutil seam 감사 선행)

**선행 조사**:
- 세 collector의 gopsutil 직접 호출 전수 조사
- 테스트 implementation detail 의존도 감사
- `gopsutil mockable seam audit` 리포트 → Phase 4-5 Golden test 실행 가능성 확인
- 필요 시 `ProcessProvider` 인터페이스 도입

**리팩토링**:
- Feature flag on/off 양쪽 회귀 테스트

### 3-4. (conditional) Disk IOCounters 대안

Phase A-4 결과 Disk 주원인이면 PDH 기반 구현.

### 3-5. (conditional, **기술 제약 인정**) StorageHealth 대안

**v2.2 명확화**: 3안 중 어느 것도 완벽한 해결 아님.

- **A. WMI 재수용 + 빈도 감소** (5분 → 1시간):
  - AV 트리거 횟수 1/12로 감속 — **누수 지연이지 해소 아님** (v2.2 명시)
  - 재부팅 주기(월 1회)에서 총량 임계 이하면 실용적
- **B. LhmHelper StorageSmart 대체**:
  - LHM의 S.M.A.R.T도 `\\.\PhysicalDriveN` 에 `SMART_RCV_DRIVE_DATA` IOCTL 사용
  - **raw device open 경로 동일 → AV 관점에서 이득 불명** (v2.2 명시)
  - .NET Framework 4.7 의존성 유지
- **C. 기능 포기**: 완전 disabled

**결정**: Phase A 결과 + 후단 파이프라인 협의 후.

### Phase 3 체크리스트

- [ ] **3-0** (v2.2 신설): 대체 접근 검토
  - [ ] ETW consumer API PoC
  - [ ] PerfLib v2 검토
  - [ ] NtQuery(SystemProcessorPerformanceInformation) 검토
  - [ ] Job Object ProcessWatch 검토
  - [ ] 우선순위 재조정 보고서
- [ ] **3-1**: NtQuery enumeration
  - [ ] 6건 Go 구현 주의사항 (KeepAlive, NextEntryOffset, sync.Pool, 32/64 분기, UNICODE_STRING, alignment)
  - [ ] Win7 SP1 호환 필드 + CreateTime
  - [ ] Feature flag 연동
  - [ ] 단위 테스트 + gopsutil 일치 비교
- [ ] **3-2**: ProcessSnapshot
  - [ ] singleflight + `ttl_epoch`
  - [ ] (PID, CreateTime) tuple 키
  - [ ] FILETIME → Unix ms 정규화 레이어
  - [ ] Race 테스트 3종
- [ ] **3-3**: 세 collector 전환
  - [ ] gopsutil seam 감사 리포트 선행
  - [ ] `ProcessProvider` 인터페이스 (필요 시)
  - [ ] Golden test 선행 (Phase 4-5)
  - [ ] Feature flag on/off 회귀
- [ ] **3-4** (conditional): Disk PDH
- [ ] **3-5** (conditional): StorageHealth 3안 판단

---

## Phase 4 — 테스트 & 검증

### 4-0. 선결 과제 (v2.3.1 갱신, Step 0 PR로 도입 완료)

- [x] **goleak 도입**: `go.uber.org/goleak v1.3.0` — Step 0 PR
  - `internal/sender/sender_test.go` TestMain (sarama / http / lumberjack 화이트리스트)
  - `internal/collector/collector_test.go` TestMain
  - `docs/runbooks/goleak-whitelist.md`
- [x] **clock injection**: `benbjohnson/clock v1.3.5` — Step 0 PR
  - `internal/sender/clock.go` 인터페이스 도입
  - 테스트 5초 sleep 2곳 단축 (200ms로) + clock 자기검증 테스트
  - production 코드 교체는 Step 5(Phase 2-1)에서
- [x] **로컬 CI 스크립트 + CSV 이력 (v2.3.1: GitHub Actions 대체)**: Step 0 PR
  - `scripts/ci.sh` + `scripts/ci.ps1`: gofmt → vet → test → goleak (basic) → race + cross-build (--full)
  - `docs/runbooks/ci-history.csv` (자동 append, PASS/FAIL 양쪽 기록)
  - `docs/runbooks/ci-history-policy.md`
  - **R-1 race condition 발견** (FileSender stdout swap) → `docs/runbooks/known-race-conditions.md` 별도 fix PR
- [x] **Coverage baseline 실측 (Day 1)**: 2026-04-26
  - **Overall: 58.5%** (macOS, Windows-only 코드 제외)
  - `internal/collector/`: 46.4% (function avg)
  - `internal/sender/`: 78.9% (function avg)
  - 파일: `docs/plans/coverage-baseline.txt`, `docs/plans/coverage-baseline.md`
  - Phase 2 완료 후: 60.5% 이상 (+2%p)
  - Phase 3 완료 후: 63.5% 이상 (+5%p)
  - 절대 하한: 70%
- [ ] **gopsutil mockable seam 감사 리포트** (Phase 3-3 선행)

### 4-1. 단위 테스트 전면 통과

(v2.1과 동일)

### 4-2. goleak 회귀 방지

8개 대상 (v2.1과 동일)

### 4-3. 플랫폼별 통합 테스트

- Windows VM 월 1회 수동 (압축 스트레스 시나리오)
- Linux regression
- `-tags=integration`

### 4-4. 벤치마크 + 의사결정 트리 (v2.2 확장)

- [ ] `BenchmarkProcessEnumeration_NtQuery` vs `Gopsutil` — 10% 이상 개선 시 Phase 3-1 채택
- [ ] `BenchmarkBufferedHTTPTransport_Deliver` — Phase 2-1 오버헤드
- [ ] **`BenchmarkDoRequestTimeout_KillPath`** (v2.2): 옵션 B lock hold 실측
  - 결과에 따른 의사결정 트리 (Phase 1-1 참조)

### 4-5. Golden test (Phase 3 리팩토링 선행)

**선행 조건**: Phase 4-0의 gopsutil seam 감사 완료 + Phase 3-3 `ProcessProvider` seam 도입

**의존 순서 명확화 (v2.2)**:
```
Phase 4-0 (seam 감사) 
  → Phase 3-3 step 1 (seam 도입, 리팩토링 없음)
    → Phase 4-5 step 1 (Golden fixture 기록)
      → Phase 3-3 step 2 (리팩토링)
        → Phase 4-5 step 2 (Golden 비교)
```

- [ ] Fixture: `go-cmp` + JSON snapshot
- [ ] 경로: `testdata/golden/*.json`
- [ ] 리팩토링 전 기록, 후 비교

### 4-6. fake_daemon 하네스 단위 테스트

**v2.2 의존 순서 명시**: Phase 4-6은 번호상 뒤이지만 **Phase 1-1 선행 필수**. 진행 순서에 명시.

시나리오 (6개 모드):
- [ ] `normal`, `crash`, `slow` (stdin 블록), `error`, `stderr-error`, `panic`

### Phase 4 체크리스트

- [ ] **4-0**: 선결 (goleak, CI, clock, coverage baseline Day 1 실측 기록, seam 감사)
- [ ] **4-1**: 단위 테스트
- [ ] **4-2**: goleak (8개)
- [ ] **4-3**: 플랫폼별 통합
- [ ] **4-4**: 벤치 (NtQuery + BufferedHTTP + KillPath + 의사결정 트리)
- [ ] **4-5**: Golden test (의존 순서 명시)
- [ ] **4-6** (Phase 1-1 선행): fake_daemon 하네스

---

## Phase 5 — 배포 & 모니터링

### 5-1. 카나리 (자동 Chain + 회로 차단기 연결)

6~9대 stratified.

**합격 조건 (72h)**:
- 재시작 0회
- SLO-1 ≤ baseline × [Phase A 확정 계수]
- SLO-2 RSS < 60MB, goroutine 상대 drift < ±10, ping ≥ 99%
- SLO-3 성공률 ≥ 99.5%, drop < 1/hr/PC

**자동 롤백 (Phase 2.5-3)**:
- 5분 내 N대 이상 임계 위반 → 회로 차단기 체크 → auto hold → 15분 무응답 auto rollback

### 5-2. 단계적 확장

1 → 10 → 100 → 1,000 → 전체. 각 48h.

### 5-3. 모니터링

SelfMetrics + 외부 ping 활성. Dead-man's switch 동작.

### 5-4. SLO 달성 (45일)

- 전체 SLO 45일 검증
- 롤백 카운트 월 사이트당 0.1% 이하
- baseline drift ±20% 이내
- 6개월마다 baseline 재측정

### 5-5. 장기 운영 정책

- 월 1회 재부팅 (생산 캘린더 조율)
- AV 리포트 (법무/보안 승인)
- baseline 6개월 재측정

### Phase 5 체크리스트

- [ ] **5-1**: 카나리 (회로 차단기 연결)
- [ ] **5-2**: 단계적 확장
- [ ] **5-3**: 모니터링 운영
- [ ] **5-4**: SLO 달성 (45일)
- [ ] **5-5**: 장기 정책

---

## 수정 대상 파일 요약 (v2.3 갱신)

| Phase | 파일 | 변경 유형 | 추정 인일 |
|-------|------|----------|---------|
| W0 | `docs/runbooks/infrastructure-audit.md` | 신규 | 1 |
| **~~0-1~~** | ~~Disk/StorageHealth DefaultConfig~~ | **삭제 (v2.3)** | ~~0.5~~ |
| 0-1 (구 0-2) | `lhm_provider_windows.go:396-403` | drainStderr H1 | 0.5 |
| 1-1-spike | `docs/designs/lhm-provider-deadline-redesign.md` + spike 브랜치 + PoC 결과 | 신규 설계+PoC (Hard Gate) | 3 |
| 1-1 | `lhm_provider_windows.go:98, 305, 434-481` | 필드 분리 + deadline (C1) | 2 |
| **1-2** | **`storage_health_windows.go:64-85`** | **WMI goroutine leak fix C2 (v2.3 신설)** | **2** |
| 2-1 (v2.3) | `config.go`, `loader.go`, `validate.go`, `defaults.go`, `kafkarest.go`, 샘플 JSON 2종 | MaxBufferedRecords + mu enforcement | 3 |
| 2.5-1 | `selfmetrics.go`, `runtime_stats.go` | 신규 + seam | 3 |
| 2.5-2 | `experimental.go` | 신규 feature flag | 1 |
| 2.5-3 | `docs/runbooks/resourceagent-rollback.md` | 신규 + 회로 차단기 | 2 |
| 2.5-4 | `install_ResourceAgent.bat`, ManagerAgent 계약 | `/rollback` + 원격 | 2 |
| 3-0 | (PoC 코드) | ETW/PerfLib v2/Job Object 검토 | 12~15 (E-Win-1 반영) |
| 3-1 | `process_nt_windows.go` | 신규 NtQuery (6건 주의사항) | 4 |
| 3-2 | `process_snapshot.go` | 신규 + singleflight + tuple + FILETIME | 2 |
| 3-3 | `cpu_process.go`, `memory_process.go`, `process_watch.go`, `process_provider.go` | 리팩토링 | 5 |
| 4-0 | `go.mod`, `scripts/ci.sh`, `scripts/ci.ps1`, `scripts/coverage.sh`, `scripts/coverage.ps1`, `docs/runbooks/ci-history.csv`, `docs/runbooks/ci-history-policy.md`, `docs/runbooks/goleak-whitelist.md`, `docs/runbooks/known-race-conditions.md`, `coverage-baseline.{txt,md}`, `internal/sender/{sender_test.go,clock.go,clock_self_test.go}`, `internal/collector/{collector_test.go,goleak_self_test.go}` | 선결 인프라 (v2.3.1) | 2 (Step 0 완료 — 실 측정) |
| 4-5 | `testdata/golden/*.json` | 신규 fixture | 2 |
| 4-6 | `testdata/fake_daemon_harness_test.go` | 신규 | 1 |
| **F** | **`docs/runbooks/av-mitigation-manual.md`** | **AV 원인 확정 시 대응 매뉴얼 (v2.3 신설)** | **2** |

**개발 합계 (Phase 5 제외): ~63 인일 (v2.3, Phase 3-0 12~15 인일 + 1-2 2 + F 2 - 0-0/0-1 2 = +12)**

**v2.3 변경분**:
- 제거: 0-0 Downstream 영향 조사 (-2), 0-1 DefaultConfig (-0.5)
- 신설: 1-2 WMI fix (+2), F AV 매뉴얼 (+2)
- 확장: 2-1 config 경로 (+1), 3-0 재산정 (E-Win-1 반영, +9~12)
- 합계: 약 ~55 → ~63 인일

---

## 재사용할 기존 패턴

(v2.1과 동일)

---

## 검증 절차 (end-to-end)

(v2.1과 동일)

---

## 진행 순서 권장 (v2.2 재조정)

1. **Week 0 (1주 선결)**: 인프라 실사 Spike + Phase A Kickoff + Phase 4-0 일부 (goleak, clock)
2. **Phase A (3~5주, 사용자 수행)** + **Phase 2.5-0 Kickoff (M1 목표 2주)** 병행
3. **Phase 0** 착수 (Phase 4-0 일부만 필요 — local 검증으로 0-2 선행 가능)
4. **Phase 4-0 완료 후**: Phase 1-1-spike (3일 timeboxing) → Phase 1-1
5. **Phase 2-1** (Phase 4-0 의존 — clock migration 완료 후)
6. **Phase 2.5** (2.5-0 M1 완료 후 나머지 착수)
7. **Phase 3-0 검토** (Phase A 결과 반영)
8. **Phase 3-1~3-3** (gopsutil seam 감사 → Golden 기록 → 리팩토링 → Golden 비교)
9. **Phase 4 전체 통합 검증**
10. **Phase 5** (SLO 기반 배포)

---

## Appendix A — v1 → v2 정정 사항 (1차)

(v2에서 이미 기록, v2.2에서 변경 없음)

1. WMI → 커널 Pool 주장 정정
2. `\\.\C:` 볼륨 open AV 미니필터 트리거 완화
3. IOCTL_STORAGE vs WMI 비교 주장 삭제
4. Phase 1-1 bufio.Reader race 오진 정정
5. Phase 1-1: Kill-on-timeout → SetReadDeadline 옵션 A 우선
6. Phase 1-2: Phase 1 필수 → Phase A 결과 후 Deferred
7. Phase 3 NtQuery: optional → 메인 목표
8. Phase 3 ProcessSnapshot: TTL only → singleflight 확정
9. Phase A-3: poolmon → RAMMap + fltmc + xperf
10. A-0, 0-0, 2.5, SLO, 4-0, 4-4, 4-5 신설

## Appendix B — v2 → v2.1 정정 사항 (2차)

(v2.1에서 이미 기록, v2.2에서 변경 없음)

1. RAMMap `-Rt` 옵션 존재하지 않음 → `-o snapshot.rmp`
2. Phase 3-5 ETW S.M.A.R.T 데이터 없음 → WMI/LhmHelper/포기 3안
3. Phase 1-1 옵션 B lock hold "수 ms" → 수십~수백 ms
4. Phase 3-1 NtQuery "Ob callback 완전 회피" → SSDT 후킹은 여전히 가능
5. 1-1-spike, 2.5-0, Dead-man's switch, bufferRecordCount atomic, feature flag semantic, 자동 트리거 Chain, Go 구현 주의사항 3건, PID/CreateTime tuple, ttl_epoch, gopsutil seam 감사, clock 확정, CI 구체, coverage, goleak 확장, KillPath 벤치, fake_daemon 하네스, SSDT/IRP 후킹, A-0 정량 임계, 5-4 추가 기준, baseline 재측정 주기, SLO-1 잠정 계수, 45일

## Appendix C — v2.1 → v2.2 정정 사항 (3차, v2.2 신설)

### 기술 주장 정정
1. **AhnLab V3 후킹 메커니즘**: v2.1의 SSDT 가설은 가능성 20~30%로 하향 → IRP/Ob callback이 주경로. Context에서 SSDT 의존도 완화, Phase A-0 진단 초점 조정.
2. **LhmHelper S.M.A.R.T 대체의 AV 이득**: LHM도 `\\.\PhysicalDriveN` IOCTL 사용 → raw device open 경로 동일. "AV 관점에서 동등/악화 가능"으로 정정 (Phase 3-5).
3. **WMI 재수용 "5분 → 1시간"**: 누수 해소가 아닌 **지연** (1/12 감속)임을 명시 (Phase 3-5).
4. **PoolMon 원격**: `-s`가 원격 옵션이 아님. PsExec으로 poolmon.exe 임시 복사 방식으로 정정 (Phase A-3).
5. **RAMMap CLI EULA**: `/accepteula` 누락 정정 (Phase A-1).
6. **`\Memory\Pool Paged Bytes` Go 읽기**: 표준 라이브러리 없음 → `lxn/win` 또는 `pdh.dll` syscall 경유 명시 (Phase 2.5-1).

### 전략 변경
7. **Week 0 인프라 실사 Spike 신설**: Alerting/워크플로우 엔진 존재 여부 + RACI 실명 확정 + GitHub Actions 조직 승인 + FTP 스토리지 용량. Phase 본 착수 전 1주 선결.
8. **Phase 1-1-spike 확장**:
   - stdin `SetWriteDeadline` 추가 (stdout 외 write block도 방어)
   - startup `doRequest` 경로 통합 설계
   - 산출물 명시: 설계 문서 + PoC 브랜치
   - Timeboxing 룰 (3일 상한)
9. **옵션 B 벤치 결과 의사결정 트리**: lock hold 시간별 후속 조치 4단계 (< 100ms / 100~300ms / 300ms~1s / > 1s)
10. **Phase 2-1 atomic 카운터 구현 계약**: `bufferCount atomic.Int64` Deliver/flush/dropOldest 3곳 일관 관리 의사코드 명시
11. **Phase 2.5-1 `RuntimeStatsProvider` seam**: interface + mock 주입으로 SelfMetrics tautology 방어
12. **SLO-2 상대 drift 계산**: 절대값 대신 baseline 60초 평균 대비 drift (goroutine flaky 방어)
13. **Phase 2.5-3 자동 롤백 회로 차단기**: 5% 상한 + 30분 cooldown + sitewide lock + kill-switch
14. **Phase 2.5-1 Ping 실패 원인 구분**: 사이트 단위 집계로 네트워크 vs ResourceAgent 판정
15. **Phase 3-0 대체 접근 검토 신설**: ETW consumer API / PerfLib v2 / Job Object / SystemProcessorPerformanceInformation
16. **Phase 3-1 Go 구현 6건 확장**: 32/64 포인터 분기 + UNICODE_STRING + alignment 추가
17. **Phase 3-2 FILETIME → Unix ms 정규화 레이어**: gopsutil Unix ms와 NtQuery FILETIME 단위 차이 해결
18. **Phase 4-0 Coverage baseline Day 1 실측치 기록**: "측정해야 함"에서 "Day 1에 실제 수치를 플랜에 직접 기록"으로 강화
19. **Phase 4-5 의존 순서 명확화**: Phase 3-3 step 1 (seam 도입) → 4-5 step 1 (Golden 기록) → 3-3 step 2 (리팩토링) → 4-5 step 2 (비교)
20. **Phase 0-0 100대 수집 타임라인**: 현장 수기 시 2주 / ManagerAgent M1 후 1일
21. **FTP 스토리지 용량 계산**: Week 0 W0-5에서 산정

### 테스트 보강
22. **Phase 2-1 Race 테스트 명시**: 동시 Deliver+flush → bufferCount 일치 검증
23. **Phase 3-2 Race 테스트 명시**: singleflight 동시 100 호출, tuple stale 감지, ttl_epoch 경계
24. **Phase 2.5-1 tautology 방어 테스트**: seam mock 주입으로 goroutine drift 시나리오 재현

### 작업량 추정 (Appendix D)
25. **수정 대상 파일 요약 표에 인일 컬럼 추가**

### 리뷰 수행 이력
- 3회차 전문가 교차 검증 (2026-04-22): Go 78/100, Windows 8.2/10, SRE 8.6/10, QA 9/10, 평균 ~8.5/10

### v2.2 착수 시점의 알려진 제약
- Week 0 결과에 따라 Phase 2.5-3 범위 크게 확대 가능
- Phase 3-0 ETW consumer PoC 실패 시 Phase 3 전략 재조정
- ManagerAgent 계약 M4 지연 시 Phase 5 진입 지연
- SelfMetrics `paged_pool_bytes`의 Windows perfmon 라이브러리 선택이 `lxn/win` vs 직접 syscall 중 구현 시 확정

---

## Appendix D — 작업량 추정 (v2.3 갱신)

**단위**: 인일 (person-day)

| Phase | 항목 | v2.2 | **v2.3** | 비고 |
|-------|------|------|---------|------|
| W0 | 인프라 실사 Spike | 5 | 5 | 1주 |
| A | 현장 진단 (사용자 수행) | 25 | **28** | A-4 5대 → 8대 (LHM-all-off + StorageSmart-only + heartbeat-only) |
| ~~0-0~~ | ~~Downstream 영향 조사~~ | ~~2~~ | **0 (삭제)** | 배포 설정 변경 안 함 |
| ~~0-1~~ | ~~Disk/StorageHealth disabled~~ | ~~0.5~~ | **0 (삭제)** | 증상 감추기 |
| 0-1 (구 0-2) | drainStderr | 0.5 | 0.5 | iotest 기반 |
| 1-1-spike | bufio.Reader 해체 + stdin + **PoC Hard Gate** | 3 | 3 | timeboxing 상한 |
| 1-1 | doRequestWithTimeout 수정 (C1) | 2 | 2 | goleak 포함 |
| **1-2** | **WMI goroutine leak 코드 수정 (C2)** | ~~0~~ | **2** | **v2.3 신설**, 옵션 C 권장 |
| 2-1 | BufferedHTTPTransport 상한 + **mu enforcement** | 2 | **3** | config 경로 확장 (loader/validate/샘플 JSON) |
| 2.5-0 | ManagerAgent Kickoff | 2 | 2 | M2~M4는 ManagerAgent 팀 |
| 2.5-1 | SelfMetrics + seam + dead-man's | 3 | 3 | pdh.dll 포함 |
| 2.5-2 | Feature flag | 1 | 1 | |
| 2.5-3 | 롤백 Runbook + 회로 차단기 | 2 | 2 | |
| 2.5-4 | ManagerAgent 원격 명령 | 2 | 2 | |
| 2.5-5 | 로그 중앙 수집 | 1 | 1 | |
| 3-0 | 대체 접근 검토 PoC | 3 | **12~15** | E-Win-1 반영 |
| 3-1 | NtQuery (6건 주의) | 4 | 4 | |
| 3-2 | ProcessSnapshot + tuple + FILETIME | 2 | 2 | |
| 3-3 | 세 collector 전환 | 5 | 5 | |
| 3-4 | (conditional) Disk PDH | 2 | 2 | |
| 3-5 | (conditional) StorageHealth 3안 | 2 | 2 | |
| 4-0 | 선결 (goleak/CI/clock/coverage) | 3 | 3 | |
| 4-2 | goleak 8개 대상 | 2 | 2 | |
| 4-3 | 플랫폼별 통합 | 2 | 2 | |
| 4-4 | 벤치마크 3종 | 2 | 2 | |
| 4-5 | Golden test | 2 | 2 | |
| 4-6 | fake_daemon 하네스 | 1 | 1 | |
| **F** | **AV 원인 대응 매뉴얼 (v2.3)** | ~~0~~ | **2** | **v2.3 신설** |
| 5 | 배포 & 모니터링 | 45 | 45 | 45일 SLO 관찰 |

**개발 총합 (Phase 5 제외)**:
- v2.2: ~55 인일
- **v2.3: ~63 인일** (+8 = +1-2(2) + 2-1(1) + 3-0(9~12) + F(2) - 0-0(2) - 0-1(0.5) - A 보정(0))

**Phase 5 포함 총 기간 (병렬 가능 감안)**:
- v2.2: ~20~24주
- **v2.3: ~22~26주 (5.5~6.5개월)**

**주요 병렬 가능 구간**:
- Week 0 + Phase A 초반 + Phase 0: 1~2주 내 병렬
- Phase 2.5-0 M1~M4 (ManagerAgent 측): 4~8주, Phase 1~2와 병렬
- Phase 4-0 선결 + Phase 0~2: 일부 병렬

**리소스 권장**: 개발 2인 + SRE 1인 + 사용자(현장 진단) + ManagerAgent 팀 1인 협업 시 **6개월 내 Phase 5 진입** 목표 가능.

### 여전히 남아있는 리스크 (v2.2)
- Week 0 결과가 "Alerting 없음" + "워크플로우 엔진 없음"이면 Phase 2.5-3 작업량 **2~3배 증가 가능**
- Phase 3-0 ETW consumer PoC 성공 시 Phase 3-1의 NtQuery 우선순위 하향 가능 (비용 절감)
- ManagerAgent 팀의 M1~M4 마일스톤 리소스 확보 실패 시 Phase 5 지연
- SLO-1 최종 계수가 Phase A에서 확정되지 않으면 Phase 5 종결 판단 지연

---

## Appendix E — v2.2 4차 최종 검증 결과 (2026-04-24)

**총 4회차에 걸친 전문가 교차 검증 종료. 네 전문가 합의: 실행 착수 가능 (GO 판정).**

### 점수 추이

| 전문가 | v2 | v2.1 | **v2.2** | Δ |
|-------|----|----|---------|---|
| Go 동시성 | 65 | 78 | **84/100** | +6 |
| Windows 커널 | 6.0 | 8.2 | **8.8/10** | +0.6 |
| SRE | 8.2 | 8.6 | **9.0/10** | +0.4 |
| QA | 8.0 | 9.0 | **9.5/10** | +0.5 |
| **평균** | 7.1 | 8.5 | **~9.1/10** | +0.6 |

### 4회차 전문가 최종 판정

- **Go 동시성**: "실행 착수 가능. Diminishing returns 진입. 남은 4건은 구현 단계 코드 리뷰로 잡힐 수준. Week 0 + Phase A 즉시 Kickoff 권장."
- **Windows 커널**: "GO 판정. 3개 minor blocker는 Week 0 병렬 해결 가능. Phase A 착수 차단 요인 아님. Phase A 실측 데이터 없이 더 이상의 planning iteration은 diminishing returns."
- **SRE**: "95% 준비 완료. v2.1 대비 diminishing returns 명확. 문서 추가보다 실행이 더 가치. Week 0 Kickoff가 최적의 다음 액션."
- **QA**: "실행 착수 gate 통과. 추가 iteration ROI 낮음. 4건은 실행 중 첫 주에 PR 리뷰에서 보강 가능."

### 실행 중 해결 TODO (12건)

각 항목은 **실행 중 PR 리뷰 또는 Week 0 내부에서 1시간 이내 해결 가능한 minor 수준**. 플랜 재작성 없이 실행과 병행 처리.

#### Go 동시성 (4건)

- [ ] **E-Go-1**: Phase 2-1의 atomic 카운터 `Store(0)` → `Add(-flushed)` 로 변경
  - 이유: flush와 Deliver 동시 발생 시 `Store(0)`이 Deliver의 `Add(...)`를 덮어써 counter underflow 가능
  - 조치: 구현 PR에서 의사코드 수정 + race 테스트로 검증
- [ ] **E-Go-2**: SLO-2 baseline 측정 기간 60초 → **5분 또는 GC 2회 완료 후**로 연장
  - 이유: Go GC 기본 주기 2분 감안 시 60초는 초기 spike가 baseline에 포함되어 왜곡
  - 조치: Phase 2.5-1 SelfMetrics 구현 시 baseline 계산 로직 설정 확정
- [ ] **E-Go-3**: `RuntimeStatsProvider` Windows-only 메서드 **build tag 분기** 명시
  - 파일 분할: `runtime_stats_windows.go` + `runtime_stats_other.go` (후자는 stub return `(0, errNotSupported)`)
  - 조치: Phase 2.5-1 구현 PR에서 처리
- [ ] **E-Go-4**: `pdh.dll` 단일 thread 제약 명시 + collector 내부 mutex 추가
  - 이유: `PdhOpenQuery` 핸들은 thread-safe하지 않음
  - 조치: Phase 2.5-1 SelfMetrics의 `paged_pool_bytes` 수집 시 mutex 필수

#### Windows 커널 (4건)

- [ ] **E-Win-1**: **Phase 3-0 인일 재산정**: 3 → 12~15 인일
  - 이유: ETW consumer API 하나만 해도 `OpenTrace` + `ProcessTrace` + 매니페스트 파싱으로 최소 5~7 인일
  - 조치: Phase 3-0 Kickoff 시 Appendix D 업데이트
- [ ] **E-Win-2**: FILETIME 상수 네이밍 혼란 제거
  - 현 상수 `FiletimeToUnixMs = 116444736000000`의 단위 주석 추가 (현재 수식은 정확하나 이름이 단위 혼란)
  - 또는 상수를 `FiletimeUnixEpochDelta100nsTicks = 116444736000000000` 로 개명 후 수식 재작성
  - 조치: Phase 3-2 구현 PR
- [ ] **E-Win-3**: Phase A-4 병렬 실험에 **"LhmHelper 비활성화" 6번째 PC 추가**
  - 이유: PawnIO 드라이버 + LhmHelper 상주가 Paged Pool 사용 기여 가능성
  - 조치: Phase A Kickoff 시 PC 6대로 확장
- [ ] **E-Win-4**: Phase A-0 SSDT 진단 절차 보강
  - WinDbg `kd -kl`은 `bcdedit /debug on` + `bcdedit /dbgsettings LOCAL` 후 재부팅 필요
  - GMER/PCHunter 다운로드 URL + 라이선스 + 사용 절차 `infrastructure-audit.md`에 포함
  - 조치: Week 0 Day 1~2에 Phase A 가이드 확장

#### SRE (3건)

- [ ] **E-SRE-1**: **회로 차단기 + sitewide lock 통합 시퀀스 다이어그램** 작성
  - 체크 순서 명시: ① sitewide lock → ② cooldown → ③ 5% rate → ④ kill-switch
  - 조치: Week 0 Day 1에 `resourceagent-rollback.md`에 다이어그램 포함
- [ ] **E-SRE-2**: **Ping 원인 구분 구현 위치 + 사이트 경계 정의**
  - 잠정 기본안: 사이트 집계 로직은 ManagerAgent 워크플로우 엔진에 구현 (없으면 Kafka consumer 측)
  - 사이트 경계: **EQP_INFO.site 필드 기반** (ResourceAgent가 이미 Redis 조회 중)
  - 조치: Week 0 W0-2 Alerting/워크플로우 엔진 실사 결과와 연동하여 확정
- [ ] **E-SRE-3**: 회로 차단기 카나리 구간 **"5% OR 50대 중 작은 값"** 절대치 병기
  - 이유: 카나리 6~9대 구간 5%=불가, 100대 단계 5%=5대. 단계별 scaling 필요
  - 조치: `resourceagent-rollback.md`에 단계별 표로 기재

#### QA (1건)

- [ ] **E-QA-1**: SLO-2 baseline 확립 전 drift 메트릭 emit 정책
  - 잠정 기본안: `baseline_ready: false` 필드 추가 + `goroutine_drift` 메트릭은 emit 생략
  - Alerting 측에서 missing data 처리
  - 조치: Phase 2.5-1 구현 시 결정

### 실행 착수 결정

**Week 0 (인프라 실사) + Phase A (현장 진단) + Phase 4-0 (goleak/clock/CI 선결) + Phase 0 (저위험 완화)** 병렬 Kickoff 권장.

- **착수 시작일**: 2026-04-24 (문서 확정 즉시)
- **Week 0 완료 목표**: 2026-05-01 (1주)
- **Phase A 완료 목표**: 2026-05-29 (3~5주, 표본 18대 + 병렬 실험)
- **Phase 5 진입 목표**: 2026-10 ~ 2026-11 (약 6개월)

### Diminishing Returns 도달 근거

1. 4회차에서 발견된 12건이 모두 **실행 중 1시간 이내에 PR 리뷰로 해결 가능한 minor 수준**
2. 전략/구조적 결함은 v2.2에서 모두 해소
3. 전문가 4명 전원이 명시적으로 "diminishing returns" 또는 "추가 iteration ROI 낮음" 언급
4. 평균 9.1/10은 대규모 프로덕션 플랜 기준 "실행 준비 완료" 임계 초과
5. **Phase A 실측 데이터 없이는 더 이상의 planning 개선 어려움** (Windows 전문가 지적)

### 최종 공식 판정

**v2.2는 실행 착수 gate 통과. 추가 planning iteration 불필요. Week 0 Kickoff 개시.**

---

---

## Appendix F — AV 원인 확정 시 사용자 대응 매뉴얼 (v2.3 신설)

**전제**: Phase A 결과 1순위 가설(AV 미니필터/Ob callback 누적)이 확정된 경우. 코드 레벨로 100% 회피 못 하는 부분에 대한 운영 매뉴얼.

**목표**: 사용자(운영팀/사이트 관리자)가 이 문서 한 장으로 다음 결정을 내릴 수 있게 함:
- AhnLab 예외 등록 신청 여부
- 대체 API 평가 우선순위
- 수집기 삭제(코드 제거) 결정 근거

### F-1. AhnLab 예외 등록 절차 (사이트 관리자 제출용)

**제출처**: AhnLab 보안관제팀 (사이트별 담당)

**제출 양식 (`docs/runbooks/ahnlab-exclusion-request.md` 별도 작성)**:

1. **대상 프로세스/경로**:
   ```
   프로세스명: ResourceAgent.exe
   설치 경로: D:\EARS\EEGAgent\bin\x86\ResourceAgent.exe
   서명: [코드 서명 인증서 SHA256]
   해시: [SHA256, 버전별 갱신]
   ```

2. **예외 등록 요청 범위 (3안)**:
   - **A안 — 프로세스 신뢰**: ResourceAgent.exe를 V3 신뢰 프로세스로 등록 (가장 강한 권한)
   - **B안 — 폴더 스캔 제외**: `D:\EARS\EEGAgent\` 폴더 스캔 제외 (제한적)
   - **C안 — 특정 동작 화이트리스트**: WMI 쿼리, raw device open, 프로세스 enum 등 특정 호출만 제외 (가장 약한 권한, AhnLab 지원 불확실)
   
   **우선순위**: A안 > B안 > C안. A안 거절 시 B안.

3. **요청 근거**:
   - Phase A 진단 보고서 첨부 (`docs/issues/win7-kernel-pool-growth.md`)
   - 메모리 증가량 측정 데이터 (RAMMap snapshot, typeperf log)
   - 영향 PC 수 (예: 사이트당 수십대~수백대)
   - 운영 영향 (Win7 PC 메모리 부족으로 생산 라인 중단 가능성)

4. **컴플라이언스/보안 검토 포인트** (AhnLab 제출 전 사내 보안팀 사전 검토):
   - ResourceAgent의 시스템 API 호출 목록 + 의도
   - 코드 서명 키 보관 정책
   - 예외 등록이 다른 위협 벡터에 미치는 영향

**예상 처리 기간**: 2~4주 (사이트별 정책 + AhnLab 응답 시간)

**거절 시 fallback**: Phase 3-0 대체 API + Phase 3-5 기능 포기 결정 진행

### F-2. 대체 API 평가 의사결정 트리

Phase 3-0 PoC 결과 기반:

```
┌─────────────────────────────────────────────┐
│ Phase A 결과: 어떤 API가 누적 주범인가?     │
└──────────────────┬──────────────────────────┘
                   │
          ┌────────┴───────┐
          │                │
   gopsutil 프로세스    raw device open    
   enumeration 주범     (Disk/StorageSmart)
          │                │
          ▼                ▼
   3-1 NtQuery PoC    3-4 PDH 기반 Disk PoC
   효과 ≥30%?         IOCTL 회피 가능?
   │       │              │      │
   YES     NO            YES    NO
   │       │              │      │
   ▼       ▼              ▼      ▼
 Phase 3-1  WMI 재수용+   Phase  StorageHealth/
 구현       빈도감소      3-4    StorageSmart
            (Phase 3-5A)  구현    수집기 삭제
                                 결정 (F-3)
```

**평가 기준**:
- 효과 ≥30%: AV 미니필터 알람 카운트 또는 RAMMap Pool delta 30% 이상 감소
- 구현 비용: 6 인일 이하 (E-Win-1로 상향된 12~15는 ETW consumer 한정)
- 배포 안정성: PoC 시점에서 64-bit + 32-bit Win7 모두 통과

### F-3. 수집기 삭제 결정 기준 (v2.3 신설)

**삭제 후보 수집기**: StorageHealth, StorageSmart, Disk (Phase A에서 주범으로 확정 시)

**삭제 결정 체크리스트**:
- [ ] **운영 가치 평가**: 해당 메트릭이 실제로 알람/대시보드에서 사용 중인가?
  - 사용 중 → 삭제 X. 대체 API 추진 (F-2)
  - 미사용 또는 보고만 받음 → 삭제 후보
- [ ] **후단 파이프라인 의존성**: Kafka consumer 측에서 해당 메트릭에 의존하는 처리 있나?
  - 있음 → 1주 grace period 후 단계적 삭제
  - 없음 → 즉시 삭제 가능
- [ ] **법규/감사 요구**: 디스크 헬스 모니터링이 컴플라이언스 요구사항?
  - 요구사항 → 삭제 X. 대체 API 또는 수동 점검 절차
  - 자율 도구 → 삭제 가능
- [ ] **대체 가능성**: ARSAgent 또는 다른 모니터링 도구가 같은 정보를 수집 중?
  - 중복 → 삭제 권장
  - 유일 → 삭제 영향 큼

**삭제 절차** (모든 체크 통과 시):
1. PR로 collector 코드 + registry 등록 + DefaultConfig 항목 + 샘플 JSON 항목 일괄 제거
2. 단위 테스트 + golden test 갱신
3. 후단 파이프라인 통보 (1주 grace)
4. CHANGELOG에 명시 + 배포

**중요 (v2.3 원칙)**: "코드는 살려두고 설정만 disable" 은 v2.3 원칙 위배. 안 쓸 거면 코드에서 삭제. 진단/디버깅용으로 남겨야 한다면 별도 build tag로 분리.

### F-4. 모니터링 — AV 영향 정량 추적 (사용자 자가 점검)

**월간 자가 점검 체크리스트**:
- [ ] 각 사이트 대표 PC 1대씩 RAMMap snapshot (`rammap.exe -accepteula -o monthly_<site>_<date>.rmp`)
- [ ] AhnLab 미니필터 altitude/active 상태 (`fltmc instances`)
- [ ] AhnLab 버전 + 패턴 DB 버전 기록
- [ ] 메모리 증가량 시계열 그래프 (Grafana 대시보드 — 2.5-1 SelfMetrics + 외부 typeperf 이중 소스)

**임계 트리거**:
- Pool 증가량이 baseline의 2배 초과 → AhnLab 패턴 업데이트 영향 의심. 보안팀 통지.
- 동일 사이트 다른 PC들과 deviation > 30% → 해당 PC만의 환경 요인. 개별 진단.

### F-5. 산출물 요약

이 매뉴얼이 다루는 산출물:
- `docs/runbooks/av-mitigation-manual.md` — 본 Appendix F를 운영 가이드로 외부화
- `docs/runbooks/ahnlab-exclusion-request.md` — AhnLab 제출 양식 템플릿
- `docs/runbooks/collector-deletion-checklist.md` — 수집기 삭제 결정 체크리스트
- 월간 자가 점검 표 (Excel/Markdown 템플릿)

---

## Appendix G — v2.2 → v2.3 정정 사항 (5차, Codex 교차 검증)

**검증일**: 2026-04-26

**검증 범위**: v2.2 + Appendix E 전체 + 실제 코드/설정 파일 fact-check

### Codex 8개 지적 분류

| # | 지적 | 분류 | v2.3 처리 |
|---|------|------|----------|
| C-1 | Phase 0의 `DefaultConfig().Enabled=false`만으로는 배포 PC에 무효 (Monitor.json은 명시적 true) | **거부** (증상 감추기 원칙 위배) | Phase 0의 0-0/0-1 자체를 삭제. 배포 설정은 변경 안 함 |
| C-2 | Phase 0에 StorageSmart 누락 (LHM IOCTL 경로) | **재해석** (배포 off 거부, 진단 매트릭스로) | Phase A-4 매트릭스에 StorageSmart-only-off variant 추가 |
| H-1 | C2(WMI goroutine leak)를 Phase A 결과 대기로 미루는 건 위험 | **수용 (재정의)** | Phase 1-2를 코드 수정으로 변경 (수집기는 유지, hang만 차단) |
| H-2 | Phase A 매트릭스가 LHM 공유 의존성 미고려 (개별 off로 분리 불가) | **수용** | A-4에 LHM-all-off (PC-A4-6, E-Win-3 통합) + heartbeat-only (PC-A4-8) 추가 |
| H-3 | C1 PoC Hard Gate 명시 (Win7 x86/x64 + 서비스 계정 + hang 모드) | **수용** | Phase 1-1-spike에 5개 합격 조건 명시 + 검증 산출물 정의 |
| M-1 | BufferedHTTPTransport atomic 카운터가 race-prone | **수용** | enforcement는 mu critical section 내부, atomic은 SelfMetrics 관측용으로만 (`bufferCountObs`) |
| M-2 | MaxBufferedRecords config 경로 부족 | **수용** | loader/validate/defaults/샘플 JSON 2종 모두 수정 대상에 추가 |
| M-3 | SLO-1 source of truth가 SelfMetrics인 것은 약함 | **수용 (이중화)** | 외부 typeperf/RAMMap snapshot = 판정 source, SelfMetrics = 고빈도 보조 |
| L-1 | 인일 추정 불일치 (~37 vs ~55) | **수용 (정정)** | "수정 대상 파일 표 (~63)"는 코드 + 문서 변경 + Appendix F. Phase 5 별도. v2.3에서 합계 명확 표기 |

### 핵심 원칙 재확립 (v2.3)

**"증상 감추기 금지"**:
- 수집기를 **영구 disable**하는 것 = 원인 미해결 + 진단 능력 손실. v2.3에서 거부.
- 안 쓸 거면 **삭제** (Appendix F-3 결정 체크리스트 통과 후)
- Phase A 진단 중 **일시 off**는 변수 분리 목적이며 24h 후 자동 원복

**책임 분리**:
- 코드 버그(C1/C2/H1/H2/M-1/M-2) → 코드 수정으로 해결
- 외부 요인(AV 미니필터/Ob callback) → 진단 절차 + 운영 매뉴얼(Appendix F)
- 후자는 코드로 100% 못 막음. 사용자(운영팀)가 보안팀/AhnLab과 협의해 결정.

### v2.3 변경 요약 (정량)

| 영역 | v2.2 | v2.3 | Δ |
|------|------|------|---|
| Phase 0 항목 수 | 3 (0-0, 0-1, 0-2) | 1 (0-1 drainStderr만) | -2 |
| Phase A-4 PC 수 | 5 | 8 | +3 |
| Phase 1-2 상태 | 결과 대기 | 코드 수정 | 작업 추가 |
| 인일 추정 | ~55 | ~63 | +8 |
| Appendix 수 | 5 (A-E) | 7 (A-G) | +2 (F, G) |
| 수정 대상 파일 표 row 수 | 17 | 19 | +2 |
| 체크박스 수 | 193 | ~210 (예상) | +17 |

### 미해결 (v2.3 착수 시점)

- 옵션 C (WMI goroutine leak fix) 의 stale-cache 윈도우 크기 — 구현 PR에서 확정
- AhnLab 예외 등록 신청 시점 — Phase A 결과 후 보안팀과 협의
- 수집기 삭제(F-3) 결정은 Phase A + 후단 파이프라인 협의 완료 후

### 5차 검증 결과 종합 판정

**Codex 지적 8건 중**:
- 수용: 5건 (H-1, H-2, H-3, M-1, M-2)
- 재해석/이중화: 2건 (C-2, M-3)
- 거부 (원칙 충돌): 1건 (C-1) — 단, "배포 영구 변경" 부분만 거부. 그 외 사실관계는 모두 정확
- 정정: 1건 (L-1)

**v2.3 실행 가능성**: GO. v2.2 대비 작업량 +8 인일이지만 **현장 진단 변별력 + 운영 가이드 완결성**이 크게 향상.

**다음 단계**: Week 0 + Phase A + Phase 4-0 + Phase 0(축소판) 병렬 Kickoff. v2.2의 GO 판정은 v2.3에서도 유효.

### G.5 — GitHub Actions 미사용 환경 반영 (v2.3.1, 2026-04-26)

**상황**: 사용자 환경에서 GitHub Actions 사용 불가 확정. 대안 검토 결과 git pre-push hook 도입 부적절(우회 가능 + 마찰 + 정책 중복) → **로컬 스크립트 + CSV 이력**으로 통일.

**v2.3.1 변경 항목**:

1. **W0-4 재정의**: GitHub Actions 조직 승인 → 사내 CI 인프라 조사 (Jenkins/GitLab CI/기타). 부재 시 로컬 스크립트 운영.

2. **Phase 4-0 재구성**:
   - `.github/workflows/ci.yml` 매트릭스 → `scripts/ci.sh` + `scripts/ci.ps1`
   - PR 본문 첨부 정책 → `docs/runbooks/ci-history.csv` 자동 append + `docs/runbooks/ci-history-policy.md`
   - Windows runner 비용 항목 삭제

3. **ci.sh 모드 분리** (Step 0 발견된 race condition 대응):
   - `basic`: gofmt + vet + test + goleak (PASS 보장, race 없이)
   - `--full`: 위 + race + cross-build (강한 검증, 알려진 race fix 후 default 승격 검토)

4. **R-1 race condition 발견** (Step 0 race detector 첫 사례):
   - `internal/sender/file.go:78` `drainConsole` goroutine vs 테스트 `os.Stdout` swap
   - 운영 영향 없음 (테스트 한정), 별도 fix PR 추적
   - `docs/runbooks/known-race-conditions.md`

5. **Coverage baseline 실측** (Day 1, 2026-04-26):
   - macOS 측정 (Windows-only 코드 제외): **Overall 58.5%**
   - 패키지별: collector 46.4%, sender 78.9%, config 64.6%, scheduler 92.3%, heartbeat 92.5%, timediff 89.9% (function avg)
   - Phase별 게이트 확정: Phase 2 후 60.5%+, Phase 3 후 63.5%+, 절대 하한 70%
   - `docs/plans/coverage-baseline.{txt,md}` 생성

6. **수정 대상 파일 표 갱신**: Phase 4-0 row 확장 (스크립트 + 정책 + 자기검증 테스트 + 산출물)

**Step 0 PR 결과**:
- Step 0.1 ~ 0.5 완료 (Step 0 자체)
- ci.sh 첫 PASS 행 CSV 기록 완료 (`tail -1 docs/runbooks/ci-history.csv`)
- goleak 자기검증 (`VERIFY_GOLEAK_INFRA=1`) — 의도적 leak 검출 확인
- clock 자기검증 (`TestClockMockAdvancesTime`) — mock 시간 진행 확인
- cross-build (windows amd64/386, linux amd64) 통과
- gofmt 미준수 12파일 자동 정리 (코드 동작 영향 없음)

---

## 문서 이력 종합

| 버전 | 날짜 | 점수 평균 | 주요 변경 |
|------|------|---------|----------|
| v1 | 2026-04-22 | - | 초기 계획 (Phase A~5 + SLO + Appendix A~D 구조 초안) |
| v2 | 2026-04-22 | 7.1/10 | Appendix A 반영 (16건 정정) |
| v2.1 | 2026-04-22 | 8.5/10 | Appendix B 반영 (30건 정정) |
| v2.2 | 2026-04-22 | **9.1/10** | Appendix C 반영 (25건 정정) + Appendix D 신설 (작업량) |
| v2.2 | 2026-04-24 | 9.1/10 | Appendix E 추가 — 4차 검증 GO 판정, 12건 TODO |
| v2.3 | 2026-04-26 | (Codex 5차) | Appendix F (AV 운영 매뉴얼) + Appendix G (Codex 검증) — 증상 감추기 제거 원칙 확립 |
| **v2.3.1** | **2026-04-26** | (Step 0 실측) | **G.5 — GitHub Actions 미사용 환경 반영. ci.sh + CSV 이력. coverage baseline 58.5%. R-1 race condition 발견.** |

리뷰 라운드 총 5회 × 전문가 5명(Go/Windows/SRE/QA/Codex) = 21건의 교차 검증 의견 반영. Step 0 PR로 인프라 도입 완료.
