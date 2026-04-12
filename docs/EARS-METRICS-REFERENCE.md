# EARS Grok Format 메트릭 레퍼런스

EARS 변환 경로(file grok, kafkarest, kafka with eqpInfo)로 전송되는 모든 메트릭의 category, metric, proc, value 정의입니다.

## EARS Row 형식

```
{timestamp} category:{category},pid:{pid},proc:{proc},metric:{metric},value:{value}
```

**예시:**
```
2026-02-25 10:30:45,123 category:cpu,pid:0,proc:@system,metric:total_used_pct,value:45.5
```

### sanitizeName (모든 EARS 변환)

모든 EARS 변환(`ToGrokString()`, `ToParsedData()`)에서 `proc`과 `metric` 필드에 `sanitizeName()` 적용. 하드웨어 센서 이름의 특수문자를 제거하여 ES 인서트 오류를 방지하고, 경로(kafkarest/kafka/file) 무관하게 동일한 값을 보장합니다.

**규칙**: 괄호 제거 → `[^a-zA-Z0-9_:.@-]` → `_` → 연속 `_` 축소

| 원본 (센서 이름) | sanitize 후 |
|-----------------|------------|
| `Intel(R) HD Graphics 530_power` | `IntelR_HD_Graphics_530_power` |
| `CPU Core #1 Distance to TjMax` | `CPU_Core_1_Distance_to_TjMax` |
| `Loopback Pseudo-Interface 1` | `Loopback_Pseudo-Interface_1` |
| `C:` | `C:` (변경 없음) |
| `total_used_pct` | `total_used_pct` (변경 없음) |

### ESID 형식

```
{Process}:{EqpID}-{metricType}-{timestamp_ms}-{counter}
```

- `metricType`: collector 타입 (cpu, memory, disk, network, temperature, gpu 등)
- `counter`: MetricData 내 EARS row 인덱스 (0부터)
- 동일 timestamp라도 metricType이 달라 타입 간 중복 없음

**예시:** `PROCESS1:EQP001-memory-1740000000000-1`

---

## 시스템 메트릭 (proc=@system, pid=0)

### cpu

| metric | 설명 | 단위 | 값 범위 | 예시 |
|--------|------|------|---------|------|
| `total_used_pct` | 전체 CPU 사용률 | % | 0~100 | `45.5` |
| `core_{N}_used_pct` | 코어별 사용률 (N=0~CoreCount-1) | % | 0~100 | `42.1` |

### memory

| metric | 설명 | 단위 | 값 범위 | 예시 |
|--------|------|------|---------|------|
| `total_used_pct` | 메모리 사용률 | % | 0~100 | `75.0` |
| `total_free_pct` | 메모리 여유률 | % | 0~100 | `25.0` |
| `total_used_size` | 사용 중인 메모리 크기 | bytes | 0~ | `12000000000` |

### disk

마운트된 파티션마다 1개 row 생성.

| metric | 설명 | 단위 | 값 범위 | 예시 |
|--------|------|------|---------|------|
| `{Mountpoint}` | 파티션 사용률 (metric 이름이 마운트포인트) | % | 0~100 | metric=`C:`, value=`60.0` |

**출력 예시:**
```
category:disk,pid:0,proc:@system,metric:C:,value:60
category:disk,pid:0,proc:@system,metric:D:,value:30
```

### network

TCP 커넥션 수는 proc=`@system`, 인터페이스별 rate는 proc=`{NIC이름}`.

| proc | metric | 설명 | 단위 | 예시 |
|------|--------|------|------|------|
| `@system` | `all_inbound` | 서버측 TCP 커넥션 수 (LISTEN 포트 기반) | 개 | `42` |
| `@system` | `all_outbound` | 클라이언트측 TCP 커넥션 수 | 개 | `38` |
| `{NIC이름}` | `recv_rate` | 인터페이스 수신 속도 | bytes/sec | `12345.6` |
| `{NIC이름}` | `sent_rate` | 인터페이스 송신 속도 | bytes/sec | `678.9` |

**출력 예시:**
```
category:network,pid:0,proc:@system,metric:all_inbound,value:42
category:network,pid:0,proc:@system,metric:all_outbound,value:38
category:network,pid:0,proc:Ethernet,metric:recv_rate,value:12345.6
category:network,pid:0,proc:Ethernet,metric:sent_rate,value:678.9
category:network,pid:0,proc:Wi-Fi,metric:recv_rate,value:100
category:network,pid:0,proc:Wi-Fi,metric:sent_rate,value:50
```

