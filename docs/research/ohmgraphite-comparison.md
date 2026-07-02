# OhmGraphite vs ResourceAgent 비교 분석

> 작성일: 2026-02-04
> 목적: GPU 수집 및 Web API 기능 추가를 위한 기술 검토

## 1. 프로젝트 개요

| 항목 | OhmGraphite | ResourceAgent |
|------|-------------|---------------|
| **GitHub** | https://github.com/nickbabcock/OhmGraphite | 현재 프로젝트 |
| **언어** | C# (.NET 8) | Go |
| **플랫폼** | Windows 전용 | Windows + Linux |
| **하드웨어 라이브러리** | LibreHardwareMonitor (직접 링크) | gopsutil + LhmHelper (외부 프로세스) |
| **설정 포맷** | XML (App.config) | JSON (Hot Reload 지원) |
| **바이너리 크기** | ~15MB (+ LHM DLL) | ~20MB (Go 바이너리) |

---

## 2. 메트릭 수집 비교

| 메트릭 | OhmGraphite | ResourceAgent | 비고 |
|--------|:-----------:|:-------------:|------|
| CPU 사용률 | ✅ (Load %) | ✅ (상세 Per-core) | ResourceAgent가 더 상세 |
| CPU 온도 | ✅ | ✅ (LhmHelper) | 동등 |
| CPU 주파수 | ✅ | ❌ | OhmGraphite 우위 |
| 메모리 | ✅ | ✅ (상세) | 동등 |
| 디스크 용량 | ✅ | ✅ | 동등 |
| 디스크 I/O | ❌ | ✅ | ResourceAgent 우위 |
| 네트워크 | ✅ (bytes) | ✅ (bytes + rate) | ResourceAgent 우위 |
| GPU 온도/로드 | ✅ | ❌ (추가 가능) | LhmHelper 확장으로 대응 가능 |
| 팬 속도 | ✅ | ✅ | 동등 |
| 전력 (Watts) | ✅ | ❌ (추가 가능) | LhmHelper 확장으로 대응 가능 |
| 전압 | ✅ | ❌ | OhmGraphite 우위 |
| **프로세스별 CPU/메모리** | ❌ | ✅ (TopN + watch) | **ResourceAgent 핵심 기능** |
| NVMe SMART | ✅ | ❌ | OhmGraphite 우위 |

### 강점 요약
- **ResourceAgent**: 프로세스별 모니터링, 디스크 I/O, 네트워크 속도 계산, 크로스 플랫폼
- **OhmGraphite**: GPU, 전력, 전압, NVMe SMART 등 하드웨어 센서 전체

---

## 3. 데이터 전송 방식 비교

| 방식 | OhmGraphite | ResourceAgent | 비고 |
|------|:-----------:|:-------------:|------|
| **Graphite** | ✅ Push | ❌ | |
| **InfluxDB** | ✅ v1 + v2 | ❌ | |
| **Prometheus** | ✅ HTTP Pull | ❌ | MetricServer 기반 |
| **TimescaleDB** | ✅ | ❌ | |
| **Kafka** | ❌ | ✅ | **ResourceAgent 핵심** |
| **File** | ❌ | ✅ (JSONL) | |

---

## 4. 서비스 관리 CLI 비교

### OhmGraphite (OhmCli.cs)
```bash
OhmGraphite.exe run       # 포그라운드 실행
OhmGraphite.exe status    # 서비스 상태 조회
OhmGraphite.exe start     # 서비스 시작
OhmGraphite.exe stop      # 서비스 중지
OhmGraphite.exe install   # 서비스 설치
OhmGraphite.exe uninstall # 서비스 제거
```

### ResourceAgent
```bash
resourceagent -config config.json   # 실행
resourceagent -version              # 버전 출력
```

> **개선 필요**: ResourceAgent에 CLI 명령 확장 필요

---

## 5. Web Service / 원격 관리 기능

### OhmGraphite의 HTTP 기능

```csharp
// PrometheusServer.cs - Prometheus용 HTTP 엔드포인트
var server = new MetricServer(
    host: "*",           // 모든 인터페이스
    port: 4445,          // 기본 포트
    url: "metrics/",     // 경로
    useHttps: true       // HTTPS 지원
);
```

- **Pull 방식**: Prometheus가 주기적으로 `/metrics` 호출
- **메트릭 조회만**: 원격 관리/컨트롤 기능 없음
- HTTPS + 인증서 바인딩 지원

### ResourceAgent

**현재 HTTP 기능 없음** - 추가 필요

---

## 6. 아키텍처 비교

### OhmGraphite 구조
```
Program.cs → OhmCli.cs → Worker.cs → IManage
                                        ├── MetricTimer (Push: Graphite/InfluxDB/Timescale)
                                        └── PrometheusServer (Pull: HTTP)
                                              └── SensorCollector → LibreHardwareMonitor
```

