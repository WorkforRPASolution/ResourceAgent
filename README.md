# ResourceAgent

공장 내 PC(10,000대 이상)의 하드웨어 자원 사용률을 수집하는 Go 기반 경량 모니터링 에이전트입니다.

## 주요 기능

- **15종 메트릭 수집**: CPU, Memory, Disk, Network, Temperature, Fan, GPU, Voltage, Motherboard Temperature, Storage S.M.A.R.T, Storage Health, Uptime, ProcessWatch, 프로세스 CPU/Memory
- **Windows 하드웨어 모니터링**: LibreHardwareMonitor (LhmHelper) 연동으로 온도, 팬, GPU, 전압, 메인보드 온도, 스토리지 S.M.A.R.T 수집
- **유연한 전송 방식**: `file`, `kafka`, `kafkarest` 3가지 sender 지원
- **EARS 호환 포맷**: ARSAgent 호환 Grok 평문 및 JSON(ParsedDataList) 포맷 지원
- **3분할 설정**: ResourceAgent.json / Monitor.json / Logging.json 독립 관리
- **Hot Reload**: Monitor.json, Logging.json 변경 시 서비스 재시작 없이 자동 반영
- **프로세스 모니터링**: CPU/Memory 상위 N개 프로세스 + 지정 프로세스 추적
- **크로스 플랫폼**: Windows, Linux 지원 (macOS 개발/테스트용)

## 시스템 요구사항

### 지원 플랫폼
- Windows 7+, Windows Server 2008 R2+
- CentOS 6.5+, RHEL 6.5+, Ubuntu 14.04+, 기타 Linux (커널 2.6.32+)
- macOS (개발/테스트용, 하드웨어 센서 제한)

### 리소스 사용량
- CPU: 유휴 시 1% 미만
- Memory: 50MB 이하
- 바이너리 크기: 20MB 이하

## 빌드

### 버전 관리

git tag 기반으로 버전을 관리합니다. 빌드 시 `git describe --tags --always`로 버전을 추출하여 `-ldflags`로 바이너리에 주입합니다. (`--dirty` 플래그는 사용하지 않음 — Fody 등 빌드 도구가 FodyWeavers.xml을 일시적으로 수정해도 버전에 `-dirty`가 붙지 않도록 함.)

**버전 문자열 형식:**

| 상태 | `git describe` 출력 | 예시 |
|------|---------------------|------|
| 태그가 가리키는 커밋 | `v{major}.{minor}.{patch}` | `v1.0.0` |
| 태그 이후 추가 커밋 | `v{tag}-{N}-g{hash}` | `v1.0.0-3-gabcdef1` |
| 태그 없음 | `{short hash}` | `abcdef1` |
| git 저장소 아님 | `dev` (fallback) | `dev` |

**시작 로그에 `version`과 `build_time`이 출력됩니다:**

```
"Starting ResourceAgent" version=v1.0.0 build_time=2026-03-06T12:00:00Z
```

**바이너리 버전 확인:**

```bash
ResourceAgent.exe -version
# 또는
./resourceagent -version
```

**패키지 스크립트(`--build` / `-Build`) 사용 시 버전이 자동 주입됩니다.** 수동 빌드 시에는 아래 "ResourceAgent 빌드" 섹션의 `-ldflags` 예시를 참고하세요.

**git tag 사용법:**

```bash
# 태그 생성
git tag v1.0.0

# 태그 푸시
git push origin v1.0.0

# 태그 목록 조회
git tag -l

# 태그 삭제 (로컬 + 원격)
git tag -d v1.0.0
git push origin --delete v1.0.0

# 현재 버전 확인
git describe --tags --always
```

### 사전 요구사항
- Go 1.21 이상 (GOTOOLCHAIN으로 Go 1.20 자동 다운로드하여 Windows 7+ 호환 빌드)
- (Windows 하드웨어 모니터링 시) .NET SDK 6+ (net47 타겟 빌드 지원, .NET Framework 4.7 Targeting Pack 포함)
- (Windows 하드웨어 모니터링 배포 PC) .NET Framework 4.7 이상 런타임 — Windows 7 공장 PC는 대부분 기본 설치되어 있음

### ResourceAgent 빌드

