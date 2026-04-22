## 상위 계획 참조

세션 중 "상위 플랜 체크해" 요청 시: `/Users/hyunkyungmin/Developer/ARS/.claude/PLANNING.md` 읽기

# CLAUDE.md

이 파일은 Claude Code (claude.ai/code)가 이 저장소에서 작업할 때 참고하는 가이드입니다.

## 프로젝트 개요

ResourceAgent는 공장 내 PC(10,000대 이상)의 하드웨어 자원 사용률을 수집하는 Go 기반 경량 모니터링 에이전트입니다. CPU, Memory, Disk, Network, 온도 메트릭을 수집하여 Kafka 또는 KafkaRest Proxy로 전송합니다.

**지원 플랫폼**: Windows 7+, Windows Server 2008 R2+, CentOS 6.5+, RHEL 6.5+, Ubuntu 14.04+ (커널 2.6.32+)

## 빌드 명령어

```bash
# 버전 정보 (git tag 기반, 패키지 스크립트 사용 시 자동 주입)
VERSION=$(git describe --tags --always --dirty)
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}"

# Windows 64-bit 빌드 (Go 1.20 툴체인 — Windows 7+ 호환)
GOTOOLCHAIN=go1.20.14 GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o ResourceAgent.exe ./cmd/resourceagent

# Windows 32-bit 빌드 (Go 1.20 툴체인 — Windows 7 32-bit 호환)
GOTOOLCHAIN=go1.20.14 GOOS=windows GOARCH=386 go build -ldflags "$LDFLAGS" -o ResourceAgent_x86.exe ./cmd/resourceagent

# Linux 빌드 (Go 1.20 툴체인 — CentOS 6+ 호환)
GOTOOLCHAIN=go1.20.14 GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o resourceagent ./cmd/resourceagent

# 패키지 스크립트로 빌드+패키징 (--build 시 Go 1.20 + 버전 자동 주입)
./scripts/package.sh --build --lhmhelper              # 64-bit (기본)
./scripts/package.sh --build --arch 386               # 32-bit (LhmHelper 자동 제외)

# 테스트 실행
go test ./...

# 커버리지 포함 테스트
go test -cover ./...

# 특정 패키지 테스트
go test ./internal/collector/...
```

## 아키텍처

SOLID 원칙을 준수하는 컴포넌트 구조:

```
ConfigManager ──► Scheduler ──► Collectors ──► Sender (Kafka)
      │                              │
      └──────────────────────────────┴──► Logger
```

### 핵심 인터페이스

- **Collector**: 특정 자원 메트릭 수집 (CPU, Memory, Disk, Network, Temperature, StorageHealth, Uptime, ProcessWatch 등 15종)
- **Sender**: Kafka로 메트릭 전송
- **ConfigManager**: 설정 로드, 변경 감지, Hot Reload 처리

### 디렉토리 구조 (개발)

```
resourceagent/
├── cmd/resourceagent/main.go       # 진입점
├── internal/                       # 핵심 구현
│   ├── collector/                  # Collector 구현체
│   ├── config/                     # 설정 관리
│   ├── discovery/                  # ServiceDiscovery HTTP 클라이언트
│   ├── eqpinfo/                    # Redis EQP_INFO 조회
│   ├── network/                    # IP 감지 + SOCKS5 dialer
│   ├── sender/                     # Kafka/KafkaRest/File Sender 구현
│   ├── scheduler/                  # 수집 스케줄링
│   ├── logger/                     # 구조화된 로깅
│   └── service/                    # Windows/Linux 서비스 통합
├── conf/ResourceAgent/             # 설정 파일 (통합 구조 기본)
│   ├── ResourceAgent.json          # 메인 설정
│   ├── Monitor.json                # 수집기 설정 (Hot Reload)
│   └── Logging.json                # 로깅 설정 (Hot Reload)
├── configs/                        # 설정 파일 (레거시 참조용)
└── scripts/                        # 설치/패키징 스크립트
    ├── install_ResourceAgent.bat / install_ResourceAgent.ps1   # Windows 설치 (패키지에 포함)
    ├── install_ResourceAgent.sh    # Linux 설치
    ├── package.sh / package.ps1    # 설치 패키지 빌드
    ├── INSTALL_GUIDE.txt           # 현장 담당자용 가이드 (패키지에 포함)
    └── resourceagent.service       # systemd 서비스 파일
```