### temperature

센서마다 1개 row 생성.

| metric | 설명 | 단위 | 예시 |
|--------|------|------|------|
| `{센서이름}` | CPU 온도 | °C | 원본: `Intel Core i7 - CPU Package` → sanitize 후 metric=`Intel_Core_i7_-_CPU_Package`, value=`65.0` |

### fan

팬 센서마다 1개 row 생성.

| metric | 설명 | 단위 | 예시 |
|--------|------|------|------|
| `{팬이름}` | 팬 회전 속도 | RPM | 원본: `CPU Fan` → sanitize 후 metric=`CPU_Fan`, value=`1200` |

### voltage

전압 센서마다 1개 row 생성.

| metric | 설명 | 단위 | 예시 |
|--------|------|------|------|
| `{센서이름}` | 전압 | V | 원본: `CPU Vcore` → sanitize 후 metric=`CPU_Vcore`, value=`1.25` |

### motherboard_temp

메인보드 온도 센서마다 1개 row 생성.

| metric | 설명 | 단위 | 예시 |
|--------|------|------|------|
| `{센서이름}` | 메인보드 온도 | °C | metric=`System`, value=`42.0` |

### gpu

GPU마다 non-nil 필드당 1개 row 생성. metric 이름은 `{GPU이름}_{필드}`.

| metric 패턴 | 설명 | 단위 | 예시 |
|-------------|------|------|------|
| `{Name}_temperature` | GPU 온도 | °C | `RTX4090_temperature` = `75.0` |
| `{Name}_core_load` | GPU 코어 사용률 | % | `RTX4090_core_load` = `90.0` |
| `{Name}_memory_load` | GPU 메모리 사용률 | % | `RTX4090_memory_load` = `60.0` |
| `{Name}_fan_speed` | GPU 팬 속도 | RPM | `RTX4090_fan_speed` = `1800` |
| `{Name}_power` | 전력 소비 | W | `RTX4090_power` = `320.5` |
| `{Name}_core_clock` | 코어 클럭 | MHz | `RTX4090_core_clock` = `2520` |
| `{Name}_memory_clock` | 메모리 클럭 | MHz | `RTX4090_memory_clock` = `1200` |

### storage_smart

스토리지마다 non-nil 필드당 1개 row 생성. metric 이름은 `{디바이스이름}_{필드}`.

| metric 패턴 | 설명 | 단위 | 예시 |
|-------------|------|------|------|
| `{Name}_temperature` | 디스크 온도 | °C | `nvme0_temperature` = `35` |
| `{Name}_remaining_life` | 잔여 수명 | % | `nvme0_remaining_life` = `98` |
| `{Name}_media_errors` | 미디어 에러 수 | 개 | `nvme0_media_errors` = `0` |
| `{Name}_power_cycles` | 전원 사이클 횟수 | 회 | `nvme0_power_cycles` = `150` |
| `{Name}_unsafe_shutdowns` | 비정상 종료 횟수 | 회 | `nvme0_unsafe_shutdowns` = `3` |
| `{Name}_power_on_hours` | 총 가동 시간 | 시간 | `nvme0_power_on_hours` = `8760` |
| `{Name}_total_bytes_written` | 총 기록량 | bytes | `nvme0_total_bytes_written` = `50000000000` |

### uptime

| metric | 설명 | 단위 | 값 범위 | 예시 |
|--------|------|------|---------|------|
| `boot_time_unix` | 마지막 부팅 시각 | Unix timestamp (초) | — | `1740614400` |
| `uptime_minutes` | 부팅 이후 경과 시간 | 분 | 0~ | `1440.5` |

**출력 예시:**
```
category:uptime,pid:0,proc:@system,metric:boot_time_unix,value:1740614400
category:uptime,pid:0,proc:@system,metric:uptime_minutes,value:1440.5
```

### process_watch

필수/금지 프로세스마다 1개 row 생성. 실행 중이면 `value=1`, 미실행이면 `value=0`.

| proc | metric | 설명 | value | 알람 조건 |
|------|--------|------|-------|----------|
| `{프로세스명}` | `required` | 필수 프로세스 실행 여부 | `1`=실행 중, `0`=미실행 | value=0 시 알람 (프로세스 다운) |
| `{프로세스명}` | `forbidden` | 금지 프로세스 실행 여부 | `1`=실행 중, `0`=미실행 | value=1 시 알람 (비인가 프로세스) |

