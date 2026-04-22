# LhmHelper .NET 8 → .NET Framework 4.7 전환 (Win7 Modified 메모리 폭증 이슈)

## 문제 현상

Windows 7 Pro SP1 + Intel i7-6700 + 4GB RAM PC에서 ResourceAgent + LhmHelper 실행 시 시스템이 사실상 멈추는 현상이 발생.

### 측정값

- **LhmHelper.exe 자체 프로세스 메모리**: ~20MB (정상)
- **Windows 리소스 모니터 "수정됨(Modified)" 메모리**: 155MB → **2267MB** (2분 만에 2GB 증가)
- **여유 메모리(Free)**: 0MB로 하락
- **디스크 I/O, 로그 파일 크기**: 원인 아님 (각각 46% 여유, 133KB)

### 재현 조건

- LhmHelper 없이 ResourceAgent만 실행: **문제 없음**
- ResourceAgent 서비스 중지: Modified 메모리가 순간적으로 "사용 중"으로 전환된 후 **즉시** 원래 상태(155MB 수준)로 원복

## 진단 과정 및 원인

### 초기 가설 (오진)

처음에는 WinRing0 커널 드라이버의 I/O 버퍼가 Modified 페이지로 누적되는 것으로 추정. 그러나 전문가 검토에서 다음과 같이 반증됨:

> 커널 풀 누수는 드라이버가 unload되기 전까지 해제되지 않음. LhmHelper.exe 프로세스 종료로 WinRing0 드라이버가 unload되지 않기에, 풀 누수라면 "즉시 원복" 불가능.

### 실제 원인

**.NET 8 런타임의 GC regions (기본값) + single-file publish 방식의 Windows 7 비호환**이 프로세스 사설 커밋 페이지를 Modified 상태로 누적시키는 것으로 판명.

프로세스 사설 커밋 페이지가 Modified로 trim된 상태는 프로세스 종료 시 즉시 해제되며, 이는 관찰된 "즉시 원복" 패턴과 정확히 일치함.

### 근거 — dotnet/runtime 공식 이슈

Microsoft가 이미 인지하고 있지만 수정하지 않기로 결정한 이슈:

- **[#79469](https://github.com/dotnet/runtime/issues/79469)**: 빈 .NET 7 앱이 Windows 7 x64에서 4GB commit (.NET 6는 14MB)
- **[#80078](https://github.com/dotnet/runtime/issues/80078)**: self-contained Hello World가 Windows 7에서 3GB 점유
- **[#62453](https://github.com/dotnet/runtime/issues/62453)**: single-file publishing이 Windows 7과 구조적 비호환

모두 "closed as not planned" — Microsoft가 고칠 의사 없음. Windows 7은 .NET 7부터 공식 미지원.

## 해결 전략 (최종)

### 전환: `.NET 8` → `.NET Framework 4.7` (`net47` 타겟)

**선정 이유**:
- Windows 7 SP1 공식 지원 (Microsoft KB4033342)
- **공장 PC에 이미 .NET Framework 4.7이 기본 설치되어 있음** — 추가 설치 불필요
- 런타임 자체가 OS 일부 → GC regions 이슈 없음
- LibreHardwareMonitorLib 0.9.4가 `netstandard2.0` 타겟 지원 (net47에서 consume 가능)
- .NET Framework 4.x는 in-place 업데이트 → ARSAgent 등 기존 .NET 앱 영향 없음 (ARSAgent는 Go 기반이라 무관)

### 중간 단계 (검증 후 이동)

1. **GC 튜닝** (커밋 `ec1fc0c`): Workstation GC로 전환, `System.GC.RetainVM=false` 등 추가
   - 결과: 프로세스 자체 메모리는 다소 개선되었으나 Modified 메모리 폭증은 해결 안 됨
   - 이유: GC regions의 구조적 이슈는 옵션으로 회피 불가

2. **net472 타겟으로 이동** (커밋 `e93fd2a`): .NET Framework 4.7.2 필요 → 공장 PC는 4.7만 있어 설치 필요
   - 장비 PC에 .NET Framework 설치는 허가되지 않은 작업이라 net47로 추가 하향 조정

3. **net47 타겟 최종 확정**: 공장 PC가 이미 4.7을 갖고 있어 **아무런 설치 없이 동작**. 코드 검증 결과 Windows 경로에서 netstandard2.0 빌드와 net472 빌드 차이 없음.

## 구현 변경 사항

### LhmHelper 프로젝트

- `LhmHelper.csproj`:
  - `TargetFramework`: `net8.0` → `net47`
  - 제거: `RuntimeIdentifier`, `PublishSingleFile`, `SelfContained`, `EnableCompressionInSingleFile`, `ImplicitUsings`, GC 튜닝 `RuntimeHostConfigurationOption`
  - 추가: `<PackageReference Include="System.Text.Json" Version="8.0.5" />` (net47 기본 미제공)
  - 추가: `<LangVersion>latest</LangVersion>` — C# 10+ 문법 유지
  - 추가: `<PlatformTarget>AnyCPU</PlatformTarget>`

- `Program.cs`:
  - `ImplicitUsings` 제거로 명시적 `using` 디렉티브 추가
  - 로직 변경 없음

### 빌드/패키지 스크립트

- `scripts/package.sh`, `scripts/package.ps1`:
  - 빌드: `dotnet publish -c Release -r win-x64 --self-contained` → `dotnet publish -c Release`
  - 출력 경로: `bin/Release/net8.0/win-x64/publish/` → `bin/Release/publish/` (또는 `bin/Release/net47/publish/`)
  - 복사: 단일 exe → **디렉토리 전체** (exe + config + DLL 다수)

### 설치 스크립트

- `scripts/install_ResourceAgent.bat`:
  - `.NET Framework 4.7+` 레지스트리 검사 (`Release >= 460798`)
  - **미설치 시 경고 메시지만 출력** (자동 설치하지 않음 — 장비 PC의 임의 시스템 변경 방지)
  - ResourceAgent 서비스는 정상 설치. LhmHelper 수집기만 빈 데이터 반환 (non-fatal).

- `scripts/package_ndp48.sh` / `package_ndp48.ps1` (별도, 옵션):
  - 드문 케이스로 .NET Framework 4.6 이하만 있는 PC용 별도 설치 패키지
  - 관리자가 승인된 PC에 수동 설치
  - `install_ndp48.bat`으로 idempotent 설치 (이미 설치된 경우 스킵)

### Go 측 진단 강화

- `internal/collector/lhm_provider_windows.go`:
  - `diagnoseStartupFailure()` 헬퍼 추가
  - "pipe being closed" 류 에러 감지 시 .NET 4.7 요구사항 힌트를 에러 메시지에 자동 포함
  - 운영자가 로그만 보고 원인 즉시 파악 가능

## 배포 영향

- 현장 Windows 7 64-bit PC: **.NET Framework 4.7 기본 설치되어 있어 추가 조치 불필요**
- 현장 Windows 7 32-bit PC: LhmHelper 미설치 대상이라 무관
- Windows 8+/10+/11 PC: .NET 4.7 이상 기본 설치되어 있어 동작
- 예외 케이스 (.NET 4.6 이하만 있는 PC): 별도 NDP48 패키지로 관리자 수동 설치

## 롤백 전략

이번 전환은 PC에 시스템 레벨 변경을 가하지 않으므로 **롤백이 자유로움**.

1. **LhmHelper만 제외**: `install_ResourceAgent.bat /nolhm` 재실행 → ResourceAgent는 유지
2. **ResourceAgent 전체 롤백**: `/uninstall` 후 이전 net8 패키지로 재설치
3. NDP48 설치가 안 되어 있으므로 .NET Framework 상태는 변함 없음

## 참고

- GC 튜닝 시도 커밋: `ec1fc0c`
- net472 전환 커밋: `e93fd2a`
- net47 최종 확정 커밋: (이번 PR)
- 전문가 검토 산출물: `.claude/plans/net8-typed-pie.md`
- 관련 이슈 문서: `docs/issues/pawnio-windows7-incompatible.md`