### 통합 배포 구조 (ARSAgent 공유 basePath)

```
D:\EARS\EEGAgent\                         ← basePath (ManagerAgent FTP home)
├── bin\x86\
│   ├── earsagent.exe                     # ARSAgent 바이너리 (기존)
│   └── ResourceAgent.exe                 # ResourceAgent 바이너리 (신규)
├── conf\
│   ├── ARSAgent\                         # ARSAgent 설정 (기존)
│   └── ResourceAgent\                    # ResourceAgent 설정 (신규)
│       ├── ResourceAgent.json
│       ├── Monitor.json
│       └── Logging.json
├── log\
│   ├── ARSAgent\                         # ARSAgent 로그 (기존)
│   └── ResourceAgent\                    # ResourceAgent 로그 (신규)
│       ├── ResourceAgent.log
│       └── metrics.jsonl
└── utils\                               # 공유 유틸 (tail.exe 등)
    └── lhm-helper\                      # Windows 전용
        ├── LhmHelper.exe                # C# 하드웨어 센서 헬퍼
        └── PawnIO_setup.exe             # 하드웨어 접근 드라이버 설치/제거
```

## 핵심 의존성

- `github.com/shirou/gopsutil/v3` - 크로스 플랫폼 시스템 메트릭
- `github.com/IBM/sarama` - Kafka 클라이언트
- `github.com/fsnotify/fsnotify` - 파일 변경 감지 (Hot Reload)
- `github.com/rs/zerolog` - 구조화된 로깅
- `golang.org/x/sys/windows/svc` - Windows 서비스 지원

## 구현 가이드

### 새 Collector 추가 방법

1. `internal/collector/`에 Collector 인터페이스 구현
2. `registry.go`에 등록
3. Scheduler, Sender 코드 변경 불필요 (개방/폐쇄 원칙)

### 비기능 요구사항

- CPU 사용률: 유휴 시 1% 미만
- 메모리: 50MB 이하
- 바이너리 크기: 20MB 이하
- 각 Collector는 격리되어야 함 - 개별 오류가 다른 Collector에 영향 없어야 함

### 플랫폼별 참고사항

**CPU 온도 수집**:
- Windows: LibreHardwareMonitor (LhmHelper.exe) - PawnIO 드라이버 사용
- Linux: `/sys/class/thermal/thermal_zone*/temp` 또는 lm-sensors (gopsutil)

#### Windows 온도 수집 구조 (Daemon 모드)

```
ResourceAgent (Go)                         LhmHelper.exe --daemon (C#)
  LhmProvider.Start()  ─── spawn ──────►  computer.Open() (1회)
  LhmProvider.GetData() ── "collect\n" ►  hardware.Update() → JSON stdout
                        ◄─ JSON line ──   (반복, stdin EOF 시 종료)
  LhmProvider.Stop()   ── stdin close ►   computer.Close()
```

- **Daemon 모드**: `computer.Open()/Close()` 1회만 수행하여 PawnIO 드라이버 핸들 누적 방지
- **프로세스 관리**: Go LhmProvider가 시작/감시/재시작 (지수 백오프: 1s~60s)
- **하위호환**: `--daemon` 플래그 없으면 기존 one-shot 모드로 동작

**LhmHelper 빌드**:
- TargetFramework: **.NET Framework 4.7.2 (`net472`)** — Windows 7 공식 지원. 과거 .NET 8 self-contained는 Win7에서 Modified 메모리 폭증 이슈(`docs/issues/win7-net8-modified-memory.md`)로 전환.
- 요구 NuGet: `LibreHardwareMonitorLib` 0.9.4, `System.Text.Json` 8.0.5 (net472 기본 미제공)
- 런타임: **.NET Framework 4.8 이상**이 현장 PC에 필요 (Release ≥ 528040). `install_ResourceAgent.bat`이 자동 감지/설치.

```bash
cd utils/lhm-helper
dotnet publish -c Release
# 출력: bin/Release/publish/ (또는 bin/Release/net472/publish/)
# LhmHelper.exe + LhmHelper.exe.config + 의존 DLL 10개+ (LibreHardwareMonitorLib, HidSharp, System.Text.Json 등)
```

