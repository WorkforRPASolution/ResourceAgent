# ResourceAgent

공장 내 PC(10,000대 이상)의 하드웨어 자원 사용률을 수집하는 Go 기반 경량 모니터링 에이전트입니다.

## 주요 기능

- **메트릭 수집**: CPU, Memory, Disk, Network, Temperature
- **프로세스 모니터링**: CPU/Memory 상위 프로세스 추적
- **유연한 전송 방식**: Kafka 또는 로컬 파일 출력
- **Hot Reload**: 설정 파일 변경 시 자동 반영
- **크로스 플랫폼**: Windows, Linux, macOS 지원

## 시스템 요구사항

### 지원 플랫폼
- Windows 10+, Windows Server 2016+
- Ubuntu 18.04+, CentOS 7+
- macOS (개발/테스트용)

### 리소스 사용량
- CPU: 유휴 시 1% 미만
- Memory: 50MB 이하
- 바이너리 크기: 20MB 이하

## 빌드

### 사전 요구사항
- Go 1.21 이상

### 빌드 명령어

```bash
# 의존성 다운로드
go mod tidy

# Linux 빌드
GOOS=linux GOARCH=amd64 go build -o resourceagent ./cmd/resourceagent

# Windows 빌드
GOOS=windows GOARCH=amd64 go build -o resourceagent.exe ./cmd/resourceagent

# macOS 빌드 (테스트용)
go build -o resourceagent ./cmd/resourceagent
```

## 설정

### 설정 파일 구조

`configs/config.json`:

```json
{
  "agent": {
    "id": "",
    "hostname": "",
    "tags": {
      "environment": "production",
      "location": "factory-1"
    }
  },
  "sender_type": "kafka",
  "file": {
    "file_path": "logs/metrics.jsonl",
    "max_size_mb": 50,
    "max_backups": 3,
    "console": true,
    "pretty": false
  },
  "kafka": {
    "brokers": ["localhost:9092"],
    "topic": "factory-metrics",
    "compression": "snappy"
  },
  "collectors": {
    "cpu": { "enabled": true, "interval": "10s" },
    "memory": { "enabled": true, "interval": "10s" },
    "disk": { "enabled": true, "interval": "30s" },
    "network": { "enabled": true, "interval": "10s" },
    "temperature": { "enabled": true, "interval": "30s" },
    "cpu_process": { "enabled": true, "interval": "30s", "top_n": 10 },
    "memory_process": { "enabled": true, "interval": "30s", "top_n": 10 }
  },
  "logging": {
    "level": "info",
    "file_path": "logs/agent.log",
    "console": false
  }
}
```

### 주요 설정 항목

| 항목 | 설명 | 기본값 |
|------|------|--------|
| `sender_type` | 전송 방식 (`kafka` 또는 `file`) | `kafka` |
| `file.console` | 콘솔에도 메트릭 출력 | `true` |
| `file.pretty` | JSON 들여쓰기 | `false` |
| `collectors.*.interval` | 수집 주기 | 10s~30s |
| `collectors.*.top_n` | 프로세스 모니터링 개수 | 10 |

## 설치

### Linux (systemd)

```bash
# 1. 바이너리 복사
sudo mkdir -p /opt/resourceagent
sudo cp resourceagent /opt/resourceagent/
sudo cp configs/config.json /opt/resourceagent/

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
New-Item -ItemType Directory -Path "C:\Program Files\ResourceAgent" -Force
Copy-Item resourceagent.exe "C:\Program Files\ResourceAgent\"
Copy-Item configs\config.json "C:\Program Files\ResourceAgent\"

# 2. 서비스 등록
sc.exe create ResourceAgent binPath= "C:\Program Files\ResourceAgent\resourceagent.exe" start= auto
sc.exe start ResourceAgent

# 3. 상태 확인
sc.exe query ResourceAgent
```

또는 설치 스크립트 사용 (관리자 권한):

```powershell
.\scripts\install.ps1
```

## 로컬 테스트 (Kafka 없이)

Kafka 연결 없이 로컬에서 테스트하려면 `sender_type`을 `file`로 설정합니다.

### 방법 1: 로컬 테스트용 설정 파일 사용

```bash
./resourceagent -config configs/config.local.json
```

### 방법 2: 기존 설정 파일 수정

`config.json`에서 `sender_type`을 `file`로 변경:

```json
{
  "sender_type": "file",
  "file": {
    "file_path": "logs/metrics.jsonl",
    "console": true,
    "pretty": false
  }
}
```

### 출력 확인

```bash
# 콘솔 출력 (console: true일 때)
./resourceagent -config configs/config.local.json

# 로그 파일 확인
tail -f logs/metrics.jsonl
```

### 출력 예시 (JSONL 형식)

```json
{"type":"cpu","timestamp":"2024-01-29T12:00:00Z","agent_id":"my-pc","hostname":"my-pc","data":{"usage_percent":15.5,"core_count":8}}
{"type":"memory","timestamp":"2024-01-29T12:00:00Z","agent_id":"my-pc","hostname":"my-pc","data":{"total_bytes":17179869184,"usage_percent":45.2}}
```

## 운영

### 로그 확인

```bash
# Linux
journalctl -u resourceagent -f

# 또는 로그 파일 직접 확인
tail -f /opt/resourceagent/logs/agent.log
```

### 서비스 관리

```bash
# Linux
sudo systemctl start resourceagent    # 시작
sudo systemctl stop resourceagent     # 중지
sudo systemctl restart resourceagent  # 재시작
sudo systemctl status resourceagent   # 상태 확인

# Windows
sc.exe start ResourceAgent
sc.exe stop ResourceAgent
sc.exe query ResourceAgent
```

### 설정 변경 (Hot Reload)

설정 파일을 수정하면 자동으로 반영됩니다. 서비스 재시작이 필요 없습니다.

```bash
# 설정 파일 수정
vi /opt/resourceagent/config.json

# 로그에서 반영 확인
# "Applying configuration changes" 메시지 출력
```

## 문제 해결

### Kafka 연결 실패

```
failed to create Kafka producer: kafka: client has run out of available brokers
```

- Kafka 브로커 주소 확인
- 네트워크 연결 확인
- 로컬 테스트 시 `sender_type: "file"` 사용

### 온도 수집 실패

macOS/Windows에서는 온도 센서 접근이 제한될 수 있습니다. 다른 Collector에는 영향을 주지 않습니다.

### 권한 문제 (Linux)

```bash
# 실행 권한 부여
chmod +x /opt/resourceagent/resourceagent

# 로그 디렉토리 권한
mkdir -p /opt/resourceagent/logs
chown -R root:root /opt/resourceagent
```

## 라이선스

MIT License
