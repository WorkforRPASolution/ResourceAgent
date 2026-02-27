# ResourceAgent PRD (Product Requirements Document)

**버전**: 1.0  
**작성일**: 2025-01-29  
**상태**: Draft

---

## 1. 개요

### 1.1 프로젝트 정보

| 항목 | 내용 |
|------|------|
| 프로젝트명 | ResourceAgent |
| 개발 언어 | Go |
| 대상 OS | Windows, Linux |
| 배포 규모 | 10,000대 이상 |

### 1.2 문서 목적

본 문서는 공장 내 PC의 하드웨어 자원 사용률 및 정보를 수집하는 경량 모니터링 Agent의 요구사항과 설계 지침을 정의한다.

---

## 2. 배경 및 목적

### 2.1 배경

- 공장 내 10,000대 이상의 PC에서 하드웨어 자원 현황 파악 필요
- 실시간에 가까운 모니터링을 통한 이상 징후 조기 감지
- 기존 Kafka 인프라 및 Elasticsearch + Grafana 시각화 환경 활용

### 2.2 목적

- PC별 CPU, Memory, Disk, Network, 온도 등 핵심 자원 지표 수집
- 항목별 개별 수집 주기 설정을 통한 유연한 모니터링
- 경량화된 Agent로 대상 PC 성능에 미치는 영향 최소화

---

## 3. 범위

### 3.1 포함 범위 (In Scope)

- 하드웨어 자원 수집 Agent 개발
- JSON 기반 설정 파일 관리
- Kafka로 메트릭 데이터 전송
- Windows 서비스 / Linux 데몬 등록
- Agent 동작 로그 기록

### 3.2 제외 범위 (Out of Scope)

- 관리 서버 및 웹 UI 개발
- 수집 데이터 로컬 저장 및 버퍼링
- Kafka Consumer 및 Elasticsearch 연동
- 알림 및 이상 감지 로직

---

## 4. 기능 요구사항

### 4.1 자원 수집 기능

#### 4.1.1 CPU 전체

| 항목 | 요구사항 |
|------|----------|
| 수집 데이터 | 전체 CPU 사용률 (%) |
| 수집 주기 | 설정 가능 (기본 10초) |
| 플랫폼 | Windows, Linux |

#### 4.1.2 CPU 프로세스별

| 항목 | 요구사항 |
|------|----------|
| 수집 데이터 | 프로세스별 CPU 사용률 (%) |
| 수집 대상 | 상위 N개 또는 특정 프로세스명 지정 (설정 가능) |
| 수집 주기 | 설정 가능 |
| 플랫폼 | Windows, Linux |

#### 4.1.3 Memory 전체

| 항목 | 요구사항 |
|------|----------|
| 수집 데이터 | 전체 메모리 사용량 (bytes), 사용률 (%) |
| 수집 주기 | 설정 가능 |
| 플랫폼 | Windows, Linux |

#### 4.1.4 Memory 프로세스별

| 항목 | 요구사항 |
|------|----------|
| 수집 데이터 | 프로세스별 메모리 사용량 (bytes) |
| 수집 대상 | 상위 N개 또는 특정 프로세스명 지정 (설정 가능) |
| 수집 주기 | 설정 가능 |
| 플랫폼 | Windows, Linux |

#### 4.1.5 Disk

| 항목 | 요구사항 |
|------|----------|
| 수집 데이터 | 드라이브별 사용량, 전체 용량, 사용률 (%) |
| 수집 대상 | 전체 드라이브 |
| 수집 주기 | 설정 가능 |
| 플랫폼 | Windows, Linux |

#### 4.1.6 Network

| 항목 | 요구사항 |
|------|----------|
| 수집 데이터 | 송신/수신 바이트, 송신/수신 패킷 |
| 수집 대상 | 전체 합계 및 개별 인터페이스 |
| 수집 주기 | 설정 가능 |
| 플랫폼 | Windows, Linux |

#### 4.1.7 CPU 온도

| 항목 | 요구사항 |
|------|----------|
| 수집 데이터 | CPU 온도 (섭씨) |
| 수집 주기 | 설정 가능 |
| 플랫폼 | Windows (WMI/OpenHardwareMonitor), Linux (/sys/class/thermal, lm-sensors) |
| 비고 | 하드웨어/드라이버 지원에 따라 수집 불가할 수 있음 |

#### 4.1.8 Uptime