- pid: 실행 중이면 해당 PID, 미실행이면 `0`

**출력 예시:**
```
category:process_watch,pid:1234,proc:mes_client.exe,metric:required,value:1
category:process_watch,pid:0,proc:scada_hmi.exe,metric:required,value:0
category:process_watch,pid:5678,proc:teamviewer.exe,metric:forbidden,value:1
category:process_watch,pid:0,proc:anydesk.exe,metric:forbidden,value:0
```

---

## 프로세스 메트릭 (proc={프로세스명}, pid={PID})

### cpu (cpu_process collector)

프로세스마다 1개 row 생성.

| category | metric | 설명 | 단위 | 예시 |
|----------|--------|------|------|------|
| `cpu` | `used_pct` | 프로세스 CPU 사용률 | % | pid=`1234`, proc=`python.exe`, value=`12.5` |

### memory (memory_process collector)

프로세스마다 2개 rows 생성.

| category | metric | 설명 | 단위 | 예시 |
|----------|--------|------|------|------|
| `memory` | `used` | 프로세스 RSS (Resident Set Size) | bytes | pid=`1234`, proc=`python.exe`, value=`104857600` |
| `memory` | `used_pct` | 프로세스 메모리 사용률 | % | pid=`1234`, proc=`python.exe`, value=`12.5` |

### storage_health (storage_health collector)

디스크마다 1개 row 생성. LhmHelper 불필요. Windows: WMI, Linux: smartctl.

| category | metric | 설명 | 단위 | 예시 |
|----------|--------|------|------|------|
| `storage_health` | `{disk}_status` | 디스크 건강 상태 | 0=OK, 1=DEGRADED, 2=PRED_FAIL, 3=FAIL, -1=UNKNOWN | proc=`@system`, metric=`Samsung_SSD_860_PRO_status`, value=`0` |

**상태값 매핑:**

| Status | value | 의미 | 매핑되는 원문 |
|--------|-------|------|-------------|
| OK | 0 | 정상 | OK, PASSED |
| DEGRADED | 1 | 성능 저하 | Degraded, Stressed |
| PRED_FAIL | 2 | 예측 고장 (교체 필요) | Pred Fail |
| FAIL | 3 | 실패/에러 (즉시 조치) | FAILED!, Error, NonRecover |
| UNKNOWN | -1 | 판별 불가 | 빈값, Unknown, smartctl 미설치 |

---

## 전체 출력 예시

