# Windows CPU 온도 수집 테스트 가이드

LibreHardwareMonitor 기반 CPU 온도 수집 기능 테스트 방법입니다.

## 사전 요구사항

- Windows 10+ 또는 Windows Server 2016+
- .NET 8.0 SDK (LhmHelper 빌드용)
- Go 1.21+ (ResourceAgent 빌드용)
- 관리자 권한

## 1. Git 브랜치 받기

```bash
# 저장소 클론 (처음인 경우)
git clone <repository-url>
cd ResourceAgent

# 또는 기존 저장소에서 브랜치 체크아웃
git fetch origin
git checkout feature/lhm-temperature
```

## 2. C# LhmHelper 빌드

Windows에서 실행:

```bash
cd tools/lhm-helper
dotnet publish -c Release -r win-x64 --self-contained
```

**출력 위치**: `tools/lhm-helper/bin/Release/net8.0/win-x64/publish/LhmHelper.exe`

## 3. Go Agent 빌드

```bash
# Windows에서 직접 빌드
go build -o resourceagent.exe ./cmd/resourceagent

# 또는 크로스 컴파일 (Linux/Mac에서)
GOOS=windows GOARCH=amd64 go build -o resourceagent.exe ./cmd/resourceagent
```

## 4. 실행 파일 준비

Windows PC에서 PowerShell 실행:

```powershell
# 테스트 폴더 생성 및 파일 복사
mkdir C:\Test\ResourceAgent
copy resourceagent.exe C:\Test\ResourceAgent\
copy tools\lhm-helper\bin\Release\net8.0\win-x64\publish\LhmHelper.exe C:\Test\ResourceAgent\
```

## 5. PawnIO 드라이버 설치

LhmHelper가 CPU MSR(Model Specific Register)에서 온도를 읽으려면 PawnIO 커널 드라이버가 필요합니다.

### 설치 방법

**옵션 A**: LibreHardwareMonitor 설치 (드라이버 포함)
- https://github.com/LibreHardwareMonitor/LibreHardwareMonitor/releases

**옵션 B**: PawnIO 드라이버만 설치
- https://github.com/openhardwaremonitor/pawnio/releases
```powershell
# 사일런트 설치
PawnIO_setup.exe /S
```

> 재부팅 불필요

## 6. LhmHelper 단독 테스트

관리자 권한 PowerShell에서:

```powershell
cd C:\Test\ResourceAgent
.\LhmHelper.exe
```

**예상 출력**:
```json
{"Sensors":[{"Name":"Intel Core i7-10700 - CPU Package","Temperature":45.5,"High":100,"Critical":105}]}
```

**오류 시 확인사항**:
- 관리자 권한으로 실행했는지 확인
- PawnIO 드라이버 설치 여부 확인
- Windows Defender가 차단하지 않았는지 확인

## 7. ResourceAgent 전체 테스트

### 테스트용 설정 파일 생성

`C:\Test\ResourceAgent\config.json`:

```json
{
  "sender_type": "file",
  "file": {
    "file_path": "metrics.jsonl",
    "console": true,
    "pretty": true
  },
  "collectors": {
    "temperature": {
      "enabled": true,
      "interval": "5s"
    },
    "cpu": { "enabled": false },
    "memory": { "enabled": false },
    "disk": { "enabled": false },
    "network": { "enabled": false },
    "cpu_process": { "enabled": false },
    "memory_process": { "enabled": false }
  }
}
```

### 실행

```powershell
cd C:\Test\ResourceAgent
.\resourceagent.exe -config config.json
```

**예상 출력** (5초마다):
```json
{
  "type": "temperature",
  "timestamp": "2024-01-30T12:00:00Z",
  "data": {
    "sensors": [
      {
        "name": "Intel Core i7-10700 - CPU Package",
        "temperature": 45.5,
        "high": 100,
        "critical": 105
      }
    ]
  }
}
```

## 8. 문제 해결

### LhmHelper.exe 실행 시 빈 센서 배열

```json
{"Sensors":[]}
```

**원인**: PawnIO 드라이버 미설치 또는 관리자 권한 부족

**해결**:
1. PawnIO 드라이버 설치 확인
2. 관리자 권한 PowerShell에서 실행

### "LhmHelper not found" 오류

**원인**: LhmHelper.exe가 올바른 위치에 없음

**해결**: LhmHelper.exe를 resourceagent.exe와 같은 폴더에 복사

### Windows Defender 차단

**해결**: Windows 보안 → 바이러스 및 위협 방지 → 제외 추가

## 명령어 요약

| 단계 | 명령어 |
|------|--------|
| 브랜치 체크아웃 | `git checkout feature/lhm-temperature` |
| LhmHelper 빌드 | `dotnet publish -c Release -r win-x64 --self-contained` |
| Agent 빌드 | `go build -o resourceagent.exe ./cmd/resourceagent` |
| LhmHelper 테스트 | `.\LhmHelper.exe` (관리자 권한) |
| Agent 테스트 | `.\resourceagent.exe -config config.json` |

## 아키텍처

```
ResourceAgent (Go)
       │
       ▼ exec.Command()
LhmHelper.exe (C#/.NET 8)
       │
       ▼ LibreHardwareMonitorLib
PawnIO Driver (Kernel)
       │
       ▼ RDMSR instruction
CPU MSR (IA32_THERM_STATUS)
       │
       ▼ TjMax - deltaT
Temperature Value
```
