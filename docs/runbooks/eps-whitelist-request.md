# ResourceAgent + EPS 환경 메모리 사용량 증가 현상 공유

> EPS 운영 담당자 협의용 참고 자료
> 작성일: 2026-04-28

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

### **ResourceAgent.exe 를 EPS 의 신뢰 SW 목록 (White list) 에 등록**

#### 등록 대상
- **파일**: `ResourceAgent.exe`
- **설치 경로** (참고): `D:\EARS\EEGAgent\bin\x86\ResourceAgent.exe`
  - 환경에 따라 경로 조금 다를 수 있음
- **가능하면 디렉토리 단위 등록 권장**: `D:\EARS\EEGAgent\` 전체

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

- [ ] ResourceAgent.exe 의 EPS 화이트리스트 등록 가능 여부 확인
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

## 부록 D. 검증된 사실 vs 미확정 사항

### 데이터로 확정된 사실
- ✅ EPS 가 메모리 증가의 필요조건
- ✅ ResourceAgent 의 동작량과 메모리 증가 정도는 무관
- ✅ 누수가 아닌 캐시 누적 패턴 (점진+정체)
- ✅ 증가 영역은 Windows 표준 컴포넌트 풀 (EPS 자체 풀 아님)

### 추가 검증 필요 사항
- ❓ 정확한 메커니즘이 H-EPS-3 (Application Control) 인지, H-EPS-1 (EDR) 인지, 둘 다인지
- ❓ Win10/11 환경에서 동일 현상 재현 여부 (OS 의존성)
- ❓ EPS 측 정책 캐시 한도 설정 가능 여부
- ❓ AhnLab 본사 측 공식 입장

→ ResourceAgent.exe 화이트리스트 등록 후 1시간 검증으로 H-EPS-3 가설 확정 가능.

---

**문의**: ResourceAgent 개발팀