| 항목 | 요구사항 |
|------|----------|
| 수집 데이터 | 마지막 부팅 시각 (Unix timestamp), 부팅 이후 경과 시간 (분) |
| 수집 주기 | 설정 가능 (기본 300초) |
| 플랫폼 | Windows, Linux |
| 비고 | gopsutil host.BootTime() 사용, 재부팅 감지 및 가동 시간 추적 |

### 4.2 설정 관리 기능

| 항목 | 요구사항 |
|------|----------|
| 설정 포맷 | JSON 파일 |
| 설정 위치 | 실행 파일과 동일 디렉토리 또는 지정 경로 |
| 설정 반영 | 파일 변경 감지 시 자동 반영 (Hot Reload) |
| 설정 항목 | 수집 항목별 활성화 여부, 수집 주기, 프로세스 필터 등 |

### 4.3 데이터 전송 기능

| 항목 | 요구사항 |
|------|----------|
| 전송 대상 | Kafka Cluster |
| 전송 포맷 | JSON |
| 압축 | Snappy (설정 가능) |
| 재시도 | 전송 실패 시 재시도 (횟수 설정 가능) |
| 버퍼링 | 없음 (네트워크 단절 시 데이터 유실 허용) |

### 4.4 서비스 관리 기능

| 항목 | 요구사항 |
|------|----------|
| Windows | Windows 서비스로 등록/실행 |
| Linux | systemd 데몬으로 등록/실행 |
| 자동 시작 | OS 부팅 시 자동 시작 |

### 4.5 로깅 기능

| 항목 | 요구사항 |
|------|----------|
| 로그 대상 | Agent 동작 로그 (시작, 종료, 오류, 설정 변경 등) |
| 로그 제외 | 수집 데이터 자체 |
| 로그 포맷 | 텍스트 (타임스탬프, 레벨, 메시지) |
| 로그 로테이션 | 파일 크기 또는 일자 기준 |

---

## 5. 비기능 요구사항

### 5.1 성능

| 항목 | 요구사항 |
|------|----------|
| CPU 사용률 | Agent 자체 CPU 사용률 1% 미만 (유휴 시) |
| 메모리 사용량 | 50MB 이하 |
| 바이너리 크기 | 20MB 이하 |

### 5.2 안정성

| 항목 | 요구사항 |
|------|----------|
| 장기 실행 | 메모리 누수 없이 장기간 안정 실행 |
| 오류 복구 | 개별 Collector 오류 시 다른 Collector에 영향 없음 |
| 설정 오류 | 잘못된 설정 시 기본값으로 동작 및 로그 기록 |

### 5.3 호환성

| 항목 | 요구사항 |
|------|----------|
| Windows | Windows 10 이상, Windows Server 2016 이상 |
| Linux | Ubuntu 18.04 이상, CentOS 7 이상, 또는 동등 배포판 |

### 5.4 유지보수성

| 항목 | 요구사항 |
|------|----------|
| 설계 원칙 | SOLID 원칙 준수 |
| 코드 구조 | 모듈화된 구조로 Collector 추가/수정 용이 |
| 테스트 | 단위 테스트 작성 가능한 구조 |

---

## 6. 아키텍처 설계

### 6.1 설계 원칙

본 Agent는 SOLID 원칙을 준수하여 설계한다.

#### 6.1.1 Single Responsibility Principle (단일 책임 원칙)

각 컴포넌트는 하나의 책임만 가진다.

| 컴포넌트 | 책임 |
|----------|------|
| Collector | 특정 자원 수집만 담당 |
| Scheduler | 수집 주기 관리만 담당 |
| Sender | Kafka 전송만 담당 |
| ConfigManager | 설정 로드/감시만 담당 |
| Logger | 로그 기록만 담당 |

#### 6.1.2 Open/Closed Principle (개방/폐쇄 원칙)

새로운 Collector 추가 시 기존 코드 수정 없이 확장 가능해야 한다.

```
새 Collector 추가 시:
1. Collector 인터페이스 구현
2. Registry에 등록
→ Scheduler, Sender 코드 변경 불필요
```

#### 6.1.3 Liskov Substitution Principle (리스코프 치환 원칙)

모든 Collector는 동일한 인터페이스를 구현하며, 상호 대체 가능해야 한다.

#### 6.1.4 Interface Segregation Principle (인터페이스 분리 원칙)

클라이언트가 사용하지 않는 메서드에 의존하지 않도록 인터페이스를 분리한다.

| 인터페이스 | 메서드 |
|------------|--------|
| Collector | Collect() |
| Configurable | LoadConfig(), OnConfigChange() |
| Sender | Send() |

