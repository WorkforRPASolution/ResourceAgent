# ResourceAgent

공장 내 PC(10,000대 이상)의 하드웨어 자원 사용률을 수집하는 Go 기반 경량 모니터링 에이전트입니다.

## 주요 기능

- **14종 메트릭 수집**: CPU, Memory, Disk, Network, Temperature, Fan, GPU, Voltage, Motherboard Temperature, Storage S.M.A.R.T, Uptime, ProcessWatch, 프로세스 CPU/Memory
- **Windows 하드웨어 모니터링**: LibreHardwareMonitor (LhmHelper) 연동으로 온도, 팬, GPU, 전압, 메인보드 온도, 스토리지 S.M.A.R.T 수집
- **유연한 전송 방식**: `file`, `kafka`, `kafkarest` 3가지 sender 지원
- **EARS 호환 포맷**: ARSAgent 호환 legacy 평문 및 JSON 파싱 포맷 지원
- **3분할 설정**: ResourceAgent.json / Monitor.json / Logging.json 독립 관리
- **Hot Reload**: Monitor.json, Logging.json 변경 시 서비스 재시작 없이 자동 반영
- **프로세스 모니터링**: CPU/Memory 상위 N개 프로세스 + 지정 프로세스 추적
- **크로스 플랫폼**: Windows, Linux 지원 (macOS 개발/테스트용)

## 시스템 요구사항

### 지원 플랫폼
- Windows 10+, Windows Server 2016+
- Ubuntu 18.04+, CentOS 7+
- macOS (개발/테스트용, 하드웨어 센서 제한)

### 리소스 사용량
- CPU: 유휴 시 1% 미만
- Memory: 50MB 이하
- 바이너리 크기: 20MB 이하

## 빌드

### 사전 요구사항
- Go 1.24 이상
- (Windows 하드웨어 모니터링 시) .NET 8 SDK

### ResourceAgent 빌드

```bash
# 의존성 다운로드
go mod tidy

# Linux 빌드
GOOS=linux GOARCH=amd64 go build -o resourceagent ./cmd/resourceagent

# Windows 빌드
GOOS=windows GOARCH=amd64 go build -o ResourceAgent.exe ./cmd/resourceagent

# 버전 정보 포함 빌드
go build -ldflags "-X main.version=1.0.0 -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o ResourceAgent.exe ./cmd/resourceagent
```

### LhmHelper 빌드 (Windows 하드웨어 모니터링)

Windows에서 온도, 팬, GPU, 전압, 메인보드 온도, 스토리지 S.M.A.R.T를 수집하려면 LhmHelper가 필요합니다.

```bash
cd utils/lhm-helper
dotnet publish -c Release -r win-x64 --self-contained
# 출력: bin/Release/net8.0/win-x64/publish/LhmHelper.exe
```

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
    "Format": "legacy"
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
| `File.Format` | 파일 출력 형식 (`json`, `legacy`) | `legacy` |
| `File.Console` | 콘솔에도 메트릭 출력 | `true` |
| `Redis.Password` | Redis 접속 암호 (비어있으면 기본 암호 사용) | `visuallove` |
| `Collectors.*.Enabled` | Collector 활성화 여부 | `true` |
| `Collectors.*.Interval` | 수집 주기 | 10s~60s |
| `Collectors.*.TopN` | 프로세스 모니터링 상위 N개 | `10` |
| `Collectors.*.WatchProcesses` | 항상 추적할 프로세스 이름 목록 | `[]` |
| `Collectors.*.Interfaces` | 모니터링 대상 NIC 지정 (빈 배열=전체) | `[]` |
| `Collectors.*.Disks` | 모니터링 대상 디스크/파티션 지정 | `[]` |

### Sender 타입별 동작

| SenderType | 전송 대상 | 포맷 | 인프라 연동 |
|------------|-----------|------|:-----------:|
| `file` | 로컬 파일 | JSON 또는 EARS legacy 평문 | - |
| `kafkarest` | KafkaRest Proxy (HTTP) | EARS legacy 평문 | ServiceDiscovery, Redis |
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
        ├── LhmHelper.exe                 # 하드웨어 센서 헬퍼
        └── PawnIO_setup.exe              # 드라이버 설치/제거
```

### Windows 설치 패키지 만들기

개발 PC에서 패키지를 생성하고, 현장 PC에 배포합니다.

```bash
# 패키지 생성 (LhmHelper 없이)
./scripts/package.sh

# LhmHelper + PawnIO 포함 패키지 생성
./scripts/package.sh --lhmhelper
```

Windows에서 패키지 생성 시:

```powershell
# 패키지 생성 (LhmHelper 없이)
.\scripts\package.ps1