```
2026-02-25 10:30:45,123 category:cpu,pid:0,proc:@system,metric:total_used_pct,value:45.5
2026-02-25 10:30:45,123 category:memory,pid:0,proc:@system,metric:total_used_pct,value:75
2026-02-25 10:30:45,123 category:memory,pid:0,proc:@system,metric:total_free_pct,value:25
2026-02-25 10:30:45,123 category:memory,pid:0,proc:@system,metric:total_used_size,value:12000000000
2026-02-25 10:30:45,123 category:disk,pid:0,proc:@system,metric:C:,value:60
2026-02-25 10:30:45,123 category:disk,pid:0,proc:@system,metric:D:,value:30
2026-02-25 10:30:45,123 category:network,pid:0,proc:@system,metric:all_inbound,value:42
2026-02-25 10:30:45,123 category:network,pid:0,proc:@system,metric:all_outbound,value:38
2026-02-25 10:30:45,123 category:network,pid:0,proc:Ethernet,metric:recv_rate,value:12345.6
2026-02-25 10:30:45,123 category:network,pid:0,proc:Ethernet,metric:sent_rate,value:678.9
2026-02-25 10:30:45,123 category:temperature,pid:0,proc:@system,metric:CPU_Package,value:65
2026-02-25 10:30:45,123 category:fan,pid:0,proc:@system,metric:CPU_Fan,value:1200
2026-02-25 10:30:45,123 category:gpu,pid:0,proc:@system,metric:RTX4090_temperature,value:75
2026-02-25 10:30:45,123 category:gpu,pid:0,proc:@system,metric:RTX4090_core_load,value:90
2026-02-25 10:30:45,123 category:gpu,pid:0,proc:@system,metric:RTX4090_memory_load,value:60
2026-02-25 10:30:45,123 category:gpu,pid:0,proc:@system,metric:RTX4090_fan_speed,value:1800
2026-02-25 10:30:45,123 category:gpu,pid:0,proc:@system,metric:RTX4090_power,value:320.5
2026-02-25 10:30:45,123 category:gpu,pid:0,proc:@system,metric:RTX4090_core_clock,value:2520
2026-02-25 10:30:45,123 category:gpu,pid:0,proc:@system,metric:RTX4090_memory_clock,value:1200
2026-02-25 10:30:45,123 category:voltage,pid:0,proc:@system,metric:CPU_Vcore,value:1.25
2026-02-25 10:30:45,123 category:motherboard_temp,pid:0,proc:@system,metric:System,value:42
2026-02-25 10:30:45,123 category:storage_smart,pid:0,proc:@system,metric:nvme0_temperature,value:35
2026-02-25 10:30:45,123 category:storage_smart,pid:0,proc:@system,metric:nvme0_remaining_life,value:98
2026-02-25 10:30:45,123 category:storage_smart,pid:0,proc:@system,metric:nvme0_media_errors,value:0
2026-02-25 10:30:45,123 category:storage_smart,pid:0,proc:@system,metric:nvme0_power_cycles,value:150
2026-02-25 10:30:45,123 category:storage_smart,pid:0,proc:@system,metric:nvme0_unsafe_shutdowns,value:3
2026-02-25 10:30:45,123 category:storage_smart,pid:0,proc:@system,metric:nvme0_power_on_hours,value:8760
2026-02-25 10:30:45,123 category:storage_smart,pid:0,proc:@system,metric:nvme0_total_bytes_written,value:50000000000
2026-02-25 10:30:45,123 category:uptime,pid:0,proc:@system,metric:boot_time_unix,value:1740614400
2026-02-25 10:30:45,123 category:uptime,pid:0,proc:@system,metric:uptime_minutes,value:1440
2026-02-25 10:30:45,123 category:process_watch,pid:1234,proc:mes_client.exe,metric:required,value:1
2026-02-25 10:30:45,123 category:process_watch,pid:0,proc:scada_hmi.exe,metric:required,value:0
2026-02-25 10:30:45,123 category:process_watch,pid:5678,proc:teamviewer.exe,metric:forbidden,value:1
2026-02-25 10:30:45,123 category:process_watch,pid:0,proc:anydesk.exe,metric:forbidden,value:0
2026-02-25 10:30:45,123 category:cpu,pid:1234,proc:python.exe,metric:used_pct,value:12.5
2026-02-25 10:30:45,123 category:cpu,pid:5678,proc:java.exe,metric:used_pct,value:8.3
2026-02-25 10:30:45,123 category:memory,pid:1234,proc:python.exe,metric:used,value:104857600
```

---

## 적용 경로

이 EARS 변환은 다음 sender 경로에서 사용됩니다:

| sender_type | 조건 | 변환 함수 |
|---|---|---|
| file | format=`grok` | `ConvertToEARSRows()` → `ToGrokString()` |
| file | format=`json` | `ConvertToEARSRows()` → `ToParsedData()` |
| kafkarest | 항상 | `PrepareRecords()` → `GrokRawFormatter` → `ToGrokString()` |
| kafka | eqpInfo 있음 | `PrepareRecords()` → `JSONRawFormatter` → `ToParsedData()` |

---

## 수집하지만 전송하지 않는 항목

아래 필드들은 collector에서 수집되어 MetricData에 포함되지만, EARSRow로 변환하지 않아 전송되지 않습니다.
10,000대 이상 공장 PC의 대규모 모니터링 환경에서 Kafka 트래픽 최소화와 실용성을 기준으로 제외했습니다.

### CPUData

| 필드 | 단위 | 제외 사유 |
|------|------|----------|
| User | % | CPU 시간 상세 비율. `total_used_pct`로 전체 사용률 파악 충분하고, 원인 분석은 프로세스별 CPU 메트릭으로 대체 |
| System | % | 동일 사유 |
| Idle | % | `100 - total_used_pct`로 산출 가능 (중복) |
| IOWait | % | Linux 전용. 디스크 병목 진단용이나 공장 PC 모니터링에서 우선순위 낮음 |
| Irq | % | 커널 인터럽트 비율. 일반 모니터링 범위 밖 |
| SoftIrq | % | 동일 사유 |
| Steal | % | VM 환경 전용. 공장 PC는 대부분 물리 머신 |
| Guest | % | VM 게스트 CPU. 해당 환경 없음 |
| CoreCount | 개 | 메타데이터(고정값). 모니터링 대상 아님 |