**Bash (Linux/macOS):**
```bash
# 의존성 다운로드
go mod tidy

# 버전 정보 (git tag 기반)
VERSION=$(git describe --tags --always)
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}"

# Linux 빌드 (Go 1.20 툴체인 — CentOS 6+ 호환)
GOTOOLCHAIN=go1.20.14 GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o resourceagent ./cmd/resourceagent

# Windows 64-bit 빌드 (Go 1.20 툴체인 — Windows 7+ 호환)
GOTOOLCHAIN=go1.20.14 GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o ResourceAgent.exe ./cmd/resourceagent

# Windows 32-bit 빌드 (Go 1.20 툴체인 — Windows 7 32-bit 호환)
GOTOOLCHAIN=go1.20.14 GOOS=windows GOARCH=386 go build -ldflags "$LDFLAGS" -o ResourceAgent_x86.exe ./cmd/resourceagent
```

**PowerShell (Windows):**
```powershell
# 버전 정보 (git tag 기반)
$Version = git describe --tags --always
$BuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$Ldflags = "-X main.version=$Version -X main.buildTime=$BuildTime"

# Windows 64-bit 빌드 (Go 1.20 툴체인 — Windows 7+ 호환)
$env:GOTOOLCHAIN="go1.20.14"; $env:GOOS="windows"; $env:GOARCH="amd64"
go build -ldflags "$Ldflags" -o ResourceAgent.exe ./cmd/resourceagent

# Windows 32-bit 빌드 (Go 1.20 툴체인 — Windows 7 32-bit 호환)
$env:GOTOOLCHAIN="go1.20.14"; $env:GOOS="windows"; $env:GOARCH="386"
go build -ldflags "$Ldflags" -o ResourceAgent_x86.exe ./cmd/resourceagent

# 또는 패키지 스크립트 사용 (빌드 + 패키징, 버전 자동 주입)
.\scripts\package.ps1 -Build -IncludeLhmHelper          # 64-bit
.\scripts\package.ps1 -Build -Arch 386                   # 32-bit
```

### LhmHelper 빌드 (Windows 하드웨어 모니터링)

Windows에서 온도, 팬, GPU, 전압, 메인보드 온도, 스토리지 S.M.A.R.T를 수집하려면 LhmHelper가 필요합니다.

TargetFramework는 **.NET Framework 4.7 (`net47`)**이며, 현장 PC에는 **.NET Framework 4.7 이상 런타임**이 필요합니다. **Windows 7 공장 PC는 대부분 4.7이 기본 설치되어 있어 추가 설치 없이 동작**합니다. 런타임이 없는 PC에서는 `install_ResourceAgent.bat`이 **경고 메시지만 출력**하고 ResourceAgent 서비스 자체는 정상 설치합니다 (장비 PC의 임의 시스템 변경 방지).

```bash
cd utils/lhm-helper
dotnet publish -c Release
# 출력: bin/Release/publish/ (또는 bin/Release/net47/publish/)
# LhmHelper.exe + LhmHelper.exe.config (Costura.Fody로 모든 의존 DLL을 exe에 embed, 단일 파일 배포)
```

> .NET 8 self-contained에서 net47로 전환한 배경은 `docs/issues/win7-net8-modified-memory.md` 참조. Windows 7 PC에서 "Modified" 메모리 폭증 이슈 해결을 위한 조치.

### 테스트

```bash
# 전체 테스트
go test ./...

# 커버리지 포함
go test -cover ./...

# 특정 패키지
go test ./internal/collector/...
go test ./internal/sender/...
```

## 설정

설정은 3개의 독립 파일로 분리되어 있습니다 (PascalCase 키 사용).

| 파일 | 용도 | Hot Reload |
|------|------|:----------:|
| `ResourceAgent.json` | 에이전트 ID, sender, Kafka, 인프라 연동 | - |
| `Monitor.json` | Collector 활성화/비활성화, 수집 주기, 필터 | O |
| `Logging.json` | 로그 레벨, 파일 경로, 로테이션 | O |

### ResourceAgent.json

```json
{
  "Agent": {
    "ID": "",
    "Hostname": "",
    "Tags": {
      "environment": "production",
      "location": "factory-1"
    }
  },
  "SenderType": "file",
  "File": {
    "FilePath": "log/ResourceAgent/metrics.jsonl",
    "MaxSizeMB": 50,
    "MaxBackups": 3,
    "Console": true,
    "Pretty": false,
    "Format": "grok"
  },
  "Kafka": {
    "Brokers": ["localhost:9092"],
    "Topic": "factory-metrics",
    "Compression": "snappy",
    "RequiredAcks": 1,
    "MaxRetries": 3,
    "EnableTLS": false,
    "SASLEnabled": false
  },
  "VirtualAddressList": "",
  "ServiceDiscoveryPort": 50009,
  "ResourceMonitorTopic": "process",
  "UpdateServerAddressInterval": "10m",
  "Redis": {
    "Port": 6379,
    "Password": "",
    "DB": 10
  },
  "PrivateIPAddressPattern": "",
  "SocksProxy": {
    "Host": "",
    "Port": 0
  }
}
```

