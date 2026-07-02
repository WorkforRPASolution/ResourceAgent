# ResourceAgent + EPS 환경 메모리 사용량 증가 현상 공유

> EPS 운영 담당자 협의용 참고 자료
> 작성일: 2026-04-28
> **갱신 (2026-04-29): EPS 화이트리스트 등록으로 메모리 증가 해소 확인됨 (H-EPS-3 가설 확정)**

---

## 처리 결과 (2026-04-29 갱신)

✅ **해결됨** — EPS 담당자와 협의하여 ResourceAgent 설치 폴더를 화이트리스트에 등록한 결과, 메모리 증가 현상이 해소되었습니다.

이로써 본 자료의 **H-EPS-3 가설 (Application Control / 행위 정책 캐시)** 이 데이터로 확정되었습니다 (가설 신뢰도 70% → 95%+).

본 문서는 향후 동일/유사 현상 진단 시 참고용 케이스 자료로 보존합니다.

---

## 한 줄 요약

EPS 가 설치된 PC 에서 ResourceAgent 를 실행하면 시스템 메모리 사용량이 천천히 증가하는 현상이 관찰되어, **ResourceAgent.exe 를 EPS 의 신뢰 SW 목록(White list)에 등록**해 주시기를 부탁드리는 자료입니다.

---

## 무슨 일이 일어났나요?

### 관찰된 현상
- ResourceAgent 를 실행하면 시스템 메모리(커널 영역)가 **약 1시간에 걸쳐 점진적으로 증가**합니다.
- 일정 시점이 지나면 더 이상 증가하지 않고 **정체**됩니다 (계속 무한히 늘어나는 건 아님).
- 증가량은 **약 800MB ~ 1GB** 수준입니다.

### 여러 PC 에서 비교 실험한 결과

| PC 환경 | 메모리 증가 |
|---------|-----------|
| EPS **있음** + ResourceAgent 실행 (모든 기능 ON) | 증가함 ⚠️ |
| EPS **있음** + ResourceAgent 실행 (모든 기능 OFF, 거의 idle) | 증가함 ⚠️ |
| EPS **없음** + ResourceAgent 실행 (모든 기능 ON) | **증가 없음** ✅ |
| EPS **없음** + ResourceAgent 미실행 | 증가 없음 |

→ **EPS 가 설치된 환경에서만 메모리 증가가 일어납니다.** ResourceAgent 자체의 동작량과는 무관했습니다.

---

## 왜 이런 현상이 일어나나요? (간단한 설명)

EPS 는 보안 SW 의 특성상 **알려지지 않은(미등록) 프로그램**의 행동을 하나하나 살피며 정책 검사 결과를 메모해 둡니다.

ResourceAgent 는 EPS 입장에서 새로운 프로그램이라, 실행되는 동안:
- 파일을 열거나 닫을 때마다
- 설정값을 읽을 때마다
- 새 동작을 할 때마다

EPS 가 그때그때 **"이 프로그램이 이런 행동을 했음"** 을 기록합니다. 시간이 지날수록 이 기록이 쌓이며 메모리를 차지하다가, 어느 정도 차면 더 이상 안 늘어나는 것으로 보입니다.

비유하자면, 새로운 직원이 사무실에 왔을 때 보안담당자가 그 사람이 어디 가고 뭘 만지는지 일일이 메모하는 상황입니다. 반면 **신뢰 등록된 사람**이라면 일일이 메모하지 않습니다.

→ ResourceAgent 의 코드 문제가 아니라, **EPS 가 ResourceAgent 를 모르는 SW 로 분류**해서 발생하는 현상입니다.

---

## 부탁드리는 조치

### **ResourceAgent 설치 폴더를 EPS 의 신뢰 SW 목록 (White list) 에 등록**

#### 등록 방식 비교 (효과 차이)

EPS 화이트리스트는 **등록 방식**에 따라 적용 범위가 다릅니다. 본 케이스는 ResourceAgent 가 자식 프로세스(LhmHelper.exe) 를 실행하고 자체 설정/로그 파일에 지속 접근하는 구조라, **폴더 단위 등록**이 가장 효과적입니다.

