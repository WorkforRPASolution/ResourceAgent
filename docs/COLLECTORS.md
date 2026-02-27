# ResourceAgent Collectors 가이드

ResourceAgent의 모든 수집기(Collector)에 대한 상세 설명, 설정 방법, 출력 예시입니다.

## 목차

- [개요](#개요)
- [공통 설정](#공통-설정)
- [권장 수집 주기](#권장-수집-주기)
- [기본 Collectors](#기본-collectors)
  - [CPU Collector](#cpu-collector)
  - [Memory Collector](#memory-collector)
  - [Disk Collector](#disk-collector)
  - [Network Collector](#network-collector)
- [프로세스 Collectors](#프로세스-collectors)
  - [CPU Process Collector](#cpu-process-collector)
  - [Memory Process Collector](#memory-process-collector)
- [하드웨어 모니터링 Collectors](#하드웨어-모니터링-collectors)
  - [Temperature Collector](#temperature-collector)
  - [Fan Collector](#fan-collector)
  - [GPU Collector](#gpu-collector)
  - [Storage S.M.A.R.T Collector](#storage-smart-collector)
  - [Voltage Collector](#voltage-collector)
  - [Motherboard Temperature Collector](#motherboard-temperature-collector)
- [시스템 Collectors](#시스템-collectors)
  - [Uptime Collector](#uptime-collector)
  - [ProcessWatch Collector](#processwatch-collector)
- [플랫폼별 지원 현황](#플랫폼별-지원-현황)
- [전체 설정 예시](#전체-설정-예시)

---

## 개요

ResourceAgent는 14개의 수집기를 제공합니다:

| Collector | 설명 | 플랫폼 |
|-----------|------|--------|
| cpu | CPU 사용률 | Windows, Linux |
| memory | 메모리 사용량 | Windows, Linux |
| disk | 디스크 사용량 및 I/O | Windows, Linux |
| network | 네트워크 트래픽 | Windows, Linux |
| cpu_process | 프로세스별 CPU 사용률 | Windows, Linux |
| memory_process | 프로세스별 메모리 사용량 | Windows, Linux |
| temperature | CPU 온도 | Windows (LHM), Linux |
| fan | 팬 속도 | Windows (LHM) |
| gpu | GPU 메트릭 | Windows (LHM) |
| storage_smart | S.M.A.R.T 디스크 상태 | Windows (LHM) |
| voltage | 전압 센서 | Windows (LHM) |
| motherboard_temp | 메인보드 온도 | Windows (LHM) |
| uptime | 시스템 부팅 시각 및 가동 시간 | Windows, Linux |
| process_watch | 필수/금지 프로세스 감시 | Windows, Linux |

> **LHM**: LibreHardwareMonitor 기반 (Windows 전용, 관리자 권한 필요)

---

## 공통 설정

모든 수집기는 다음 공통 설정을 지원합니다:

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 수집기 활성화 여부 | `true` |
| `interval` | string | 수집 주기 (예: "10s", "1m") | 수집기별 상이 |

```json
{
  "collectors": {
    "collector_name": {
      "enabled": true,
      "interval": "10s"
    }
  }
}
```

---

## 권장 수집 주기

대규모 PC 모니터링(10,000대+) 환경에서 권장하는 수집 주기입니다.

### 주기 결정 기준

수집 주기는 다음 요소를 고려하여 결정합니다:

1. **변동성**: 메트릭이 얼마나 빠르게 변하는가?
2. **긴급성**: 이상 탐지가 얼마나 빨리 필요한가?
3. **수집 비용**: 수집에 필요한 시스템 리소스는?
4. **데이터 볼륨**: Kafka/스토리지 부하는 적절한가?

### 권장 주기 테이블

| Collector | 권장 주기 | 근거 |
|-----------|----------|------|
| **cpu** | 10s | CPU 사용률은 빠르게 변동, 스파이크 조기 감지 필요 |
| **memory** | 10s | 메모리 누수 조기 감지, OOM 예방에 중요 |
| **network** | 10s | 트래픽 패턴이 빠르게 변화, 이상 트래픽 탐지 |
| **disk** | 30s | I/O 패턴은 상대적으로 안정적, 추세 분석에 충분 |
| **cpu_process** | 30s | 프로세스 CPU 패턴 분석, 30s면 충분 |
| **memory_process** | 30s | 프로세스 메모리 증가 추세 파악 |
| **temperature** | 30s | 열 이벤트는 수초 내 발생하지 않음 |
| **fan** | 30s | 온도에 따라 변화, temperature와 동기화 |
| **gpu** | 30s | GPU 워크로드 모니터링에 적절 |
| **voltage** | 60s | PSU 전압은 매우 안정적, 급격한 변화 드묾 |
| **motherboard_temp** | 60s | 주변 온도/방열로 천천히 변화 |
| **storage_smart** | 300s (5분) | S.M.A.R.T 값은 시간/일 단위로 변화, I/O 부하 감소 |
| **process_watch** | 60s | 프로세스 상태 변화 감시, 1분이면 충분 |

### 주기별 그룹

```
┌─────────────────────────────────────────────────────────────────┐
│  10s (실시간)      │  cpu, memory, network                      │
├─────────────────────────────────────────────────────────────────┤
│  30s (중간)        │  disk, temperature, fan, gpu,              │
│                    │  cpu_process, memory_process               │
├─────────────────────────────────────────────────────────────────┤
│  60s (저빈도)      │  voltage, motherboard_temp, process_watch   │
├─────────────────────────────────────────────────────────────────┤
│  300s (최저빈도)   │  storage_smart                             │
└─────────────────────────────────────────────────────────────────┘
```

### 데이터 볼륨 예측

**10,000대 PC 기준 시간당 메시지 수:**

| 주기 | 수집기 수 | PC당 수집/시간 | 총 메시지/시간 |
|------|----------|---------------|---------------|
| 10s | 3개 | 1,080 | 10,800,000 |
| 30s | 6개 | 720 | 7,200,000 |
| 60s | 3개 | 180 | 1,800,000 |
| 300s | 1개 | 12 | 120,000 |
| **합계** | **13개** | **1,992** | **~19,920,000** |

> **참고**: 실제 메시지 크기는 평균 500B~2KB, 시간당 약 10~40GB 데이터 발생

### 시나리오별 조정 가이드

#### 리소스 제약 환경 (저사양 PC)

CPU/메모리 부하를 줄이려면 주기를 늘립니다:

```json
{
  "collectors": {
    "cpu": { "interval": "30s" },
    "memory": { "interval": "30s" },
    "network": { "interval": "30s" },
    "temperature": { "interval": "60s" },
    "storage_smart": { "interval": "600s" }
  }
}
```

#### 고빈도 모니터링 (중요 서버)

더 빠른 이상 탐지가 필요한 경우:

```json
{
  "collectors": {
    "cpu": { "interval": "5s" },
    "memory": { "interval": "5s" },
    "temperature": { "interval": "10s" },
    "gpu": { "interval": "10s" }
  }
}
```

#### Kafka 부하 감소

데이터 볼륨을 줄이려면:

```json
{
  "collectors": {
    "cpu": { "interval": "30s" },
    "memory": { "interval": "30s" },
    "disk": { "interval": "60s" },
    "network": { "interval": "30s" },
    "storage_smart": { "interval": "600s" }
  }
}
```

### 프로덕션 설정 파일

권장 주기가 적용된 프로덕션 설정: [`configs/config.production.json`](../configs/config.production.json)

---

## 기본 Collectors

### CPU Collector

시스템 전체 CPU 사용률을 수집합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"10s"` |

```json
{
  "collectors": {
    "cpu": {
      "enabled": true,
      "interval": "10s"
    }
  }
}
```

#### 출력 예시

```json
{
  "type": "cpu",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "total_percent": 23.5,
    "per_cpu": [15.2, 31.8, 20.1, 27.0],
    "load_average": {
      "load1": 1.25,
      "load5": 1.10,
      "load15": 0.95
    }
  }
}
```

#### 수집 항목

- `total_percent`: 전체 CPU 사용률 (%)
- `per_cpu`: 코어별 CPU 사용률 배열
- `load_average`: 시스템 로드 평균 (Linux만 해당)

#### Windows 작업관리자와의 값 차이

ResourceAgent의 CPU 사용률은 Windows 작업관리자 성능 탭보다 **10~15% 낮게** 표시될 수 있습니다. 이는 두 도구가 서로 다른 Windows 성능 카운터를 사용하기 때문입니다.

| 항목 | ResourceAgent | 작업관리자 |
|------|--------------|-----------|
| 성능 카운터 | `% Processor Time` | `% Processor Utility` |
| Windows API | `GetSystemTimes()` | `NtQuerySystemInformation()` |
| 측정 방식 | 시간 기반 (CPU가 바쁜 시간 비율) | 유틸리티 기반 (전원 관리 상태 포함) |

**차이 원인**: `% Processor Utility`는 CPU parking, C-state 등 Windows 전원 관리 상태를 반영하여 "유효 활용률"을 계산합니다. Turbo Boost가 없는 환경에서도 두 값은 차이가 발생합니다.

**검증 방법** (PowerShell):

```powershell
Get-Counter '\Processor(_Total)\% Processor Time', '\Processor Information(_Total)\% Processor Utility'
```

- `% Processor Time` ≈ ResourceAgent 값
- `% Processor Utility` ≈ 작업관리자 값

> **참고**: ResourceAgent의 측정값은 정확합니다. gopsutil 라이브러리가 사용하는 `GetSystemTimes()` API는 업계 표준 방식이며, Prometheus node_exporter 등 대부분의 모니터링 도구가 동일한 방식을 사용합니다.

---

### Memory Collector

시스템 메모리 사용량을 수집합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"10s"` |

```json
{
  "collectors": {
    "memory": {
      "enabled": true,
      "interval": "10s"
    }
  }
}
```

#### 출력 예시

```json
{
  "type": "memory",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "total_bytes": 17179869184,
    "used_bytes": 8589934592,
    "available_bytes": 8589934592,
    "used_percent": 50.0,
    "swap_total_bytes": 4294967296,
    "swap_used_bytes": 1073741824,
    "swap_used_percent": 25.0
  }
}
```

#### 수집 항목

- `total_bytes`: 전체 물리 메모리 (bytes)
- `used_bytes`: 사용 중인 메모리 (bytes)
- `available_bytes`: 사용 가능한 메모리 (bytes)
- `used_percent`: 메모리 사용률 (%)
- `swap_*`: 스왑 메모리 관련 정보

---

### Disk Collector

디스크 사용량과 I/O 통계를 수집합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"30s"` |
| `disks` | []string | 수집할 디스크 목록 (빈 배열 = 전체) | `[]` |

```json
{
  "collectors": {
    "disk": {
      "enabled": true,
      "interval": "30s",
      "disks": ["C:", "D:"]
    }
  }
}
```

**Linux 예시**:
```json
{
  "collectors": {
    "disk": {
      "enabled": true,
      "interval": "30s",
      "disks": ["/dev/sda1", "/dev/nvme0n1p1"]
    }
  }
}
```

#### 출력 예시

```json
{
  "type": "disk",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "disks": [
      {
        "device": "C:",
        "mountpoint": "C:",
        "fstype": "NTFS",
        "total_bytes": 500107862016,
        "used_bytes": 250053931008,
        "free_bytes": 250053931008,
        "used_percent": 50.0,
        "read_bytes": 1073741824,
        "write_bytes": 536870912,
        "read_count": 10000,
        "write_count": 5000
      }
    ]
  }
}
```

#### 수집 항목

- `device`: 디바이스 이름
- `mountpoint`: 마운트 포인트
- `fstype`: 파일시스템 타입
- `total_bytes`, `used_bytes`, `free_bytes`: 용량 정보
- `used_percent`: 사용률 (%)
- `read_bytes`, `write_bytes`: I/O 바이트
- `read_count`, `write_count`: I/O 횟수

---

### Network Collector

네트워크 인터페이스 트래픽을 수집합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"10s"` |
| `interfaces` | []string | 수집할 인터페이스 목록 (빈 배열 = 전체) | `[]` |

```json
{
  "collectors": {
    "network": {
      "enabled": true,
      "interval": "10s",
      "interfaces": ["Ethernet", "Wi-Fi"]
    }
  }
}
```

**Linux 예시**:
```json
{
  "collectors": {
    "network": {
      "enabled": true,
      "interval": "10s",
      "interfaces": ["eth0", "wlan0"]
    }
  }
}
```

#### 출력 예시

```json
{
  "type": "network",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "interfaces": [
      {
        "name": "Ethernet",
        "bytes_sent": 1073741824,
        "bytes_recv": 2147483648,
        "packets_sent": 1000000,
        "packets_recv": 2000000,
        "errors_in": 0,
        "errors_out": 0,
        "drop_in": 0,
        "drop_out": 0
      }
    ]
  }
}
```

---

## 프로세스 Collectors

### CPU Process Collector

CPU 사용률이 높은 상위 프로세스를 수집합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"30s"` |
| `top_n` | int | 상위 N개 프로세스 | `10` |
| `watch_processes` | []string | 항상 모니터링할 프로세스 이름 | `[]` |

```json
{
  "collectors": {
    "cpu_process": {
      "enabled": true,
      "interval": "30s",
      "top_n": 10,
      "watch_processes": ["chrome.exe", "code.exe", "java.exe"]
    }
  }
}
```

#### watch_processes 기능

`watch_processes`에 지정된 프로세스는 CPU 사용률 순위에 관계없이 **항상** 수집 결과에 포함됩니다:

- Top N에 포함된 경우: 중복 없이 한 번만 표시
- Top N에 미포함 시: 별도로 추가 수집

**사용 사례**:
- 특정 애플리케이션 모니터링 (예: 공장 MES 프로그램)
- 중요 서비스 상태 확인

#### 출력 예시

```json
{
  "type": "cpu_process",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "processes": [
      {
        "pid": 1234,
        "name": "chrome.exe",
        "cpu_percent": 15.5,
        "memory_percent": 8.2,
        "status": "running",
        "username": "user"
      },
      {
        "pid": 5678,
        "name": "code.exe",
        "cpu_percent": 12.3,
        "memory_percent": 5.1,
        "status": "running",
        "username": "user"
      }
    ]
  }
}
```

---

### Memory Process Collector

메모리 사용량이 높은 상위 프로세스를 수집합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"30s"` |
| `top_n` | int | 상위 N개 프로세스 | `10` |
| `watch_processes` | []string | 항상 모니터링할 프로세스 이름 | `[]` |

```json
{
  "collectors": {
    "memory_process": {
      "enabled": true,
      "interval": "30s",
      "top_n": 10,
      "watch_processes": ["sqlservr.exe", "mysqld"]
    }
  }
}
```

#### 출력 예시

```json
{
  "type": "memory_process",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "processes": [
      {
        "pid": 2345,
        "name": "sqlservr.exe",
        "memory_bytes": 2147483648,
        "memory_percent": 12.5,
        "cpu_percent": 5.2,
        "status": "running",
        "username": "SYSTEM"
      }
    ]
  }
}
```

---

## 하드웨어 모니터링 Collectors

> **요구사항**: Windows 전용, 관리자 권한 필요, PawnIO 드라이버 설치 필요

하드웨어 모니터링 수집기들은 LibreHardwareMonitor(LHM)를 통해 센서 데이터를 수집합니다.

### 아키텍처

```
ResourceAgent (Go)
       │
       ▼ exec.Command()
LhmHelper.exe (C#/.NET 8)
       │
       ▼ LibreHardwareMonitorLib
PawnIO Driver (Kernel)
       │
       ▼ Hardware Access
CPU/GPU/Storage/Motherboard Sensors
```

### 공통 요구사항

1. **LhmHelper.exe**: ResourceAgent.exe와 같은 폴더에 위치
2. **PawnIO 드라이버**: 설치 필요 (재부팅 불필요)
3. **관리자 권한**: 하드웨어 센서 접근에 필요

---

### Temperature Collector

CPU 온도를 수집합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"30s"` |
| `include_zones` | []string | 포함할 센서 이름 (빈 배열 = 전체) | `[]` |

```json
{
  "collectors": {
    "temperature": {
      "enabled": true,
      "interval": "30s",
      "include_zones": ["CPU Package", "Core 0", "Core 1"]
    }
  }
}
```

#### 출력 예시

```json
{
  "type": "temperature",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "sensors": [
      {
        "name": "Intel Core i7-10700 - CPU Package",
        "temperature_celsius": 45.5,
        "high_celsius": 100,
        "critical_celsius": 105
      },
      {
        "name": "Intel Core i7-10700 - Core 0",
        "temperature_celsius": 43.0,
        "high_celsius": 100,
        "critical_celsius": 105
      }
    ]
  }
}
```

#### 플랫폼별 동작

| 플랫폼 | 데이터 소스 |
|--------|------------|
| Windows | LibreHardwareMonitor (LhmHelper.exe) |
| Linux | `/sys/class/thermal/thermal_zone*/temp` 또는 lm-sensors |

---

### Fan Collector

팬 속도를 수집합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"30s"` |
| `include_zones` | []string | 포함할 팬 이름 (빈 배열 = 전체) | `[]` |

```json
{
  "collectors": {
    "fan": {
      "enabled": true,
      "interval": "30s",
      "include_zones": ["CPU Fan", "System Fan"]
    }
  }
}
```

#### 출력 예시

```json
{
  "type": "fan",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "fans": [
      {
        "name": "CPU Fan",
        "speed_rpm": 1200,
        "min_rpm": 500,
        "max_rpm": 2000
      },
      {
        "name": "System Fan #1",
        "speed_rpm": 800,
        "min_rpm": 400,
        "max_rpm": 1500
      }
    ]
  }
}
```

#### 호환성

- **지원율**: 60~70% (일부 메인보드/칩셋에서만 지원)
- 미지원 시 빈 배열 반환

---

### GPU Collector

GPU 온도, 로드, 전력, 클럭 등을 수집합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"30s"` |
| `include_zones` | []string | 포함할 GPU 이름 (빈 배열 = 전체) | `[]` |

```json
{
  "collectors": {
    "gpu": {
      "enabled": true,
      "interval": "30s",
      "include_zones": []
    }
  }
}
```

#### 출력 예시

```json
{
  "type": "gpu",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "gpus": [
      {
        "name": "NVIDIA GeForce RTX 3080",
        "temperature_celsius": 65.5,
        "core_load_percent": 45.2,
        "memory_load_percent": 32.1,
        "fan_speed_rpm": 1800,
        "power_watts": 285.0,
        "core_clock_mhz": 1905.0,
        "memory_clock_mhz": 9501.0
      }
    ]
  }
}
```

#### 수집 항목

| 필드 | 설명 | 단위 |
|------|------|------|
| `name` | GPU 이름 | - |
| `temperature_celsius` | GPU 온도 | °C |
| `core_load_percent` | GPU 코어 사용률 | % |
| `memory_load_percent` | GPU 메모리 사용률 | % |
| `fan_speed_rpm` | GPU 팬 속도 | RPM |
| `power_watts` | 전력 소비 | W |
| `core_clock_mhz` | 코어 클럭 | MHz |
| `memory_clock_mhz` | 메모리 클럭 | MHz |

#### 지원 GPU

- **NVIDIA**: GeForce GTX/RTX 시리즈 (80~90% 지원)
- **AMD**: Radeon RX 시리즈 (70~80% 지원)
- **Intel**: Arc 시리즈 (제한적)

---

### Storage S.M.A.R.T Collector

디스크 S.M.A.R.T 상태를 수집합니다. 디스크 수명 예측 및 장애 조기 감지에 유용합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"60s"` |
| `disks` | []string | 수집할 디스크 이름 (빈 배열 = 전체) | `[]` |

```json
{
  "collectors": {
    "storage_smart": {
      "enabled": true,
      "interval": "60s",
      "disks": []
    }
  }
}
```

#### 출력 예시

```json
{
  "type": "storage_smart",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "storages": [
      {
        "name": "Samsung 970 EVO Plus 1TB",
        "type": "NVMe",
        "temperature_celsius": 38.0,
        "remaining_life_percent": 98.5,
        "media_errors": 0,
        "power_cycles": 1250,
        "unsafe_shutdowns": 5,
        "power_on_hours": 8760,
        "total_bytes_written": 52428800000000
      },
      {
        "name": "WD Blue 2TB",
        "type": "HDD",
        "temperature_celsius": 35.0,
        "remaining_life_percent": null,
        "media_errors": null,
        "power_cycles": 500,
        "unsafe_shutdowns": null,
        "power_on_hours": 15000,
        "total_bytes_written": null
      }
    ]
  }
}
```

#### 수집 항목

| 필드 | 설명 | NVMe | SATA SSD | HDD |
|------|------|------|----------|-----|
| `name` | 디스크 이름 | ✓ | ✓ | ✓ |
| `type` | 디스크 타입 | ✓ | ✓ | ✓ |
| `temperature_celsius` | 온도 | ✓ | ✓ | ✓ |
| `remaining_life_percent` | 잔여 수명 | ✓ | ✓ | - |
| `media_errors` | 미디어 에러 수 | ✓ | - | - |
| `power_cycles` | 전원 사이클 | ✓ | ✓ | ✓ |
| `unsafe_shutdowns` | 비정상 종료 횟수 | ✓ | - | - |
| `power_on_hours` | 가동 시간 | ✓ | ✓ | ✓ |
| `total_bytes_written` | 총 기록량 | ✓ | ✓ | - |

#### 사용 사례

- **잔여 수명 모니터링**: `remaining_life_percent < 20%` 시 디스크 교체 알림
- **미디어 에러 감지**: `media_errors > 0` 시 데이터 손실 위험 경고
- **비정상 종료 추적**: 전원 품질 문제 파악

---

### Voltage Collector

PSU(전원 공급 장치) 전압을 수집합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"30s"` |
| `include_zones` | []string | 포함할 전압 센서 이름 (빈 배열 = 전체) | `[]` |

```json
{
  "collectors": {
    "voltage": {
      "enabled": true,
      "interval": "30s",
      "include_zones": ["+12V", "+5V", "+3.3V", "CPU Vcore"]
    }
  }
}
```

#### 출력 예시

```json
{
  "type": "voltage",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "sensors": [
      {
        "name": "+12V",
        "voltage": 12.096
      },
      {
        "name": "+5V",
        "voltage": 5.040
      },
      {
        "name": "+3.3V",
        "voltage": 3.312
      },
      {
        "name": "CPU Vcore",
        "voltage": 1.225
      }
    ]
  }
}
```

#### 주요 전압 레일

| 레일 | 정상 범위 | 용도 |
|------|----------|------|
| +12V | 11.4V ~ 12.6V | CPU, GPU, 모터 |
| +5V | 4.75V ~ 5.25V | USB, SATA, 로직 회로 |
| +3.3V | 3.14V ~ 3.47V | 메모리, 칩셋 |
| CPU Vcore | 0.8V ~ 1.5V | CPU 코어 전압 |

#### 호환성

- **지원율**: 50~60% (SuperIO 칩 지원 메인보드)
- 미지원 시 빈 배열 반환

---

### Motherboard Temperature Collector

메인보드 온도 센서를 수집합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"30s"` |
| `include_zones` | []string | 포함할 센서 이름 (빈 배열 = 전체) | `[]` |

```json
{
  "collectors": {
    "motherboard_temp": {
      "enabled": true,
      "interval": "30s",
      "include_zones": ["System", "Chipset", "VRM"]
    }
  }
}
```

#### 출력 예시

```json
{
  "type": "motherboard_temp",
  "timestamp": "2026-02-05T10:00:00Z",
  "data": {
    "sensors": [
      {
        "name": "System",
        "temperature_celsius": 35.0
      },
      {
        "name": "Chipset",
        "temperature_celsius": 42.5
      },
      {
        "name": "VRM",
        "temperature_celsius": 48.0
      }
    ]
  }
}
```

#### 주요 센서

| 센서 | 설명 | 주의 온도 |
|------|------|----------|
| System | 시스템 전체 온도 | > 45°C |
| Chipset | 칩셋(PCH) 온도 | > 70°C |
| VRM | 전압 조정 모듈 온도 | > 80°C |

#### 호환성

- **지원율**: 50~60%
- SuperIO 칩 지원 메인보드에서만 동작

---

## 시스템 Collectors

### Uptime Collector

시스템 부팅 시각과 가동 경과 시간을 수집합니다.

#### 설정

```json
{
  "uptime": {
    "Enabled": true,
    "Interval": "300s"
  }
}
```

#### 출력 데이터

| 필드 | 타입 | 설명 |
|------|------|------|
| `boot_time_unix` | int64 | 마지막 부팅 시각 (Unix timestamp, 초) |
| `boot_time` | string | 마지막 부팅 시각 (로컬 시간, ISO 8601) |
| `uptime_minutes` | float64 | 부팅 이후 경과 시간 (분) |

#### EARS 출력

```
category:uptime,pid:0,proc:@system,metric:boot_time_unix,value:1740614400
category:uptime,pid:0,proc:@system,metric:uptime_minutes,value:1440
```

#### Grafana 표시

`boot_time_unix` 값은 Grafana에서 Unit을 **"Datetime > From (Unix seconds)"**로 설정하면 로컬 타임존에 맞는 날짜/시각 텍스트로 표시됩니다.

#### 권장 주기

| 환경 | 주기 | 이유 |
|------|------|------|
| 프로덕션 | 300s (5분) | 재부팅은 드물게 발생, 자주 수집할 필요 없음 |
| 디버깅 | 60s (1분) | 재부팅 감지 빠르게 |

#### 플랫폼

- **Windows**: `gopsutil/host.BootTime()` (Windows API)
- **Linux**: `gopsutil/host.BootTime()` (`/proc/uptime`)
- **macOS**: `gopsutil/host.BootTime()` (sysctl)

---

### ProcessWatch Collector

공장 PC에서 반드시 실행되어야 하는 필수 프로세스와, 실행되면 안 되는 금지 프로세스를 감시합니다.

#### 설정

| 필드 | 타입 | 설명 | 기본값 |
|------|------|------|--------|
| `enabled` | boolean | 활성화 여부 | `true` |
| `interval` | string | 수집 주기 | `"60s"` |
| `required_processes` | []string | 반드시 실행 중이어야 하는 프로세스 이름 | `[]` |
| `forbidden_processes` | []string | 실행되면 안 되는 프로세스 이름 | `[]` |

```json
{
  "collectors": {
    "process_watch": {
      "enabled": true,
      "interval": "60s",
      "required_processes": ["mes_client.exe", "scada_hmi.exe", "plc_driver.exe"],
      "forbidden_processes": ["teamviewer.exe", "anydesk.exe", "chrome.exe"]
    }
  }
}
```

#### 동작 원리

- **필수 프로세스 (required)**: 목록의 각 프로세스가 실행 중인지 확인. `value=1`이면 정상(실행 중), `value=0`이면 알람(프로세스 다운)
- **금지 프로세스 (forbidden)**: 목록의 각 프로세스가 실행 중인지 확인. `value=1`이면 알람(비인가 프로세스 감지), `value=0`이면 정상(미실행)

#### 출력 예시

```json
{
  "type": "process_watch",
  "timestamp": "2026-02-27T10:00:00Z",
  "data": {
    "processes": [
      {
        "name": "mes_client.exe",
        "pid": 1234,
        "type": "required",
        "running": true
      },
      {
        "name": "scada_hmi.exe",
        "pid": 0,
        "type": "required",
        "running": false
      },
      {
        "name": "teamviewer.exe",
        "pid": 5678,
        "type": "forbidden",
        "running": true
      }
    ]
  }
}
```

#### EARS 출력

```
category:process_watch,pid:1234,proc:mes_client.exe,metric:required,value:1
category:process_watch,pid:0,proc:scada_hmi.exe,metric:required,value:0
category:process_watch,pid:5678,proc:teamviewer.exe,metric:forbidden,value:1
category:process_watch,pid:0,proc:anydesk.exe,metric:forbidden,value:0
```

#### 알람 조건

| 타입 | value | 의미 | 알람 |
|------|-------|------|------|
| required | 1 | 실행 중 | 정상 |
| required | 0 | 미실행 | 알람 (프로세스 다운) |
| forbidden | 1 | 실행 중 | 알람 (비인가 프로세스) |
| forbidden | 0 | 미실행 | 정상 |

#### 사용 사례

- **공장 MES/SCADA 필수 프로세스 감시**: 핵심 프로세스 다운 시 즉시 알림
- **비인가 소프트웨어 탐지**: 원격 제어 프로그램, 개인 브라우저 등 사용 금지 프로세스 감지
- **보안 컴플라이언스**: 운영 정책에 따른 프로세스 허용/차단 모니터링

#### 권장 주기

| 환경 | 주기 | 이유 |
|------|------|------|
| 프로덕션 | 60s (1분) | 프로세스 상태 변경은 즉각적이지만, 너무 빈번한 수집은 불필요 |
| 보안 중시 | 30s | 비인가 프로세스 빠른 탐지 |

#### 플랫폼

- **Windows**: `gopsutil/process` (Windows API)
- **Linux**: `gopsutil/process` (`/proc`)
- **macOS**: `gopsutil/process` (개발/테스트용)

---

## 플랫폼별 지원 현황

| Collector | Windows | Linux | macOS |
|-----------|---------|-------|-------|
| cpu | ✓ | ✓ | ✓ |
| memory | ✓ | ✓ | ✓ |
| disk | ✓ | ✓ | ✓ |
| network | ✓ | ✓ | ✓ |
| cpu_process | ✓ | ✓ | ✓ |
| memory_process | ✓ | ✓ | ✓ |
| temperature | ✓ (LHM) | ✓ (sysfs) | ✓ (limited) |
| fan | ✓ (LHM) | - | - |
| gpu | ✓ (LHM) | - | - |
| storage_smart | ✓ (LHM) | - | - |
| voltage | ✓ (LHM) | - | - |
| motherboard_temp | ✓ (LHM) | - | - |
| uptime | ✓ | ✓ | ✓ |
| process_watch | ✓ | ✓ | ✓ |

> Linux/macOS에서 LHM 기반 수집기는 빈 데이터를 반환합니다 (에러 아님).

---

## 전체 설정 예시

### 최소 설정 (CPU/Memory만)

```json
{
  "sender_type": "file",
  "file": {
    "file_path": "metrics.jsonl",
    "console": true
  },
  "collectors": {
    "cpu": { "enabled": true, "interval": "10s" },
    "memory": { "enabled": true, "interval": "10s" },
    "disk": { "enabled": false },
    "network": { "enabled": false },
    "temperature": { "enabled": false },
    "cpu_process": { "enabled": false },
    "memory_process": { "enabled": false },
    "fan": { "enabled": false },
    "gpu": { "enabled": false },
    "storage_smart": { "enabled": false },
    "voltage": { "enabled": false },
    "motherboard_temp": { "enabled": false },
    "uptime": { "enabled": false },
    "process_watch": { "enabled": false }
  }
}
```

### 전체 하드웨어 모니터링 (Windows)

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
  "kafka": {
    "brokers": ["kafka.example.com:9092"],
    "topic": "factory-metrics",
    "compression": "snappy"
  },
  "collectors": {
    "cpu": {
      "enabled": true,
      "interval": "10s"
    },
    "memory": {
      "enabled": true,
      "interval": "10s"
    },
    "disk": {
      "enabled": true,
      "interval": "30s",
      "disks": ["C:", "D:"]
    },
    "network": {
      "enabled": true,
      "interval": "10s",
      "interfaces": ["Ethernet"]
    },
    "temperature": {
      "enabled": true,
      "interval": "30s"
    },
    "cpu_process": {
      "enabled": true,
      "interval": "30s",
      "top_n": 10,
      "watch_processes": ["mes.exe", "scada.exe"]
    },
    "memory_process": {
      "enabled": true,
      "interval": "30s",
      "top_n": 10,
      "watch_processes": ["sqlservr.exe"]
    },
    "fan": {
      "enabled": true,
      "interval": "30s"
    },
    "gpu": {
      "enabled": true,
      "interval": "30s"
    },
    "storage_smart": {
      "enabled": true,
      "interval": "60s"
    },
    "voltage": {
      "enabled": true,
      "interval": "30s"
    },
    "motherboard_temp": {
      "enabled": true,
      "interval": "30s"
    },
    "uptime": {
      "enabled": true,
      "interval": "300s"
    },
    "process_watch": {
      "enabled": true,
      "interval": "60s",
      "required_processes": ["mes.exe", "scada.exe"],
      "forbidden_processes": ["teamviewer.exe"]
    }
  },
  "logging": {
    "level": "info",
    "file_path": "logs/agent.log",
    "console": false
  }
}
```

### 특정 프로세스 모니터링

공장 MES/SCADA 시스템 전용 설정:

```json
{
  "collectors": {
    "cpu": { "enabled": true, "interval": "5s" },
    "memory": { "enabled": true, "interval": "5s" },
    "cpu_process": {
      "enabled": true,
      "interval": "10s",
      "top_n": 5,
      "watch_processes": [
        "mes_client.exe",
        "scada_hmi.exe",
        "plc_driver.exe",
        "oracle.exe",
        "opc_server.exe"
      ]
    },
    "memory_process": {
      "enabled": true,
      "interval": "10s",
      "top_n": 5,
      "watch_processes": [
        "mes_client.exe",
        "scada_hmi.exe",
        "oracle.exe"
      ]
    },
    "disk": { "enabled": false },
    "network": { "enabled": false },
    "temperature": { "enabled": false },
    "fan": { "enabled": false },
    "gpu": { "enabled": false },
    "storage_smart": { "enabled": false },
    "voltage": { "enabled": false },
    "motherboard_temp": { "enabled": false },
    "uptime": { "enabled": false },
    "process_watch": {
      "enabled": true,
      "interval": "30s",
      "required_processes": [
        "mes_client.exe",
        "scada_hmi.exe",
        "plc_driver.exe"
      ],
      "forbidden_processes": [
        "teamviewer.exe",
        "anydesk.exe"
      ]
    }
  }
}
```

---

## 문제 해결

### 빈 데이터 반환

| 증상 | 원인 | 해결 |
|------|------|------|
| 온도/팬/GPU 빈 배열 | LhmHelper.exe 미설치 | LhmHelper.exe를 agent와 같은 폴더에 복사 |
| 온도/팬/GPU 빈 배열 | PawnIO 드라이버 미설치 | PawnIO 드라이버 설치 |
| 온도/팬/GPU 빈 배열 | 관리자 권한 부족 | 관리자 권한으로 실행 |
| 프로세스 빈 배열 | 권한 부족 | 관리자 권한으로 실행 |

### LhmHelper 테스트

```powershell
# 관리자 권한 PowerShell에서
cd C:\Path\To\ResourceAgent
.\LhmHelper.exe
```

정상 출력 예시:
```json
{
  "Sensors": [...],
  "Fans": [...],
  "Gpus": [...],
  "Storages": [...],
  "Voltages": [...],
  "MotherboardTemps": [...]
}
```

### 로그 확인

```bash
# 로그 레벨을 debug로 변경
{
  "logging": {
    "level": "debug",
    "console": true
  }
}
```

---

## 관련 문서

- [Windows 온도 수집 테스트 가이드](./TESTING-LHM-TEMPERATURE.md)
- [OhmGraphite vs ResourceAgent 분석](./OhmGraphite-vs-ResourceAgent-분석.md)
- [PDCA Report: lhm-hardware-extension](./04-report/features/lhm-hardware-extension.report.md)