### Monitor.json

```json
{
  "Collectors": {
    "cpu":              { "Enabled": true, "Interval": "10s" },
    "memory":           { "Enabled": true, "Interval": "10s" },
    "disk":             { "Enabled": true, "Interval": "30s", "Disks": [] },
    "network":          { "Enabled": true, "Interval": "10s", "Interfaces": [] },
    "temperature":      { "Enabled": true, "Interval": "30s", "IncludeZones": [] },
    "cpu_process":      { "Enabled": true, "Interval": "30s", "TopN": 10, "WatchProcesses": [] },
    "memory_process":   { "Enabled": true, "Interval": "30s", "TopN": 10, "WatchProcesses": [] },
    "fan":              { "Enabled": true, "Interval": "30s", "IncludeZones": [] },
    "gpu":              { "Enabled": true, "Interval": "30s", "IncludeZones": [] },
    "voltage":          { "Enabled": true, "Interval": "30s", "IncludeZones": [] },
    "motherboard_temp": { "Enabled": true, "Interval": "30s", "IncludeZones": [] },
    "storage_smart":    { "Enabled": true, "Interval": "60s", "Disks": [] },
    "process_watch":    { "Enabled": true, "Interval": "60s", "RequiredProcesses": [], "ForbiddenProcesses": [] }
  }
}
```

### Logging.json

```json
{
  "Level": "info",
  "FilePath": "log/ResourceAgent/ResourceAgent.log",
  "MaxSizeMB": 10,
  "MaxBackups": 5,
  "MaxAgeDays": 30,
  "Compress": true,
  "Console": false
}
```

### 주요 설정 항목

| 항목 | 설명 | 기본값 |
|------|------|--------|
| `SenderType` | 전송 방식 (`file`, `kafka`, `kafkarest`) | `kafka` |
| `File.Format` | 파일 출력 형식 (`json`, `grok`, `legacy`→`grok` 자동 매핑) | `grok` |
| `File.Console` | 콘솔에도 메트릭 출력 | `true` |
| `Redis.Password` | Redis 접속 암호 (비어있으면 기본 암호 사용) | `visuallove` |
| `UpdateServerAddressInterval` | ServiceDiscovery 주소 갱신 주기 (Go duration, 음수=비활성화) | `10m` |
| `Collectors.*.Enabled` | Collector 활성화 여부 | `true` |
| `Collectors.*.Interval` | 수집 주기 | 10s~60s |
| `Collectors.*.TopN` | 프로세스 모니터링 상위 N개 | `10` |
| `Collectors.*.WatchProcesses` | 항상 추적할 프로세스 이름 목록 | `[]` |
| `Collectors.*.Interfaces` | 모니터링 대상 NIC 지정 (빈 배열=전체) | `[]` |
| `Collectors.*.Disks` | 모니터링 대상 디스크/파티션 지정 | `[]` |

### Sender 타입별 동작

| SenderType | 전송 대상 | 포맷 | 인프라 연동 |
|------------|-----------|------|:-----------:|
| `file` | 로컬 파일 | JSON(ParsedDataList) 또는 EARS Grok 평문 | - |
| `kafkarest` | KafkaRest Proxy (HTTP) | EARS Grok 평문 | ServiceDiscovery, Redis |
| `kafka` | Kafka 직접 연결 (sarama) | EARS JSON (ParsedData) 또는 MetricData JSON | Redis (optional) |

## CLI 플래그

```bash
resourceagent [flags]

  -config string    ResourceAgent.json 경로 (기본값: "conf/ResourceAgent/ResourceAgent.json")
  -monitor string   Monitor.json 경로 (기본값: "conf/ResourceAgent/Monitor.json")
  -logging string   Logging.json 경로 (기본값: "conf/ResourceAgent/Logging.json")
  -version          버전 정보 출력
```

## 설치

ResourceAgent는 ARSAgent와 공유 basePath에 통합 배포됩니다.

### 통합 배포 구조