### MemoryData

| 필드 | 단위 | 제외 사유 |
|------|------|----------|
| TotalBytes | bytes | 고정값(하드웨어 사양). 자산관리 영역이며 시계열 모니터링 불필요 |
| AvailableBytes | bytes | `total_used_pct`에서 사용 상황 파악 가능 (중복) |
| SwapTotalBytes | bytes | 고정값 |
| SwapUsedBytes | bytes | Windows 공장 PC에서 swap 모니터링 우선순위 낮음. 물리 메모리 사용률로 메모리 압박 감지 충분 |
| SwapFreeBytes | bytes | 동일 사유 |
| SwapPercent | % | 동일 사유 |
| Cached | bytes | Linux 전용. 커널 캐시로 필요 시 해제되므로 실질 메모리 부족 지표 아님 |
| Buffers | bytes | Linux 전용. 동일 사유 |

### DiskPartition

| 필드 | 단위 | 제외 사유 |
|------|------|----------|
| Device | — | 메타데이터 |
| FSType | — | 메타데이터 |
| TotalBytes | bytes | 고정값. `usage_percent`로 용량 부족 알림 충분 |
| UsedBytes | bytes | `usage_percent`로 대체 가능 |
| FreeBytes | bytes | 동일 사유 |
| InodesTotal | 개 | Linux inode. 공장 PC(대부분 Windows)에서 해당 없음 |
| InodesUsed | 개 | 동일 사유 |
| InodesFree | 개 | 동일 사유 |
| InodesPercent | % | 동일 사유 |
| ReadBytes | bytes | 누적 I/O 카운터. rate 계산 없이 누적값 자체는 의미 제한적 |
| WriteBytes | bytes | 동일 사유 |
| ReadCount | 회 | 동일 사유 |
| WriteCount | 회 | 동일 사유 |
| ReadTime | ms | 동일 사유 |
| WriteTime | ms | 동일 사유 |

### NetworkInterface

| 필드 | 단위 | 제외 사유 |
|------|------|----------|
| BytesSent | bytes | 누적값. `sent_rate`(bytes/s)로 실시간 트래픽 파악 가능 (중복) |
| BytesRecv | bytes | 누적값. `recv_rate`로 대체 (중복) |
| PacketsSent | 개 | 누적 패킷 수. 네트워크 장비 모니터링 영역 |
| PacketsRecv | 개 | 동일 사유 |
| ErrorsIn | 개 | 누적 에러 수. NIC 수준 장애 진단용이며 네트워크 전용 모니터링 도구 영역 |
| ErrorsOut | 개 | 동일 사유 |
| DropsIn | 개 | 누적 드롭 수. 동일 사유 |
| DropsOut | 개 | 동일 사유 |

### ProcessMemory

| 필드 | 단위 | 제외 사유 |
|------|------|----------|
| MemoryPercent | % | RSS 절대값이 더 정확한 지표. 퍼센트는 전체 메모리 크기에 따라 의미 달라짐 |
| VMS | bytes | Virtual Memory Size. 공유 라이브러리, 매핑된 파일 포함이라 실제 점유량과 괴리 큼 |
| Swap | bytes | 프로세스별 swap 사용량. 시스템 수준 swap도 제외했으므로 동일 기준 적용 |

### TemperatureSensor

| 필드 | 단위 | 제외 사유 |
|------|------|----------|
| High | °C | 센서 임계 온도(하드웨어 스펙). 모니터링 값이 아닌 고정 임계값 |
| Critical | °C | 동일 사유 |

### 기타 메타데이터/플래그

| 타입 | 필드 | 제외 사유 |
|------|------|----------|
| ProcessCPU | Username, CreateTime, Watched | 식별/관리용 메타데이터. 시계열 메트릭 아님 |
| ProcessMemory | Username, CreateTime, Watched | 동일 사유 |
| StorageSmartSensor | Type | 디바이스 종류(NVMe/SSD/HDD). 고정 메타데이터 |
| UptimeData | BootTimeStr | `boot_time_unix`의 문자열 표현 (중복) |
