# PawnIO 2.0.1.0 Windows 7 비호환 이슈

## 현상

PawnIO_setup.exe 2.0.1.0을 Windows 7에서 실행 시 오류 발생:

```
The procedure entry point LdrSetDefaultDllDirectories could not be located
in the dynamic link library ntdll.dll
```

## 원인

`LdrSetDefaultDllDirectories`는 Windows 8부터 추가된 `ntdll.dll` 함수.
PawnIO 2.x는 Windows 8+ 타겟으로 빌드되어 Windows 7에서 실행 불가.
(OS 레벨 API 부재 — KB 패치로 해결 불가)

## 실제 영향: 없음

**PawnIO 설치가 실패해도 온도 수집은 정상 동작한다.**

LibreHardwareMonitor(LHM)는 하드웨어 접근 드라이버를 다음 순서로 시도:

1. **PawnIO** 로드 시도 → Windows 7에서는 설치 불가 → 실패
2. **WinRing0 자동 폴백** → LHM NuGet 패키지에 `WinRing0x64.sys`/`WinRing0x64.dll`이 내장 리소스로 포함되어 있어 자동 추출 후 로드

따라서 LhmHelper.exe는 PawnIO 없이도 WinRing0를 통해 CPU 온도, 마더보드 온도 등을 수집할 수 있다.
GPU 데이터는 NVAPI/NVML(NVIDIA), ADL(AMD)을 사용하므로 커널 드라이버 자체가 불필요.

### Windows 7 실제 테스트 결과 (2026-03-05)

- CPU 온도: 수집 정상 (WinRing0 폴백)
- GPU: 수집 정상 (NVAPI/ADL, 드라이버 불필요)
- PawnIO 설치 실패 로그만 남을 뿐 실질적 영향 없음

## 배경: PawnIO vs WinRing0

둘 다 유저모드에서 하드웨어 레지스터(I/O 포트, MSR, PCI)에 접근하기 위한 커널 드라이버.
수집 가능 항목은 동일하며, 차이는 비기능적 측면에만 존재.

| | WinRing0 | PawnIO |
|---|---|---|
| 수집 항목 | 동일 | 동일 |
| 출처 | OpenLibSys (2007) | LHM 자체 개발 (2023~) |
| 서명 | 없음/만료 | Microsoft WHQL |
| Windows 7 지원 | O | X (8+) |
| 보안 | CVE 다수, 악성코드 악용 사례 | WinRing0 보안 문제 해결 목적 |
| Defender 탐지 | 차단될 수 있음 | 문제 없음 |
| Secure Boot | 서명 문제로 로드 실패 가능 | 정상 |

## 결론

**추가 조치 불필요.** LHM의 WinRing0 내장 폴백이 Windows 7에서 자동으로 동작하므로,
PawnIO 설치 실패와 무관하게 온도/GPU 수집이 정상 수행된다.

공장 내부망 환경에서 Defender가 WinRing0를 차단할 경우에만 화이트리스트 처리 필요.

## 대응: install_ResourceAgent.bat OS 버전 자동 분기

`install_ResourceAgent.bat`이 OS 버전을 감지하여 자동 분기:

- **Windows 8+**: PawnIO 드라이버 설치 (기존 동작)
- **Windows 7**: PawnIO 설치 스킵, LhmHelper만 복사 (WinRing0 자동 폴백)

수동으로 `/nolhm` 옵션을 지정할 필요 없이, Windows 7에서는 자동으로 PawnIO를 건너뛴다.

## 상태

- [x] 대응 방안 결정 → install_ResourceAgent.bat에서 OS 버전별 자동 분기
- [x] Windows 7 실제 테스트 완료 (2026-03-05)
- [x] install_ResourceAgent.bat 수정 완료