```
<BasePath>\                               ← ARSAgent basePath (예: D:\EARS\EEGAgent)
├── bin\x86\
│   ├── earsagent.exe                     # ARSAgent 바이너리 (기존)
│   └── ResourceAgent.exe                 # ResourceAgent 바이너리
├── conf\
│   ├── ARSAgent\                         # ARSAgent 설정 (기존)
│   └── ResourceAgent\                    # ResourceAgent 설정
│       ├── ResourceAgent.json
│       ├── Monitor.json
│       └── Logging.json
├── log\
│   ├── ARSAgent\                         # ARSAgent 로그 (기존)
│   └── ResourceAgent\                    # ResourceAgent 로그
│       ├── ResourceAgent.log
│       └── metrics.jsonl
└── utils\
    └── lhm-helper\                       # Windows 전용
        ├── LhmHelper.exe                 # 하드웨어 센서 헬퍼 (.NET Framework 4.7, 단일 exe)
        ├── LhmHelper.exe.config
        └── PawnIO_setup.exe              # 드라이버 설치/제거 (Win8+만)
```

### Windows 설치 패키지 만들기

개발 PC에서 패키지를 생성하고, 현장 PC에 배포합니다.

```bash
# 64-bit: 빌드 + 패키징 (Go 1.20 자동 사용, LhmHelper 포함)
./scripts/package.sh --build --lhmhelper

# 64-bit: 빌드 + 패키징 (LhmHelper 없이)
./scripts/package.sh --build

# 32-bit: 빌드 + 패키징 (Windows 7 32-bit용, LhmHelper 자동 제외)
./scripts/package.sh --build --arch 386

# 이미 빌드된 바이너리로 패키징만
./scripts/package.sh --lhmhelper
```

Windows에서 패키지 생성 시:

```powershell
# 64-bit: 빌드 + 패키징 (Go 1.20 자동 사용, LhmHelper 포함)
.\scripts\package.ps1 -Build -IncludeLhmHelper

# 32-bit: 빌드 + 패키징 (Windows 7 32-bit용, LhmHelper 자동 제외)
.\scripts\package.ps1 -Build -Arch 386

# 이미 빌드된 바이너리로 패키징만
.\scripts\package.ps1 -IncludeLhmHelper
```

생성되는 패키지 구조:

```
install_package_windows/                 ← 64-bit (기본)
├── INSTALL_GUIDE.txt
├── install_ResourceAgent.bat
├── install_ResourceAgent.ps1
├── bin\x86\
│   └── ResourceAgent.exe                # 64-bit 바이너리
├── conf\ResourceAgent\
│   ├── ResourceAgent.json
│   ├── Monitor.json
│   └── Logging.json
└── utils\lhm-helper\                   (--lhmhelper 옵션 시)
    ├── LhmHelper.exe                   # net47 단일 exe (의존성 embed)
    ├── LhmHelper.exe.config
    └── PawnIO_setup.exe

install_package_ndp48/                   ← .NET 4.8 설치 전용 별도 패키지 (옵션)
├── NDP48-x86-x64-AllOS-ENU.exe          # ~112MB (현장 PC에 4.7 미만일 때만 필요)
├── install_ndp48.bat                    # 관리자가 수동 실행
└── README.txt

install_package_windows_x86/             ← 32-bit (--arch 386)
├── INSTALL_GUIDE.txt
├── install_ResourceAgent.bat
├── install_ResourceAgent.ps1
├── bin\x86\
│   └── ResourceAgent.exe                # 32-bit 바이너리
└── conf\ResourceAgent\
    ├── ResourceAgent.json
    ├── Monitor.json
    └── Logging.json
    (LhmHelper 미포함 — 64-bit 전용)
```

> 각각 `.zip`도 함께 생성됩니다. 현장 PC에 zip을 복사 후 압축 해제하여 사용합니다.

**32-bit 패키지 참고사항:**
- LhmHelper는 이제 AnyCPU로 빌드되어 32-bit Windows에서도 실행 가능하나, 현재 패키지는 64-bit에만 포함됩니다 (안전 유지).
- 32-bit Windows에서 LhmHelper 포함 배포는 향후 별도 검증 후 지원 예정.
- 32-bit Windows에서는 하드웨어 센서(온도, GPU, 팬, 전압, 메인보드 온도, S.M.A.R.T) 수집이 불가합니다.
- CPU, Memory, Disk, Network, Uptime, ProcessWatch 등 OS 메트릭은 정상 수집됩니다.
- 설치 스크립트(`install_ResourceAgent.bat`)는 64-bit/32-bit 패키지 모두 동일하게 동작합니다.