| 등록 방식 | 예시 | 본 메모리 증가 현상 차단 효과 |
|---------|------|--------------------------|
| ① 프로세스 이름만 | `ResourceAgent.exe` | **부분적 + 보안 위험** — 같은 이름의 다른 .exe 도 화이트가 됨. Microsoft 도 권고 안 함 |
| ② 단일 파일 전체 경로 | `D:\EARS\EEGAgent\bin\x86\ResourceAgent.exe` | **부분적** — ResourceAgent.exe 자체만 제외. 자식 프로세스(LhmHelper.exe) 와 설정/로그 파일 추적 잔존 |
| ③ **폴더 경로 (재귀, 권장)** | `D:\EARS\EEGAgent\` 전체 | **가장 완전** — 폴더 내 모든 실행 파일 + 그 안에서 일어나는 파일 행위 통째 커버 |

#### 권장 등록 (방식 ③)

- **등록 경로**: `D:\EARS\EEGAgent\` 전체 (재귀, 하위 폴더 포함)
  - 환경에 따라 EARS 설치 폴더 경로가 조금 다를 수 있음
- **등록 정책**: **가능한 모든 정책에 동일 적용** 부탁드립니다.
  - 실시간 파일 스캔 (Scan Exception)
  - **Application Control / 행위 정책** (가능하다면) ← 본 케이스의 메모리 증가 주범으로 추정
  - **EDR / 행위 추적** (가능하다면)
  - NetFilter / 네트워크 추적 (가능하다면)

#### 왜 폴더 단위가 더 효과적인가

ResourceAgent 가 실행되는 동안 EPS 가 추적하는 대상은 ResourceAgent.exe 한 파일이 아닙니다:

```
EPS 추적 대상
  ├─ ResourceAgent.exe 의 행위                   ← 방식 ②, ③ 모두 커버
  ├─ ResourceAgent 가 실행하는 LhmHelper.exe     ← ②는 미커버, ③만 커버
  ├─ ResourceAgent 가 읽고 쓰는 설정 파일들        ← ③만 커버
  └─ ResourceAgent 가 기록하는 로그 파일들        ← ③만 커버