#### 6.1.5 Dependency Inversion Principle (의존성 역전 원칙)

고수준 모듈이 저수준 모듈에 의존하지 않고, 추상화에 의존한다.

```
Scheduler → Collector Interface ← CPUCollector, MemoryCollector, ...
Collector → Sender Interface ← KafkaSender
```

### 6.2 컴포넌트 구조

```
┌─────────────────────────────────────────────────────────────────┐
│                        ResourceAgent                            │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐    ┌─────────────────────────────────────┐    │
│  │   Config    │    │            Collectors               │    │
│  │   Manager   │    ├─────────────────────────────────────┤    │
│  │             │    │ ┌─────────┐ ┌─────────┐ ┌─────────┐ │    │
│  │ - Load      │    │ │   CPU   │ │ Memory  │ │  Disk   │ │    │
│  │ - Watch     │    │ └─────────┘ └─────────┘ └─────────┘ │    │
│  │ - Notify    │    │ ┌─────────┐ ┌─────────┐ ┌─────────┐ │    │
│  └──────┬──────┘    │ │ Network │ │  Temp   │ │ Process │ │    │
│         │           │ └─────────┘ └─────────┘ └─────────┘ │    │
│         │           └──────────────────┬──────────────────┘    │
│         │                              │                       │
│         ▼                              ▼                       │
│  ┌─────────────┐              ┌─────────────┐                  │
│  │  Scheduler  │──────────────│   Sender    │                  │
│  │             │              │   (Kafka)   │                  │
│  │ - Register  │              │             │                  │
│  │ - Start     │              │ - Connect   │                  │
│  │ - Stop      │              │ - Send      │                  │
│  └─────────────┘              │ - Retry     │                  │
│                               └─────────────┘                  │
│                                      │                         │
│  ┌─────────────┐                     │                         │
│  │   Logger    │◄────────────────────┘                         │
│  └─────────────┘                                               │
└─────────────────────────────────────────────────────────────────┘
```

### 6.3 인터페이스 정의

#### 6.3.1 Collector Interface

```go
type Collector interface {
    // Name returns the collector identifier
    Name() string
    
    // Collect gathers metrics and returns the result
    Collect(ctx context.Context) (*MetricData, error)
    
    // Configure updates collector settings
    Configure(config CollectorConfig) error
    
    // Interval returns the collection interval
    Interval() time.Duration
}
```

#### 6.3.2 Sender Interface

```go
type Sender interface {
    // Send transmits metric data
    Send(ctx context.Context, data *MetricData) error
    
    // Close releases resources
    Close() error
}
```

#### 6.3.3 ConfigManager Interface

```go
type ConfigManager interface {
    // Load reads configuration from file
    Load(path string) (*Config, error)
    
    // Watch monitors configuration changes
    Watch(path string, callback func(*Config)) error
    
    // Stop stops watching
    Stop()
}
```

### 6.4 디렉토리 구조

```
resourceagent/
├── cmd/
│   └── resourceagent/
│       └── main.go              # 진입점
├── internal/
│   ├── collector/
│   │   ├── collector.go         # Collector 인터페이스
│   │   ├── registry.go          # Collector 등록/관리
│   │   ├── cpu.go               # CPU 전체 수집
│   │   ├── cpu_process.go       # CPU 프로세스별 수집
│   │   ├── memory.go            # Memory 전체 수집
│   │   ├── memory_process.go    # Memory 프로세스별 수집
│   │   ├── disk.go              # Disk 수집
│   │   ├── network.go           # Network 수집
│   │   └── temperature.go       # CPU 온도 수집
│   ├── config/
│   │   ├── config.go            # 설정 구조체
│   │   ├── loader.go            # 설정 로드
│   │   └── watcher.go           # 설정 파일 감시
│   ├── sender/
│   │   ├── sender.go            # Sender 인터페이스
│   │   └── kafka.go             # Kafka 구현체
│   ├── scheduler/
│   │   └── scheduler.go         # 수집 스케줄링
│   ├── logger/
│   │   └── logger.go            # 로깅
│   └── service/
│       ├── service.go           # 서비스 인터페이스
│       ├── windows.go           # Windows 서비스
│       └── linux.go             # Linux 데몬
├── configs/
│   └── config.json              # 설정 파일 샘플
├── scripts/
│   ├── install.ps1              # Windows 설치 스크립트
│   └── install.sh               # Linux 설치 스크립트
├── go.mod
├── go.sum
└── README.md
```