### Windows 설치

패키지 압축 해제 후, 관리자 권한으로 실행 (우클릭 → "관리자 권한으로 실행"):

```cmd
REM 기본 설치 (BasePath: D:\EARS\EEGAgent)
install_ResourceAgent.bat

REM BasePath 지정
install_ResourceAgent.bat /basepath D:\EARS\EEGAgent

REM LhmHelper 제외 설치
install_ResourceAgent.bat /nolhm

REM VirtualAddressList 직접 지정
install_ResourceAgent.bat /site 10.0.0.1,10.0.0.2

REM 파일이 이미 복사되어 있는 경우 (원격 배포 등)
install_ResourceAgent.bat /basepath D:\EARS\EEGAgent /nocopy /site 10.0.0.1

REM 옵션
REM   /basepath PATH    BasePath 지정 (ARSAgent 서비스에서 자동 감지)
REM   /nolhm            LhmHelper + PawnIO 드라이버 제외
REM   /site ADDR        VirtualAddressList 직접 지정
REM   /nocopy           파일 복사 생략 (서비스 등록만 수행)
REM   /uninstall        제거
```

또는 PowerShell:

```powershell
.\install_ResourceAgent.ps1
.\install_ResourceAgent.ps1 -BasePath "D:\EARS\EEGAgent" -IncludeLhmHelper
.\install_ResourceAgent.ps1 -BasePath "D:\EARS\EEGAgent" -Site "10.0.0.1" -NoCopy
```

> 설정 파일(`conf/ResourceAgent/*.json`)은 대상 경로에 이미 존재하면 덮어쓰지 않습니다. 바이너리만 업데이트됩니다.

### Linux (systemd)

```bash
# 기본 설치 (basePath: /opt/EEGAgent)
sudo ./scripts/install_ResourceAgent.sh

# basePath 지정
sudo ./scripts/install_ResourceAgent.sh --base-path /opt/EEGAgent

# VirtualAddressList 직접 지정
sudo ./scripts/install_ResourceAgent.sh --base-path /opt/EEGAgent --site 10.0.0.1

# 파일이 이미 복사되어 있는 경우 (원격 배포 등)
sudo ./scripts/install_ResourceAgent.sh --base-path /opt/EEGAgent --nocopy --site 10.0.0.1

# 옵션
#   --base-path PATH    BASE 경로 지정 (기본값: /opt/EEGAgent)
#   --user USER         서비스 사용자 지정 (기본값: resourceagent)
#   --site ADDR         VirtualAddressList 직접 지정
#   --nocopy            파일 복사 생략 (서비스 등록만 수행)
#   --uninstall         제거
```

또는 수동 설치:

```bash
# 1. 디렉토리 생성 및 파일 복사
BASE_PATH=/opt/EEGAgent
sudo mkdir -p $BASE_PATH/{bin/x86,conf/ResourceAgent,log/ResourceAgent}
sudo cp resourceagent $BASE_PATH/bin/x86/
sudo cp conf/ResourceAgent/*.json $BASE_PATH/conf/ResourceAgent/

# 2. 서비스 등록
sudo cp scripts/resourceagent.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable resourceagent
sudo systemctl start resourceagent

# 3. 상태 확인
sudo systemctl status resourceagent
```

> systemd 서비스 파일은 `/opt/EEGAgent` basePath를 기본으로 사용합니다. 다른 경로를 사용하는 경우 `resourceagent.service`의 `WorkingDirectory`와 `ExecStart` 경로를 수정하세요.

## 제거 (Uninstall)

### Linux

```bash
sudo ./scripts/install_ResourceAgent.sh --uninstall
```

또는 수동 제거:

```bash
# 서비스 중지 및 제거
sudo systemctl stop resourceagent
sudo systemctl disable resourceagent
sudo rm /etc/systemd/system/resourceagent.service
sudo systemctl daemon-reload

# ResourceAgent 파일만 삭제 (ARSAgent는 유지)
BASE_PATH=/opt/EEGAgent
sudo rm -f $BASE_PATH/bin/x86/resourceagent
sudo rm -rf $BASE_PATH/conf/ResourceAgent
sudo rm -rf $BASE_PATH/log/ResourceAgent

# 서비스 사용자 삭제 (선택)
sudo userdel resourceagent
```

### Windows

패키지 폴더에서 관리자 권한으로 실행:

```cmd
install_ResourceAgent.bat /uninstall
```

또는 PowerShell: `.\install_ResourceAgent.ps1 -Uninstall`

