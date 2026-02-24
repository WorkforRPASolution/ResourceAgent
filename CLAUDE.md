## 상위 계획 참조

세션 중 "상위 플랜 체크해" 요청 시: `/Users/hyunkyungmin/Developer/ARS/.claude/PLANNING.md` 읽기

# CLAUDE.md

이 파일은 Claude Code (claude.ai/code)가 이 저장소에서 작업할 때 참고하는 가이드입니다.

## 프로젝트 개요

ResourceAgent는 공장 내 PC(10,000대 이상)의 하드웨어 자원 사용률을 수집하는 Go 기반 경량 모니터링 에이전트입니다. CPU, Memory, Disk, Network, 온도 메트릭을 수집하여 Kafka 또는 KafkaRest Proxy로 전송합니다.

**지원 플랫폼**: Windows 10+, Windows Server 2016+, Ubuntu 18.04+, CentOS 7+

## 빌드 명령어

```bash
# Windows 빌드
GOOS=windows GOARCH=amd64 go build -o resourceagent.exe ./cmd/resourceagent

# Linux 빌드
GOOS=linux GOARCH=amd64 go build -o resourceagent ./cmd/resourceagent

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

- **Collector**: 특정 자원 메트릭 수집 (CPU, Memory, Disk, Network, Temperature)
- **Sender**: Kafka로 메트릭 전송
- **ConfigManager**: 설정 로드, 변경 감지, Hot Reload 처리

### 디렉토리 구조

```
resourceagent/
├── cmd/resourceagent/main.go       # 진입점
├── internal/
│   ├── collector/                  # Collector 구현체
│   │   ├── collector.go           # 인터페이스 정의
│   │   ├── registry.go            # Collector 등록/관리
│   │   ├── cpu.go, memory.go, disk.go, network.go
│   │   ├── temperature.go         # 공통 인터페이스
│   │   ├── temperature_windows.go # Windows LHM 연동
│   │   ├── temperature_linux.go   # Linux gopsutil
│   │   └── cpu_process.go, memory_process.go
│   ├── config/                     # 설정 관리
│   ├── discovery/                  # ServiceDiscovery HTTP 클라이언트
│   ├── eqpinfo/                    # Redis EQP_INFO 조회
│   ├── network/                    # IP 감지 + SOCKS5 dialer
│   ├── sender/                     # Kafka/KafkaRest/File Sender 구현
│   ├── scheduler/                  # 수집 스케줄링
│   ├── logger/                     # 구조화된 로깅
│   └── service/                    # Windows/Linux 서비스 통합
├── tools/
│   └── lhm-helper/                 # C# LibreHardwareMonitor 헬퍼
├── configs/config.json             # 설정 파일 샘플
└── scripts/                        # 설치 스크립트
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

#### Windows 온도 수집 구조

```
ResourceAgent (Go) → LhmHelper.exe (C#) → LibreHardwareMonitorLib → PawnIO Driver → MSR
```

**LhmHelper 빌드**:
```bash
cd tools/lhm-helper
dotnet publish -c Release -r win-x64 --self-contained
# 출력: bin/Release/net8.0/win-x64/publish/LhmHelper.exe
```

**PawnIO 드라이버**:
- LibreHardwareMonitor의 하드웨어 접근 드라이버 (WinRing0 대체)
- 설치: `PawnIO_setup.exe /S` (사일런트 설치)
- 재부팅 불필요
- Microsoft 서명 버전 제공

**배포 시 포함 파일**:
- `resourceagent.exe` (Go 바이너리)
- `LhmHelper.exe` (C# 헬퍼, ~60-80MB self-contained)
- PawnIO 드라이버 설치 스크립트

### 데이터 흐름

1. ConfigManager가 JSON 설정 로드 및 변경 감시
2. sender_type != "file" 이면: IP 감지 → Redis EQP_INFO 조회 → ServiceDiscovery → KafkaRest/Topic 결정
3. Scheduler가 설정된 주기로 Collector 등록
4. Collector가 스케줄에 따라 메트릭 수집
5. Sender가 메트릭 전송:
   - **kafkarest**: MetricData → EARSRow[] → 평문 raw (Grok 호환) → HTTP POST (`KafkaMessageWrapper2`)
   - **kafka**: MetricData → EARSRow[] → JSON raw (ParsedDataList) → sarama produce (`KafkaValue`)
   - **file**: MetricData JSON 그대로 파일에 기록
6. 로컬 버퍼링 없음 - 네트워크 단절 시 데이터 유실 허용

## 서비스 설치

**Windows**:
```cmd
sc create ResourceAgent binPath= "C:\Program Files\ResourceAgent\resourceagent.exe" start= auto
```

**Linux (systemd)**:
```bash
# resourceagent.service를 /etc/systemd/system/에 복사
systemctl enable resourceagent
systemctl start resourceagent
```