---

## 7. 데이터 명세

### 7.1 설정 파일 스키마 (config.json)

```json
{
  "agent": {
    "id": "auto",
    "hostname": "",
    "tags": {
      "factory": "FAC01",
      "line": "LINE03"
    }
  },
  "kafka": {
    "brokers": ["kafka1:9092", "kafka2:9092"],
    "topic": "factory-metrics",
    "compression": "snappy",
    "retry_count": 3,
    "retry_interval_ms": 1000
  },
  "collectors": {
    "cpu": {
      "enabled": true,
      "interval_seconds": 10
    },
    "cpu_process": {
      "enabled": true,
      "interval_seconds": 30,
      "top_n": 10,
      "include_processes": ["MESClient.exe", "ERPAgent.exe"],
      "exclude_processes": []
    },
    "memory": {
      "enabled": true,
      "interval_seconds": 10
    },
    "memory_process": {
      "enabled": true,
      "interval_seconds": 30,
      "top_n": 10,
      "include_processes": [],
      "exclude_processes": []
    },
    "disk": {
      "enabled": true,
      "interval_seconds": 60
    },
    "network": {
      "enabled": true,
      "interval_seconds": 10
    },
    "temperature": {
      "enabled": true,
      "interval_seconds": 30
    }
  },
  "logging": {
    "level": "info",
    "file": "logs/agent.log",
    "max_size_mb": 10,
    "max_backups": 5
  }
}
```

### 7.2 Kafka 메시지 포맷

#### 7.2.1 공통 헤더

```json
{
  "agent_id": "FAC01-LINE03-PC042",
  "hostname": "PC042",
  "tags": {
    "factory": "FAC01",
    "line": "LINE03"
  },
  "timestamp": "2025-01-29T10:30:00.000Z",
  "metric_type": "cpu",
  "agent_version": "1.0.0"
}
```

#### 7.2.2 CPU 전체

```json
{
  "agent_id": "FAC01-LINE03-PC042",
  "timestamp": "2025-01-29T10:30:00.000Z",
  "metric_type": "cpu",
  "data": {
    "usage_percent": 45.2,
    "core_count": 8,
    "per_core_percent": [42.1, 48.3, 44.0, 46.5, 45.8, 44.2, 47.1, 43.6]
  }
}
```

#### 7.2.3 CPU 프로세스별

```json
{
  "agent_id": "FAC01-LINE03-PC042",
  "timestamp": "2025-01-29T10:30:00.000Z",
  "metric_type": "cpu_process",
  "data": {
    "processes": [
      {"name": "MESClient.exe", "pid": 1234, "cpu_percent": 12.5},
      {"name": "chrome.exe", "pid": 5678, "cpu_percent": 8.2},
      {"name": "ERPAgent.exe", "pid": 9012, "cpu_percent": 5.1}
    ]
  }
}
```

#### 7.2.4 Memory 전체

```json
{
  "agent_id": "FAC01-LINE03-PC042",
  "timestamp": "2025-01-29T10:30:00.000Z",
  "metric_type": "memory",
  "data": {
    "total_bytes": 17179869184,
    "used_bytes": 13312004096,
    "available_bytes": 3867865088,
    "usage_percent": 77.5
  }
}
```

#### 7.2.5 Memory 프로세스별

```json
{
  "agent_id": "FAC01-LINE03-PC042",
  "timestamp": "2025-01-29T10:30:00.000Z",
  "metric_type": "memory_process",
  "data": {
    "processes": [
      {"name": "chrome.exe", "pid": 5678, "memory_bytes": 1073741824},
      {"name": "MESClient.exe", "pid": 1234, "memory_bytes": 536870912},
      {"name": "ERPAgent.exe", "pid": 9012, "memory_bytes": 268435456}
    ]
  }
}
```

#### 7.2.6 Disk

```json
{
  "agent_id": "FAC01-LINE03-PC042",
  "timestamp": "2025-01-29T10:30:00.000Z",
  "metric_type": "disk",
  "data": {
    "drives": [
      {
        "mount_point": "C:",
        "total_bytes": 536870912000,
        "used_bytes": 193273528320,
        "free_bytes": 343597383680,
        "usage_percent": 36.0
      },
      {
        "mount_point": "D:",
        "total_bytes": 1099511627776,
        "used_bytes": 549755813888,
        "free_bytes": 549755813888,
        "usage_percent": 50.0
      }
    ]
  }
}
```