**.NET Framework 4.8 오프라인 설치기**:
- **메인 설치 패키지에 번들링되지 않음** (장비 PC의 임의 시스템 변경 방지 정책)
- `scripts/package_ndp48.sh` / `package_ndp48.ps1`로 **별도 패키지 생성**
- `scripts/vendor/NDP48-x86-x64-AllOS-ENU.exe` (~112MB)에 수동 배치 필요 (git에 커밋되지 않음). 상세: `scripts/vendor/README.md`
- `install_ResourceAgent.bat`은 .NET 버전 감지 후 미달 시 **경고만 출력**, 자동 설치하지 않음. 관리자 승인 후 별도 NDP48 패키지를 수동 설치.

**PawnIO 드라이버**:
- LibreHardwareMonitor의 하드웨어 접근 드라이버 (WinRing0 대체)
- 설치: `PawnIO_setup.exe /S` (사일런트 설치)
- 재부팅 불필요
- Microsoft 서명 버전 제공
- Windows 7에서는 미지원이므로 WinRing0 fallback

**배포**: `scripts/package.sh --lhmhelper` 또는 `scripts/package.ps1 -IncludeLhmHelper`로 메인 설치 패키지 생성
- 패키지에 ResourceAgent.exe, 설정 파일, install_ResourceAgent.bat/ps1, INSTALL_GUIDE.txt 포함
- LhmHelper 디렉토리(폴더 전체) + PawnIO_setup.exe 포함 (`/nolhm` 옵션으로 제외 가능)
- **.NET Framework 4.8 설치기는 메인 패키지에 없음** — `scripts/package_ndp48.sh`로 별도 패키지 생성
- PawnIO 드라이버 설치/제거는 `install_ResourceAgent.bat`에서 자동 처리. .NET Framework는 경고만.

### 데이터 흐름

1. ConfigManager가 JSON 설정 로드 및 변경 감시
2. sender_type != "file" 이면: IP 감지 → Redis EQP_INFO 조회 → ServiceDiscovery → KafkaRest/Topic 결정
3. LhmProvider가 LhmHelper.exe를 daemon 모드(`--daemon`)로 시작 (Windows만, 실패 시 non-fatal)
4. Scheduler가 설정된 주기로 Collector 등록
5. Collector가 스케줄에 따라 메트릭 수집 (LHM 기반 collector는 LhmProvider 공유 캐시 사용)
6. Sender가 메트릭 전송:
   - **kafkarest**: MetricData → EARSRow[] → `sanitizeName()` 적용 → 평문 raw (Grok 호환) → HTTP POST (`KafkaMessageWrapper2`)
   - **kafka**: MetricData → EARSRow[] → `sanitizeName()` 적용 → JSON raw (ParsedDataList: `{iso_timestamp, parsed: [{field, value, dataformat}]}`) → sarama produce (`KafkaValue`), broker 주소는 ServiceDiscovery KafkaRest 주소의 호스트 + BrokerPort(기본 9092)로 자동 결정, 토픽은 kafkarest와 동일하게 `ResolveTopic` 사용
   - **file**: MetricData → EARSRow[] → `sanitizeName()` 적용 → format에 따라 grok 평문 또는 JSON(ParsedDataList) 파일에 기록
   - **ESID**: `{Process}:{EqpID}-{metricType}-{timestamp_ms}-{counter}` — metricType으로 타입 간 중복 방지
7. 로컬 버퍼링 없음 - 네트워크 단절 시 데이터 유실 허용

## 서비스 설치

**Windows** (설치 패키지 사용):
```cmd
REM 패키지 생성 (개발 PC)
scripts\package.ps1 -IncludeLhmHelper

REM 현장 PC에서 설치 (패키지 압축 해제 후)
install_ResourceAgent.bat
install_ResourceAgent.bat /basepath D:\EARS\EEGAgent
install_ResourceAgent.bat /nolhm
install_ResourceAgent.bat /uninstall
```

**Linux (systemd)**:
```bash
sudo ./scripts/install_ResourceAgent.sh --base-path /opt/EEGAgent
sudo ./scripts/install_ResourceAgent.sh --uninstall
```