### ResourceAgent 구조
```
main.go → Service → Scheduler → Collectors → Sender
              │         │            │           ├── KafkaSender
              │         │            │           └── FileSender
              │         │            ├── CPU, Memory, Disk, Network
              │         │            ├── Temperature, Fan (via LhmHelper)
              │         │            └── CPU_Process, Memory_Process
              │         └── Registry
              └── ConfigWatcher (Hot Reload)
```

---

## 7. GPU 수집 기능 추가 방안

### 현재 LhmHelper 설정
```csharp
var computer = new Computer
{
    IsCpuEnabled = true,
    IsMotherboardEnabled = true
    // IsGpuEnabled = true 추가하면 GPU 수집 가능
};
```

### LibreHardwareMonitor GPU 지원 센서

| 메트릭 | SensorType | 설명 |
|--------|------------|------|
| GPU 온도 | Temperature | 코어 온도 |
| GPU 로드 | Load | 코어/메모리/비디오엔진 사용률 (%) |
| GPU 메모리 | SmallData | 사용량 (MB) |
| GPU 팬 속도 | Fan | RPM |
| GPU 전력 | Power | Watts |
| GPU 클럭 | Clock | Core/Memory MHz |

### 지원 GPU
- **NVIDIA**: GeForce/Quadro/Tesla (NVML 사용)
- **AMD**: Radeon (ADL 사용)
- **Intel**: 내장 GPU (일부)

---

## 8. 구현 방향 결정: OhmGraphite 수정 vs ResourceAgent 확장

### 옵션 1: OhmGraphite 수정

| 장점 | 단점 |
|------|------|
| LibreHardwareMonitor 직접 링크 | Kafka Sender 처음부터 구현 필요 |
| 모든 하드웨어 센서 접근 | Windows 전용 (Linux 미지원) |
| 안정적인 오픈소스 | 프로세스 모니터링 없음 |
| | Fork 유지 관리 부담 |
| | Hot Reload 없음 |

### 옵션 2: ResourceAgent 확장 (권장)

| 장점 | 단점 |
|------|------|
| Kafka 전송 이미 완료 | LhmHelper 외부 프로세스 오버헤드 (미미) |
| 크로스 플랫폼 | 일부 센서만 사용 |
| 프로세스 모니터링 있음 | |
| JSON 설정 + Hot Reload | |
| Go 단일 바이너리 배포 | |
| 기존 아키텍처 일관성 | |

### 작업량 비교

```
OhmGraphite 수정 시 필요한 작업:
├── Kafka Sender 구현 (C#) ──────── 500+ 줄
├── 프로세스 모니터링 구현 ──────── 300+ 줄
├── JSON 설정 변환 ─────────────── 200+ 줄
├── Hot Reload 구현 ────────────── 150+ 줄
└── 테스트/통합 ────────────────── ???

ResourceAgent 확장 시 필요한 작업:
├── LhmHelper에 IsGpuEnabled 추가 ─ 1줄
├── GPU 센서 파싱 코드 ───────────── 50줄
├── GPU Collector (Go) ───────────── 100줄
└── 설정 항목 추가 ────────────────── 10줄
```

### 결론

**ResourceAgent 확장**이 압도적으로 효율적:

1. **Kafka가 이미 있음** - 핵심 요구사항 충족
2. **프로세스 모니터링** - 공장 PC 모니터링의 핵심 기능
3. **단일 코드베이스** - 유지보수 효율
4. **기존 패턴 재사용** - temperature collector와 동일한 구조
5. **크로스 플랫폼** - Linux 지원 유지

---

## 9. 향후 구현 계획

### Phase 1: GPU 수집 기능
1. LhmHelper에 GPU 센서 수집 추가
2. Go GPU Collector 구현
3. 설정 항목 추가

### Phase 2: Web API 기능
```
┌─────────────────────────────────────────────────────────────┐
│                    ResourceAgent HTTP API                    │
├─────────────────────────────────────────────────────────────┤
│  GET  /health              헬스체크 (liveness/readiness)     │
│  GET  /metrics             현재 메트릭 조회 (JSON)           │
│  GET  /metrics/prometheus  Prometheus 포맷                   │
│  GET  /status              에이전트 상태 (uptime, collectors)│
│  GET  /config              현재 설정 조회                    │
│  PUT  /config              설정 변경 (Hot Reload 트리거)     │
│  POST /collectors/{name}/enable   Collector 활성화          │
│  POST /collectors/{name}/disable  Collector 비활성화        │
│  POST /reload              설정 리로드                       │
└─────────────────────────────────────────────────────────────┘
```

### Phase 3: CLI 확장
```bash
resourceagent run       # 포그라운드 실행
resourceagent status    # 서비스 상태 조회
resourceagent start     # 서비스 시작
resourceagent stop      # 서비스 중지
resourceagent install   # 서비스 설치
resourceagent uninstall # 서비스 제거
```

---

## 10. 참고 자료

- OhmGraphite GitHub: https://github.com/nickbabcock/OhmGraphite
- LibreHardwareMonitor: https://github.com/LibreHardwareMonitor/LibreHardwareMonitor
- PawnIO Driver: LibreHardwareMonitor의 하드웨어 접근 드라이버