#### 7.2.7 Network

```json
{
  "agent_id": "FAC01-LINE03-PC042",
  "timestamp": "2025-01-29T10:30:00.000Z",
  "metric_type": "network",
  "data": {
    "total": {
      "bytes_sent": 123456789,
      "bytes_recv": 987654321,
      "packets_sent": 12345,
      "packets_recv": 54321
    },
    "interfaces": [
      {
        "name": "Ethernet",
        "bytes_sent": 123456789,
        "bytes_recv": 987654321,
        "packets_sent": 12345,
        "packets_recv": 54321
      }
    ]
  }
}
```

#### 7.2.8 CPU 온도

```json
{
  "agent_id": "FAC01-LINE03-PC042",
  "timestamp": "2025-01-29T10:30:00.000Z",
  "metric_type": "temperature",
  "data": {
    "cpu_celsius": 62.0,
    "sensors": [
      {"name": "Core 0", "celsius": 60.0},
      {"name": "Core 1", "celsius": 64.0}
    ]
  }
}
```

---

## 8. 구현 가이드

### 8.1 핵심 라이브러리

| 용도 | 라이브러리 | 비고 |
|------|-----------|------|
| 시스템 정보 | github.com/shirou/gopsutil/v3 | Windows/Linux 통합 지원 |
| Kafka 클라이언트 | github.com/IBM/sarama | 고성능 Kafka 클라이언트 |
| 설정 파일 감시 | github.com/fsnotify/fsnotify | 파일 변경 감지 |
| 로깅 | github.com/rs/zerolog | 고성능 JSON 로깅 |
| Windows 서비스 | golang.org/x/sys/windows/svc | Windows 서비스 관리 |

### 8.2 CPU 온도 수집 방법

#### Windows

```
1차: WMI (MSAcpi_ThermalZoneTemperature)
2차: Open Hardware Monitor 라이브러리 연동
제한: 일부 시스템에서 지원하지 않을 수 있음
```

#### Linux

```
1차: /sys/class/thermal/thermal_zone*/temp
2차: lm-sensors (sensors 명령어)
제한: 가상화 환경에서 미지원 가능
```

### 8.3 크로스 컴파일

```bash
# Windows 64bit
GOOS=windows GOARCH=amd64 go build -o ResourceAgent.exe ./cmd/resourceagent

# Linux 64bit
GOOS=linux GOARCH=amd64 go build -o resourceagent ./cmd/resourceagent
```

### 8.4 서비스 등록

#### Windows

```powershell
# 서비스 등록
sc create ResourceAgent binPath= "C:\Program Files\ResourceAgent\ResourceAgent.exe" start= auto

# 서비스 시작
sc start ResourceAgent
```

#### Linux (systemd)

```ini
# /etc/systemd/system/resourceagent.service
[Unit]
Description=ResourceAgent Monitoring Service
After=network.target

[Service]
Type=simple
ExecStart=/opt/resourceagent/resourceagent
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

---

## 9. 향후 확장 계획

### 9.1 Phase 2 - 웹 UI 연동

| 항목 | 내용 |
|------|------|
| 설정 동기화 | 주기적으로 관리 서버에서 설정 polling |
| 상태 보고 | Agent 상태 (버전, 실행 상태 등) 보고 |
| 원격 제어 | 재시작, 설정 변경 명령 수신 |

### 9.2 Phase 3 - 기능 확장

| 항목 | 내용 |
|------|------|
| GPU 모니터링 | NVIDIA GPU 사용률, 메모리, 온도 |
| 애플리케이션 로그 | 특정 애플리케이션 로그 수집 |
| 이벤트 로그 | Windows 이벤트 로그 수집 |

### 9.3 Phase 4 - 자동화

| 항목 | 내용 |
|------|------|
| 자동 업데이트 | Agent 바이너리 자동 업데이트 |
| 배포 자동화 | AD GPO 또는 SCCM 연동 |

---

## 10. 부록

### 10.1 용어 정의

| 용어 | 정의 |
|------|------|
| Collector | 특정 자원 유형을 수집하는 모듈 |
| Metric | 수집된 측정값 |
| Hot Reload | 재시작 없이 설정 변경 반영 |

### 10.2 참고 자료

- gopsutil: https://github.com/shirou/gopsutil
- sarama (Kafka): https://github.com/IBM/sarama
- Go Windows Service: https://pkg.go.dev/golang.org/x/sys/windows/svc

---

**문서 끝**