> LhmHelper가 설치된 경우(`/nolhm` 없이 설치), PawnIO 드라이버도 자동으로 제거됩니다.

수동 제거:

```powershell
# 1. 서비스 중지 및 제거
sc.exe stop ResourceAgent
sc.exe delete ResourceAgent

# 2. ResourceAgent 파일만 삭제 (ARSAgent는 유지)
$BasePath = "D:\EARS\EEGAgent"
Remove-Item -Path "$BasePath\bin\x86\ResourceAgent.exe" -Force
Remove-Item -Path "$BasePath\conf\ResourceAgent" -Recurse -Force
Remove-Item -Path "$BasePath\log\ResourceAgent" -Recurse -Force
Remove-Item -Path "$BasePath\utils\lhm-helper" -Recurse -Force -ErrorAction SilentlyContinue

# 3. PawnIO 드라이버 제거 (LhmHelper 사용 시)
# 제어판 → 프로그램 추가/제거 → "PawnIO" 제거
# 또는 사일런트 제거:
PawnIO_setup.exe /S /uninstall
```

> 단독 실행(서비스 미등록)의 경우 해당 디렉토리만 삭제하면 됩니다.

## 단독 실행 (서비스 등록 없이)

서비스로 등록하지 않고 단독 실행할 수 있습니다. 통합 배포 구조의 basePath에서 실행합니다:

```cmd
cd D:\EARS\EEGAgent
bin\x86\ResourceAgent.exe
```

설정 파일 기본 경로가 `conf/ResourceAgent/ResourceAgent.json` 등 상대 경로이므로, basePath에서 실행하면 플래그 없이 바로 동작합니다.

경로를 직접 지정하려면:

```cmd
ResourceAgent.exe -config conf\ResourceAgent\ResourceAgent.json -monitor conf\ResourceAgent\Monitor.json -logging conf\ResourceAgent\Logging.json
```

> 상대 경로는 작업 디렉토리(cwd) 기준입니다. 다른 경로에서 실행할 경우 절대 경로를 지정하세요.

## 로컬 테스트 (Kafka 없이)

Kafka/인프라 연결 없이 로컬에서 테스트하려면 `SenderType`을 `file`로 설정합니다.

### 설정

`conf/ResourceAgent/ResourceAgent.json`에서:

```json
{
  "SenderType": "file",
  "File": {
    "FilePath": "log/ResourceAgent/metrics.jsonl",
    "Console": true,
    "Pretty": false,
    "Format": "json"
  }
}
```

`Format` 옵션:
- `json` — ParsedDataList JSON 형식 (Kafka direct/JSON mapper와 동일)
- `grok` — EARS Grok 평문 형식 (`category:cpu,pid:0,proc:@system,metric:total_used_pct,value:45.5`)
- `legacy` — `grok`의 별칭 (하위호환)

### 실행

```bash
# 기본 경로 사용 (Linux/macOS, basePath에서 실행)
./bin/x86/resourceagent

# 설정 파일 경로 지정
./bin/x86/resourceagent -config conf/ResourceAgent/ResourceAgent.json -monitor conf/ResourceAgent/Monitor.json -logging conf/ResourceAgent/Logging.json
```

### 출력 확인

```bash
# 콘솔 출력 (Console: true 일 때 실행하면 바로 출력)
./bin/x86/resourceagent

# 로그 파일 확인
tail -f log/ResourceAgent/metrics.jsonl
```

### 출력 예시

**JSON 형식** (`Format: "json"`) — ParsedDataList (Kafka direct/JSON mapper와 동일):
```json
{"iso_timestamp":"2026-02-25T10:30:45.123","parsed":[{"field":"EARS_PROCESS","value":"","dataformat":"String"},{"field":"EARS_CATEGORY","value":"cpu","dataformat":"String"},{"field":"EARS_PID","value":"0","dataformat":"Integer"},{"field":"EARS_PROCNAME","value":"@system","dataformat":"String"},{"field":"EARS_METRIC","value":"total_used_pct","dataformat":"String"},{"field":"EARS_VALUE","value":"15.5","dataformat":"Double"}]}
```

**Grok 형식** (`Format: "grok"`):
```
2026-02-25 10:30:45,123 category:cpu,pid:0,proc:@system,metric:total_used_pct,value:15.5
2026-02-25 10:30:45,123 category:memory,pid:0,proc:@system,metric:total_used_pct,value:75
2026-02-25 10:30:45,123 category:disk,pid:0,proc:@system,metric:C:,value:60
2026-02-25 10:30:45,123 category:network,pid:0,proc:@system,metric:all_inbound,value:42
2026-02-25 10:30:45,123 category:network,pid:0,proc:Ethernet,metric:recv_rate,value:12345.6
```

