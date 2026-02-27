# EARS Legacy Format 메트릭 레퍼런스

EARS 변환 경로(file legacy, kafkarest, kafka with eqpInfo)로 전송되는 모든 메트릭의 category, metric, proc, value 정의입니다.

## EARS Row 형식

```
{timestamp} category:{category},pid:{pid},proc:{proc},metric:{metric},value:{value}
```

**예시:**
```
2026-02-25 10:30:45,123 category:cpu,pid:0,proc:@system,metric:total_used_pct,value:45.5
```

### sanitizeName (Grok/KafkaRest 전용)

`ToLegacyString()`에서 `proc`과 `metric` 필드에 `sanitizeName()` 적용. 하드웨어 센서 이름의 특수문자를 제거하여 ES 인서트 오류를 방지합니다. JSON format(`ToParsedData`)에는 미적용.

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

프로세스마다 1개 row 생성.

| category | metric | 설명 | 단위 | 예시 |
|----------|--------|------|------|------|
| `memory` | `used` | 프로세스 RSS (Resident Set Size) | bytes | pid=`1234`, proc=`python.exe`, value=`104857600` |

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
| file | format=`legacy` | `ConvertToEARSRows()` → `ToLegacyString()` |
| kafkarest | 항상 | `WrapMetricDataLegacy()` → `ToLegacyString()` |
| kafka | eqpInfo 있음 | `WrapMetricDataJSON()` → `ToParsedData()` |

> **참고**: file(format=`json`)과 kafka(eqpInfo 없음)는 MetricData JSON을 그대로 전송하므로 위 변환을 거치지 않습니다.