# LhmHelper + PawnIO 포함 패키지 생성
.\scripts\package.ps1 -IncludeLhmHelper
```

생성되는 패키지 구조:

```
install_package_windows/
├── INSTALL_GUIDE.txt                    # 설치 가이드 (현장 담당자용)
├── install.bat                          # 설치 스크립트
├── install.ps1                          # PowerShell 설치 스크립트
├── bin\x86\
│   └── ResourceAgent.exe
├── conf\ResourceAgent\
│   ├── ResourceAgent.json
│   ├── Monitor.json
│   └── Logging.json
└── utils\lhm-helper\                   (--lhmhelper 옵션 시)
    ├── LhmHelper.exe
    └── PawnIO_setup.exe
```

> `install_package_windows.zip`도 함께 생성됩니다. 현장 PC에 zip을 복사 후 압축 해제하여 사용합니다.

### Windows 설치

패키지 압축 해제 후, 관리자 권한으로 실행 (우클릭 → "관리자 권한으로 실행"):

```cmd
REM 기본 설치 (BasePath: D:\EARS\EEGAgent)
install.bat

REM BasePath 지정
install.bat /basepath D:\EARS\EEGAgent

REM LhmHelper + PawnIO 포함 설치
install.bat /lhmhelper

REM 옵션
REM   /basepath PATH    BasePath 지정 (기본값: D:\EARS\EEGAgent)
REM   /lhmhelper        LhmHelper + PawnIO 드라이버 설치
REM   /uninstall        제거
```

또는 PowerShell:

```powershell
.\install.ps1
.\install.ps1 -BasePath "D:\EARS\EEGAgent" -IncludeLhmHelper
```

> 설정 파일(`conf/ResourceAgent/*.json`)은 대상 경로에 이미 존재하면 덮어쓰지 않습니다. 바이너리만 업데이트됩니다.

### Linux (systemd)

```bash
# 기본 설치 (basePath: /opt/EEGAgent)
sudo ./scripts/install.sh

# basePath 지정
sudo ./scripts/install.sh --base-path /opt/EEGAgent

# 옵션
#   --base-path PATH    BASE 경로 지정 (기본값: /opt/EEGAgent)
#   --user USER         서비스 사용자 지정 (기본값: resourceagent)
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
sudo ./scripts/install.sh --uninstall
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
install.bat /uninstall
```

또는 PowerShell: `.\install.ps1 -Uninstall`

> `/lhmhelper`로 설치한 경우, PawnIO 드라이버도 자동으로 제거됩니다.

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
- `json` — MetricData JSON 형식 (구조화된 데이터)
- `legacy` — EARS 평문 형식 (`category:cpu,pid:0,proc:@system,metric:total_used_pct,value:45.5`)

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

**JSON 형식** (`Format: "json"`):
```json
{"type":"cpu","timestamp":"2026-02-25T10:30:45+09:00","agent_id":"my-pc","hostname":"my-pc","data":{"usage_percent":15.5,"core_count":8,"per_core_percent":[12.3,18.7]}}
```

**Legacy 형식** (`Format: "legacy"`):
```
2026-02-25 10:30:45,123 category:cpu,pid:0,proc:@system,metric:total_used_pct,value:15.5
2026-02-25 10:30:45,123 category:memory,pid:0,proc:@system,metric:total_used_pct,value:75
2026-02-25 10:30:45,123 category:disk,pid:0,proc:@system,metric:C:,value:60
2026-02-25 10:30:45,123 category:network,pid:0,proc:@system,metric:all_inbound,value:42
2026-02-25 10:30:45,123 category:network,pid:0,proc:Ethernet,metric:recv_rate,value:12345.6
```

> EARS legacy 포맷 상세 명세는 [`docs/EARS-METRICS-REFERENCE.md`](docs/EARS-METRICS-REFERENCE.md) 참조.
> 14종 Collector 상세 설명은 [`docs/COLLECTORS.md`](docs/COLLECTORS.md) 참조.

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
ResourceAgent (Go) → LhmHelper.exe (C#) → LibreHardwareMonitorLib → PawnIO Driver → MSR/SMBus
```

### 필요 파일

1. **LhmHelper.exe** — C# LibreHardwareMonitor 헬퍼 (~60-80MB, self-contained)
2. **PawnIO 드라이버** — 하드웨어 접근 드라이버 (WinRing0 대체, Microsoft 서명)

### PawnIO 드라이버 설치

`install.bat /lhmhelper` 사용 시 PawnIO 드라이버가 자동으로 설치됩니다.

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
- Windows: PawnIO 드라이버 설치 여부 확인 (`sc.exe query PawnIO`)
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