> EARS Grok 포맷 상세 명세는 [`docs/EARS-METRICS-REFERENCE.md`](docs/EARS-METRICS-REFERENCE.md) 참조.
> 15종 Collector 상세 설명은 [`docs/COLLECTORS.md`](docs/COLLECTORS.md) 참조.

## 운영

### 로그 확인

```bash
# Linux (systemd journal)
journalctl -u resourceagent -f

# 로그 파일 직접 확인
tail -f /opt/EEGAgent/log/ResourceAgent/ResourceAgent.log
```

### 서비스 관리

```bash
# Linux
sudo systemctl start resourceagent
sudo systemctl stop resourceagent
sudo systemctl restart resourceagent
sudo systemctl status resourceagent

# Windows
sc.exe start ResourceAgent
sc.exe stop ResourceAgent
sc.exe query ResourceAgent
```

### 설정 변경 (Hot Reload)

`Monitor.json` 또는 `Logging.json`을 수정하면 서비스 재시작 없이 자동 반영됩니다.

```bash
# Collector 수집 주기 변경 예시
vi /opt/EEGAgent/conf/ResourceAgent/Monitor.json
# 로그에서 "Monitor configuration updated" 메시지 확인

# 로그 레벨 변경 예시
vi /opt/EEGAgent/conf/ResourceAgent/Logging.json
# 로그에서 "Logging configuration updated" 메시지 확인
```

> `ResourceAgent.json` 변경은 서비스 재시작이 필요합니다.

## Windows 하드웨어 모니터링

Windows에서 온도, 팬, GPU, 전압, 메인보드 온도, 스토리지 S.M.A.R.T를 수집하려면 LhmHelper가 필요합니다.

### 구조

```
ResourceAgent (Go) → LhmHelper.exe (C#) → LibreHardwareMonitorLib → 커널 드라이버 → MSR/SMBus
```

LHM은 하드웨어 접근 드라이버를 자동으로 선택합니다:

| OS | 드라이버 | 설명 |
|---|---|---|
| Windows 8+ | **PawnIO** | Microsoft WHQL 서명, `install_ResourceAgent.bat`이 자동 설치 |
| Windows 7 | **WinRing0** (자동 폴백) | LHM에 내장, PawnIO 설치 불필요 |

### 필요 파일

1. **LhmHelper.exe** — C# LibreHardwareMonitor 헬퍼 (.NET Framework 4.7 타겟, Costura.Fody로 의존 DLL 내장)
2. **.NET Framework 4.7 런타임** — Windows 7 공장 PC는 대부분 기본 설치되어 있음 (추가 설치 불필요)
3. **PawnIO 드라이버** (Windows 8+ 전용) — 하드웨어 접근 드라이버 (Microsoft 서명)

> Windows 7에서는 PawnIO가 지원되지 않으며, `install_ResourceAgent.bat`이 OS 버전을 감지하여 자동으로 설치를 건너뜁니다.
> LHM이 내장 WinRing0 드라이버로 폴백하여 온도/GPU 등 하드웨어 센서를 정상 수집합니다.

### PawnIO 드라이버 설치

`install_ResourceAgent.bat` 실행 시 Windows 8+에서는 PawnIO 드라이버가 자동 설치됩니다 (`/nolhm` 옵션으로 제외 가능).

수동 설치가 필요한 경우:

```cmd
PawnIO_setup.exe /S
```

재부팅 불필요.

### 수집 항목

| Collector | 수집 데이터 | LhmHelper 필요 |
|-----------|-------------|:--------------:|
| temperature | CPU 패키지/코어 온도 | O (Windows) |
| fan | 팬 RPM | O |
| gpu | 온도, 코어/메모리 사용률, 팬, 전력, 클럭 | O |
| voltage | CPU/메모리 전압 | O |
| motherboard_temp | 메인보드 온도 | O |
| storage_smart | 온도, 잔여 수명, 에러, 전원 사이클, 가동 시간, 기록량 | O |

> Linux에서는 gopsutil을 통해 `/sys/class/thermal/` 등에서 온도를 수집합니다. LhmHelper 불필요.

## 메트릭 레퍼런스

### 레코드 형식