```

방식 ② (단일 .exe 등록) 로는 위에서 자식 프로세스 + 파일 I/O 추적이 그대로 남아 메모리 증가가 일부 잔존할 가능성이 있습니다. 방식 ③ (폴더 등록) 은 폴더 안 모든 것을 통째로 신뢰권에 두어 가장 완전.

#### EPS 가 폴더 등록 미지원 시 차선책

폴더 단위 등록이 불가능한 경우, 다음 두 파일을 **모두** 동시 등록 부탁드립니다:
- `D:\EARS\EEGAgent\bin\x86\ResourceAgent.exe`
- `D:\EARS\EEGAgent\utils\lhm-helper\LhmHelper.exe`

#### 왜 이 조치가 적절한가
- ResourceAgent 는 사내 공식 모니터링 SW 로, 운영 보장된 프로그램입니다.
- 다른 사내 SW 도 동일한 방식으로 화이트리스트 등록되어 운영 중일 것으로 예상됩니다.
- 등록 후에도 **EPS 의 기본 보호 (실시간 감시, 백신 검사) 는 그대로 작동**합니다. 단지 행위 기록만 줄어듭니다.

#### 기대 효과
- ResourceAgent 실행 시 메모리 증가 현상 해소
- 등록 외 다른 SW 의 보안 정책에는 영향 없음
- 추가 부작용 없음 (현 시점 예상)

---

## 검증 방법

화이트리스트 등록 후 다음 절차로 효과 확인 가능합니다:

1. EPS 정책에 ResourceAgent.exe (또는 디렉토리) 등록
2. ResourceAgent 서비스 재시작
3. **1시간 동안 메모리 사용량 모니터링**
4. 증가가 멈췄는지 확인 (작업관리자 > 성능 > 메모리)

증가가 멈추면 등록이 제대로 적용된 것입니다.

---

## 협의 요청 사항 (체크리스트)

- [ ] **`D:\EARS\EEGAgent\` 폴더 단위 (재귀)** 등록 가능 여부 확인
- [ ] 등록 가능 정책 범위 확인:
  - [ ] 실시간 파일 스캔 (Scan Exception)
  - [ ] Application Control / 행위 정책
  - [ ] EDR / 행위 추적
  - [ ] NetFilter / 네트워크 추적
- [ ] 화이트리스트된 프로세스가 실행한 자식 프로세스 (LhmHelper.exe) 가 자동 적용되는지, 별도 등록 필요한지 확인
- [ ] 폴더 등록 미지원 시 차선책 적용 가능 여부 (ResourceAgent.exe + LhmHelper.exe 두 파일 동시 등록)
- [ ] 등록 가능하다면 일정 협의 (테스트 PC 1대 우선 적용 권장)
- [ ] 1시간 검증 후 결과 공유
- [ ] 검증 OK 시 운영 PC 전체 적용 일정 협의
- [ ] (필요 시) AhnLab 본사 측 확인 요청 — EPS 가 미등록 SW 의 행위 기록을 일정 한도 이상 가져가는 것이 정상 동작인지

---

---

# 참고 자료 — 기술 검토 내용

> 아래는 위 결론에 도달한 기술적 근거입니다. 협의 시 기술 질문이 나올 경우 참고용으로 첨부합니다.

## 부록 A. 실험 데이터 상세

### 시나리오 매트릭스 (5건)

| # | OS | RAM | EPS | ResourceAgent | Collector | 1h 후 Paged Pool |
|---|----|----|-----|--------------|----------|-----------------|
| ① | Win7 SP1 | 8GB | 있음 | 실행 | 모두 OFF | 증가 (점진) |
| ② | Win7 SP1 | 8GB | 있음 | 실행 | 모두 ON | 증가 (점진) |
| ③ | Win7 SP1 | 8GB | 없음 | (이전 측정) | - | 증가 없음 |
| ④ | Win7 SP1 | 8GB | **삭제** | 실행 | 모두 ON | **증가 없음 (1.5h)** |
| ⑤ | Win7 SP1 | 8GB | 없음 | 미실행 | - | 증가 없음 |

### 핵심 관찰
- **시나리오 ④ 가 결정적**: 동일 PC 에서 EPS 만 제거하면 ResourceAgent 모든 기능을 켜도 증가 없음
- **시나리오 ① vs ② 비교**: ResourceAgent 의 동작량(collector ON/OFF) 은 영향 거의 없음
- **점진적 증가 후 정체** 패턴: 무한히 늘어나는 누수가 아닌 캐시 한도 도달

### 메모리 증가 패턴
```
시점         Paged Pool 증가량
T0          baseline
T+10m       +200MB
T+30m       +500MB
T+1h        +800MB ~ 1GB
T+1.5h      ≈ T+1h (정체)
```

(실측 값은 PC 별로 차이 있음)

---

## 부록 B. 기술적 메커니즘

### Pool Tag 분석
RAMMap / poolmon 측정 결과, 증가하는 메모리 영역의 상위 5개 태그는:

| Tag | 컴포넌트 | 의미 |
|-----|--------|------|
| FMfn | fltmgr.sys (Filter Manager) | 파일 시스템 필터 드라이버의 NAME_CACHE |
| Ntff | ntfs.sys | NTFS File Control Block (FCB) |
| Ntfx | ntfs.sys | NTFS 확장 메타데이터 |
| MmSt | ntoskrnl.exe (Memory Manager) | Mapped File Subsections |
| CM31 | ntoskrnl.exe (Configuration Manager) | 레지스트리 hive 캐시 |

→ 모두 **Windows 표준 시스템 컴포넌트의 풀 태그**. EPS 자체 풀 태그(`V3*`, `Ahn*`, `Asd*`, `Ayag` 등) 가 직접 증가한 것이 아님.

### 메커니즘 (가설: H-EPS-3 Application Control 정책 캐시 / 신뢰도 70%)

1. ResourceAgent.exe 가 EPS 의 신뢰 SW 목록에 미등록
2. EPS 의 Application Control 모듈이 ResourceAgent 의 모든 행위를 정책 검사 대상으로 분류
3. 매 검사 결과를 NTFS / Filter Manager / Memory Manager / Registry 측 reference 로 캐시 (자체 풀 안 잡고)
4. ResourceAgent 의 행위 다양성에 따라 캐시 누적 → 시간 따라 점진 증가
5. 정책 캐시 한도 도달 → 증가 정체

### 보조 메커니즘 (병합 가능)
- EPS 의 EDR 모듈이 ResourceAgent 의 syscall 추적 (H-EPS-1)
- EPS 의 NetFilter 가 ResourceAgent 의 네트워크 통신 추적 (H-EPS-2)

→ 위 셋이 동시 작동하면서 양쪽 캐시 누적이 합산. 점진+정체 패턴은 두 캐시의 합계.

### 왜 누수가 아닌 정상 동작인가
- 점진 증가 후 **정체** 패턴은 bound (한도) 가 있음을 의미
- 진짜 누수라면 무한 단조 증가
- 따라서 "EPS 의 정상 캐시 동작 + 한도가 큼 (8GB Win7 환경에 부적합 가능성)"

---

## 부록 C. 확인된 일반 사례 (Microsoft / OSR 커뮤니티)

| 출처 | 핵심 내용 |
|------|---------|
| Microsoft Q&A "Memory Leak NtfF/MmSt/FMfn" | AV 소프트웨어가 Paged Pool 증가의 원인이 되는 사례 다수 보고 |
| OSR Community "PoolTags and normal growth" | minifilter 가 NTFS / Filter Manager 측 풀 사용을 늘리는 것은 알려진 패턴 |
| Microsoft Japan Blog (2022) | Filter Manager 의 NAME_CACHE (FMfn) 누적 메커니즘 설명 |
| Microsoft Support KB | Filter Manager 측 풀 누수 사례들 |

---

## 부록 E. 화이트리스트 등록 방식별 효과 비교 (기술 검토)

### 일반 보안 SW (Microsoft Defender / AppLocker / WDAC) 의 화이트리스트 모델

대부분의 EDR/EPS 가 채택하는 화이트리스트 식별 방식은 다음 3가지:

| 식별 방식 | 보안성 | 적용 범위 | 운영 부담 |
|---------|--------|---------|---------|
| **이미지 이름** (e.g. `ResourceAgent.exe`) | 낮음 — 동일명 파일 스푸핑 가능 | 어디서 실행되든 적용 | 낮음 |
| **전체 경로** (e.g. `D:\EARS\...\ResourceAgent.exe`) | 중간 — 위치+이름 일치만 적용 | 단일 파일만 | 중간 (재배포 시 갱신) |
| **폴더 경로 (재귀)** (e.g. `D:\EARS\EEGAgent\`) | 중간 — 폴더 내 임의 파일 신뢰 | 폴더 내 모든 실행+I/O | 낮음 (파일 추가/변경 자동 커버) |
| 해시 / 서명자 | 높음 — 파일 변조 즉시 차단 | 정확히 그 빌드만 | 높음 (버전 갱신마다 재등록) |

### Microsoft Defender 공식 권고 (Common exclusion mistakes)

> Don't exclude `Filename.exe` from scanning. Exclude the complete path and file: `C:\Program Files\Contoso\Filename.exe`.

→ **이미지 이름만 등록은 권고 안 함**. 최소 전체 경로 이상.

### 본 케이스 (H-EPS-3 가설) 에서의 효과 차이

증가하는 풀 태그가 **파일 시스템 캐시 (FMfn / Ntff / Ntfx)** 와 **Memory Manager / Registry (MmSt / CM31)** 인 점이 핵심:

- **이미지 이름만**: 파일 식별이 약함. 메모리 증가 차단 효과 부분적 + 보안 위험
- **단일 파일 전체 경로**: ResourceAgent.exe 자체 행위는 차단되지만,
  - **누락 1**: 자식 프로세스 LhmHelper.exe 의 행위 (별개 프로세스로 EPS 가 별도 추적)
  - **누락 2**: ResourceAgent 가 접근하는 conf/log 파일들 (FMfn/Ntff/Ntfx 캐시 메커니즘은 "파일 단위" 로 누적)
- **폴더 경로 (재귀)**: 폴더 내 모든 실행 파일 + 그 폴더 안에서 일어나는 모든 파일 I/O 통째 신뢰 → 5개 풀 태그 모두 차단

### 단, EPS 제품별 "예외 / 화이트리스트" 적용 모듈 차이 주의

화이트리스트 등록 항목이 EPS 의 어느 모듈에 적용되는지 제품마다 다름:

| 적용 모듈 | 의미 |
|---------|------|
| Scan Exception | 실시간 / 수동 파일 스캔만 제외 |
| Application Control 정책 | 행위 검사 정책 검사 제외 (본 케이스 핵심) |
| EDR / Behavior Monitoring | syscall / API 후킹 추적 제외 |
| NetFilter / Network | 네트워크 통신 추적 제외 |

AhnLab EPS 공식 문서는 주로 "Scan Exception" 만 명시. **본 케이스 해소를 위해선 Application Control 정책 + EDR 모듈에도 동일 적용되어야 효과 발생**. EPS 담당자 협의 시 적용 범위 확인 필수.

### 권장 등록 (반복)

1순위: **`D:\EARS\EEGAgent\` 폴더 (재귀, 가능한 모든 정책에 적용)**
2순위 (폴더 미지원 시): **두 파일 동시 등록**
- `D:\EARS\EEGAgent\bin\x86\ResourceAgent.exe`
- `D:\EARS\EEGAgent\utils\lhm-helper\LhmHelper.exe`

---

## 부록 E-1. 모델 보정 — "수명" 단독은 결정 변수가 아님 (2026-04-29 추가)

### 관찰 — RAMMAP idle 반증
RAMMAP.exe 를 장시간 띄워두는 (idle 상태) 경우에도 Paged Pool 증가 현상은 관찰되지 않음. 따라서 "프로세스 수명이 길다" 는 단독으로 본 케이스의 트리거가 아님.

### 보정된 모델

```
EPS 캐시 누적 ≈ ∫ (단위 시간당 행위 빈도 × 행위 다양성) dt × (1 - 신뢰도)
```

= **"신규 시스템 객체 접근 빈도×다양성"** × **"신뢰 분류 (서명)"** 의 곱.

수명은 적분 구간일 뿐 — 빈도가 0이면 시간을 늘려도 누적 0.

| 변수 | RAMMAP idle | RAMMAP active (refresh 반복) | ResourceAgent |
|------|-----------|------|-------------|
| 신규 객체 접근 빈도×다양성 | ≈ 0 (스냅샷 1회 후 정적) | 중간 (같은 path 재조회) | 높음 (매 분 new WMI/device/registry path) |
| 신뢰 분류 (1 - 검사율) | ≈ 1 (MS 서명) | ≈ 1 (MS 서명) | ≈ 0 (미서명) |
| 누적량 | 0 | ≈ 0 (서명으로 막힘) | 큼 |

### 1순위 결정 변수 — 신뢰 분류 (서명)

대부분의 EPS / EDR 은 출시 시점에 Microsoft 코드 서명 인증서를 baseline trust 에 등록. MS 서명 SW 는 Application Control 단계에서 검사 자체가 우회되어, 행위 빈도 무관하게 캐시 entry 미생성. ResourceAgent 는 미서명 → 모든 신규 객체 접근에 대해 캐시 entry 발생.

### 대안 / 보완 조치 — 코드 서명

화이트리스트 등록은 PC 별 + EPS 정책별 운영 부담. 더 근본 대안은 **자사 SW 에 코드 서명 도입**:

- 인증서 1장 발급 → 자사 모든 SW (ResourceAgent / LhmHelper / 기타 사내 도구) 가 EPS baseline 에 자동 포함
- 화이트리스트 등록 불필요 (대부분의 EPS 가 알려진 인증서 자동 신뢰)
- 신규 SW 배포 시에도 별도 EPS 협의 불필요

**도입 절차** (대략):
1. 코드 서명 인증서 발급 (DigiCert / Sectigo / GlobalSign 등, 연 ~$300~$500)
2. `signtool.exe` 로 빌드 산출물 서명
3. 패키징 스크립트 (`scripts/package.ps1`) 에 서명 단계 추가
4. EPS 측에서 해당 인증서가 신뢰 목록에 있는지 확인 (드물게 사내 PKI 별도 등록 필요)

### 결정적 검증 실험 (참고, 미실행)

본 모델을 더 정확히 검증하려면:
- **테스트 A**: MS 서명된 활성 Sysinternals (procmon/procexp) 1시간 → 증가 없으면 "서명이 1순위" 확정
- **테스트 B**: 미서명 idle hello.exe 1시간 → 증가 없으면 "빈도 0 이면 미서명도 무영향" 확정

본 케이스는 화이트리스트 등록으로 해소되었으므로 위 실험은 미실행. 향후 서명 도입 검토 시 ROI 판단 자료로 활용.

---

## 부록 D. 검증된 사실 vs 미확정 사항

### 데이터로 확정된 사실
- ✅ EPS 가 메모리 증가의 필요조건
- ✅ ResourceAgent 의 동작량과 메모리 증가 정도는 무관
- ✅ 누수가 아닌 캐시 누적 패턴 (점진+정체)
- ✅ 증가 영역은 Windows 표준 컴포넌트 풀 (EPS 자체 풀 아님)
- ✅ **(2026-04-29 갱신)** EPS 화이트리스트 등록으로 메모리 증가 해소 — H-EPS-3 가설 확정
- ✅ **(2026-04-29 갱신)** 수명은 단독 결정 변수가 아님 (RAMMAP idle 반증). 신뢰 분류 + 행위 빈도×다양성 곱이 핵심

### 잔여 미확정 사항
- ❓ 보조 메커니즘 분리 — H-EPS-1 (EDR) / H-EPS-2 (NetFilter) 가 H-EPS-3 와 동시 작동하는지 단독은 무영향인지
- ❓ Win10/11 환경에서 동일 현상 재현 여부 (OS 의존성)
- ❓ EPS 측 정책 캐시 한도 설정 가능 여부
- ❓ AhnLab 본사 측 공식 입장 — 미등록 SW 의 정책 캐시 누적이 정상 동작인지
- ❓ 코드 서명 도입 시 화이트리스트 등록 없이 동일 효과 가능 여부 (테스트 A/B 실험 필요)

---

**문의**: ResourceAgent 개발팀
