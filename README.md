# ResourceAgent

공장 내 PC(10,000대 이상)의 하드웨어 자원 사용률을 수집하는 Go 기반 경량 모니터링 에이전트입니다.

## 주요 기능

- **12종 메트릭 수집**: CPU, Memory, Disk, Network, Temperature, Fan, GPU, Voltage, Motherboard Temperature, Storage S.M.A.R.T, 프로세스 CPU/Memory
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
GOOS=windows GOARCH=amd64 go build -o resourceagent.exe ./cmd/resourceagent

# 버전 정보 포함 빌드
go build -ldflags "-X main.version=1.0.0 -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o resourceagent.exe ./cmd/resourceagent
```

### LhmHelper 빌드 (Windows 하드웨어 모니터링)

Windows에서 온도, 팬, GPU, 전압, 메인보드 온도, 스토리지 S.M.A.R.T를 수집하려면 LhmHelper가 필요합니다.

```bash
cd tools/lhm-helper
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
    "FilePath": "logs/metrics.jsonl",
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
  "ResourceMonitorTopic": "all",
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
    "storage_smart":    { "Enabled": true, "Interval": "60s", "Disks": [] }
  }
}
```

### Logging.json

```json
{
  "Level": "info",
  "FilePath": "logs/agent.log",
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

  -config string    ResourceAgent.json 경로 (기본값: "configs/ResourceAgent.json")
  -monitor string   Monitor.json 경로 (기본값: "configs/Monitor.json")
  -logging string   Logging.json 경로 (기본값: "configs/Logging.json")
  -version          버전 정보 출력
```

## 설치

### 배포 파일 목록

| 파일 | 필수 | 설명 |
|------|:----:|------|
| `resourceagent.exe` (또는 `resourceagent`) | O | Go 바이너리 |
| `configs/ResourceAgent.json` | O | 에이전트/sender 설정 |
| `configs/Monitor.json` | O | Collector 설정 |
| `configs/Logging.json` | O | 로깅 설정 |
| `LhmHelper.exe` | - | Windows 하드웨어 센서 수집용 (temperature, fan, gpu, voltage, motherboard_temp, storage_smart) |
| PawnIO 드라이버 | - | LhmHelper 하드웨어 접근 드라이버 (`PawnIO_setup.exe /S`로 설치) |

### Linux (systemd)

```bash
# 1. 바이너리 및 설정 복사
sudo mkdir -p /opt/resourceagent/{configs,logs}
sudo cp resourceagent /opt/resourceagent/
sudo cp configs/ResourceAgent.json /opt/resourceagent/configs/
sudo cp configs/Monitor.json /opt/resourceagent/configs/
sudo cp configs/Logging.json /opt/resourceagent/configs/

# 2. 서비스 등록
sudo cp scripts/resourceagent.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable resourceagent
sudo systemctl start resourceagent

# 3. 상태 확인
sudo systemctl status resourceagent
```

또는 설치 스크립트 사용:

```bash
sudo ./scripts/install.sh
```

### Windows

```powershell
# 1. 폴더 생성 및 파일 복사
$InstallPath = "C:\Program Files\ResourceAgent"
New-Item -ItemType Directory -Path "$InstallPath\configs" -Force
New-Item -ItemType Directory -Path "$InstallPath\logs" -Force
Copy-Item resourceagent.exe "$InstallPath\"
Copy-Item configs\ResourceAgent.json "$InstallPath\configs\"
Copy-Item configs\Monitor.json "$InstallPath\configs\"
Copy-Item configs\Logging.json "$InstallPath\configs\"

# (선택) LhmHelper 복사 - 하드웨어 센서 수집 시 필요
Copy-Item LhmHelper.exe "$InstallPath\"

# 2. 서비스 등록
sc.exe create ResourceAgent binPath= "`"$InstallPath\resourceagent.exe`"" start= auto
sc.exe start ResourceAgent

# 3. 상태 확인
sc.exe query ResourceAgent
```

또는 설치 스크립트 사용 (관리자 권한):

```powershell
.\scripts\install.ps1
```

## 제거 (Uninstall)

### Linux

```bash
# 서비스 중지 및 제거
sudo systemctl stop resourceagent
sudo systemctl disable resourceagent
sudo rm /etc/systemd/system/resourceagent.service
sudo systemctl daemon-reload

# 설치 디렉토리 삭제
sudo rm -rf /opt/resourceagent

# 서비스 사용자 삭제 (선택)
sudo userdel resourceagent
```

또는 설치 스크립트 사용:

```bash
sudo ./scripts/install.sh --uninstall
```

### Windows

```powershell
# 1. 서비스 중지 및 제거
sc.exe stop ResourceAgent
sc.exe delete ResourceAgent

# 2. 설치 디렉토리 삭제
Remove-Item -Path "C:\Program Files\ResourceAgent" -Recurse -Force

# 3. PawnIO 드라이버 제거 (LhmHelper 사용 시)
# 제어판 → 프로그램 추가/제거 → "PawnIO" 제거
# 또는 사일런트 제거:
PawnIO_setup.exe /S /uninstall
```

또는 설치 스크립트 사용 (관리자 권한):

```powershell
.\scripts\install.ps1 -Uninstall
```

> 단독 실행(서비스 미등록)의 경우 설치 디렉토리만 삭제하면 됩니다.

## 단독 실행 (서비스 등록 없이)

서비스로 등록하지 않고 단독 실행할 수 있습니다. exe 파일과 설정 파일을 아래 구조로 배치합니다:

```
C:\ResourceAgent\
├── resourceagent.exe
├── LhmHelper.exe          (선택 - 하드웨어 센서 수집 시 필요)
└── configs\
    ├── ResourceAgent.json
    ├── Monitor.json
    └── Logging.json
```

설정 파일 기본 경로가 `configs/ResourceAgent.json` 등 상대 경로이므로, exe가 있는 폴더에서 실행하면 플래그 없이 바로 동작합니다:

```cmd
cd C:\ResourceAgent
resourceagent.exe
```

경로를 직접 지정하려면:

```cmd
resourceagent.exe -config configs\ResourceAgent.json -monitor configs\Monitor.json -logging configs\Logging.json
```

> 상대 경로는 작업 디렉토리(cwd) 기준입니다. 다른 경로에서 실행할 경우 절대 경로를 지정하세요.

## 로컬 테스트 (Kafka 없이)

Kafka/인프라 연결 없이 로컬에서 테스트하려면 `SenderType`을 `file`로 설정합니다.

### 설정

`configs/ResourceAgent.json`에서:

```json
{
  "SenderType": "file",
  "File": {
    "FilePath": "logs/metrics.jsonl",
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
# 기본 경로 사용 (Linux/macOS)
./resourceagent

# 설정 파일 경로 지정
./resourceagent -config configs/ResourceAgent.json -monitor configs/Monitor.json -logging configs/Logging.json
```

### 출력 확인

```bash
# 콘솔 출력 (Console: true 일 때 실행하면 바로 출력)
./resourceagent

# 로그 파일 확인
tail -f logs/metrics.jsonl
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

## 운영

### 로그 확인

```bash
# Linux (systemd journal)
journalctl -u resourceagent -f

# 로그 파일 직접 확인
tail -f /opt/resourceagent/logs/agent.log
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
vi /opt/resourceagent/configs/Monitor.json
# 로그에서 "Monitor configuration updated" 메시지 확인

# 로그 레벨 변경 예시
vi /opt/resourceagent/configs/Logging.json
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

- Windows: LhmHelper.exe가 ResourceAgent.exe와 같은 디렉토리에 있는지 확인
- Windows: PawnIO 드라이버 설치 여부 확인
- macOS: 온도 센서 접근 제한 (개발 테스트용이므로 무시 가능)
- 다른 Collector에는 영향 없음 (Collector 격리)

### 권한 문제 (Linux)

```bash
chmod +x /opt/resourceagent/resourceagent
mkdir -p /opt/resourceagent/logs
chown -R resourceagent:resourceagent /opt/resourceagent
```

## 라이선스

MIT License