```
{timestamp} category:{category},pid:{pid},proc:{proc},metric:{metric},value:{value}
```

- `proc`: 시스템 메트릭은 `@system`, 프로세스/인터페이스별 메트릭은 해당 이름
- `pid`: 프로세스 메트릭은 PID, 시스템 메트릭은 `0`
- `{...}`: 시스템 환경에 따라 동적으로 결정되는 값

### 전체 수집 항목

| Category | ProcName | Metric | 설명 | 단위 |
|---|---|---|---|---|
| cpu | @system | `total_used_pct` | 전체 CPU 사용률 | % |
| cpu | @system | `core_{N}_used_pct` | 코어별 사용률 (N=0~) | % |
| cpu | {process} | `used_pct` | 프로세스 CPU 사용률 | % |
| memory | @system | `total_used_pct` | 메모리 사용률 | % |
| memory | @system | `total_free_pct` | 메모리 여유률 | % |
| memory | @system | `total_used_size` | 사용량 | bytes |
| memory | {process} | `used` | 프로세스 RSS | bytes |
| disk | @system | `{mountpoint}` | 파티션 사용률 | % |
| network | @system | `all_inbound` | TCP 인바운드 연결 수 | count |
| network | @system | `all_outbound` | TCP 아웃바운드 연결 수 | count |
| network | {interface} | `recv_rate` | 수신 속도 | bytes/s |
| network | {interface} | `sent_rate` | 송신 속도 | bytes/s |
| temperature | @system | `{sensor}` | CPU 온도 | °C |
| gpu | @system | `{gpu}_temperature` | GPU 온도 | °C |
| gpu | @system | `{gpu}_core_load` | GPU 코어 부하 | % |
| gpu | @system | `{gpu}_memory_load` | GPU 메모리 부하 | % |
| gpu | @system | `{gpu}_fan_speed` | GPU 팬 속도 | RPM |
| gpu | @system | `{gpu}_power` | GPU 전력 | W |
| gpu | @system | `{gpu}_core_clock` | GPU 코어 클럭 | MHz |
| gpu | @system | `{gpu}_memory_clock` | GPU 메모리 클럭 | MHz |
| fan | @system | `{sensor}` | 팬 속도 | RPM |
| voltage | @system | `{sensor}` | 전압 | V |
| motherboard_temp | @system | `{sensor}` | 메인보드 온도 | °C |
| storage_smart | @system | `{storage}_temperature` | 스토리지 온도 | °C |
| storage_smart | @system | `{storage}_remaining_life` | 수명 잔량 | % |
| storage_smart | @system | `{storage}_media_errors` | 미디어 에러 | count |
| storage_smart | @system | `{storage}_power_cycles` | 전원 사이클 | count |
| storage_smart | @system | `{storage}_unsafe_shutdowns` | 비정상 종료 | count |
| storage_smart | @system | `{storage}_power_on_hours` | 사용 시간 | hours |
| storage_smart | @system | `{storage}_total_bytes_written` | 총 기록량 | bytes |
| process_watch | {process} | `required` / `required_alert` | 필수 프로세스 상태 | 1/0 |
| process_watch | {process} | `forbidden` / `forbidden_alert` | 금지 프로세스 상태 | 1/0 |
| uptime | @system | `boot_time_unix` | 부팅 시각 | unix ts |
| uptime | @system | `uptime_minutes` | 가동 시간 | min |

## 문제 해결

### Kafka 연결 실패

```
failed to create Kafka producer: kafka: client has run out of available brokers
```

- Kafka 브로커 주소 확인
- 네트워크 연결 확인
- 로컬 테스트 시 `SenderType: "file"` 사용

### 온도/하드웨어 센서 수집 실패

- Windows: LhmHelper.exe가 `<BasePath>\utils\lhm-helper\`에 있는지 확인
- Windows 8+: PawnIO 드라이버 설치 여부 확인 (`sc.exe query PawnIO`)
- Windows 7: PawnIO 불필요 — LHM이 WinRing0로 자동 폴백 (Defender 차단 시 화이트리스트 필요)
- macOS: 온도 센서 접근 제한 (개발 테스트용이므로 무시 가능)
- 다른 Collector에는 영향 없음 (Collector 격리)

### 권한 문제 (Linux)

```bash
chmod +x /opt/EEGAgent/bin/x86/resourceagent
mkdir -p /opt/EEGAgent/log/ResourceAgent
chown -R resourceagent:resourceagent /opt/EEGAgent/log/ResourceAgent
```

## 라이선스

MIT License
