# Elasticsearch 예상 데이터량 산정

ResourceAgent가 수집한 메트릭이 Kafka → Elasticsearch로 적재될 때, PC 1대당 생성되는 document 수와 저장 용량을 산정합니다.

## 기준 설정

### Monitor.json (현재 기본값)

| Collector | Interval | Enabled | 비고 |
|-----------|----------|---------|------|
| cpu | 30s | O | |
| memory | 30s | O | |
| network | 30s | O | |
| disk | 60s | O | |
| temperature | 60s | O | LhmHelper 필요 (Windows) |
| cpu_process | 60s | O | TopN=10 |
| memory_process | 60s | O | TopN=10 |
| fan | 60s | O | LhmHelper 필요 |
| gpu | 60s | O | LhmHelper 필요 |
| voltage | 60s | O | LhmHelper 필요 |
| motherboard_temp | 60s | O | LhmHelper 필요 |
| storage_smart | 300s | O | LhmHelper 필요 |

### 기준 PC 하드웨어 가정

| 항목 | 가정값 | 비고 |
|------|--------|------|
| 디스크 파티션 | 2개 | C:, D: |
| 네트워크 인터페이스 | 1개 | Ethernet |
| CPU 온도 센서 | 3개 | CPU Package, Core #0, Core #1 |
| Fan 센서 | 2개 | CPU Fan, System Fan |
| Voltage 센서 | 3개 | CPU Core, +3.3V, +5V |
| Motherboard 온도 센서 | 2개 | System, Auxiliary |
| GPU | 없음 | 공장 PC 기준 (내장 그래픽) |
| Storage SMART 드라이브 | 1개 | SSD 1대 |
| SMART 속성 수 | 6개 | temperature, remaining_life, media_errors, power_cycles, unsafe_shutdowns, power_on_hours |

---

## 수집기별 EARS Row 수 (= ES Document 수)

각 수집 주기(cycle)마다 생성되는 EARS Row가 1:1로 Kafka record → ES document에 대응됩니다.

### 30초 주기 수집기 (2 cycles/min)

| Collector | Rows/Cycle | 산출 근거 | Rows/Min | Rows/Hour |
|-----------|------------|-----------|----------|-----------|
| **cpu** | 1 | `total_used_pct` 1개 | 2 | 120 |
| **memory** | 3 | `total_used_pct` + `total_free_pct` + `total_used_size` | 6 | 360 |
| **network** | 4 | `all_inbound` + `all_outbound` + NIC×(`recv_rate` + `sent_rate`) = 2 + 1×2 | 8 | 480 |
| **소계** | **8** | | **16** | **960** |

### 60초 주기 수집기 (1 cycle/min)

| Collector | Rows/Cycle | 산출 근거 | Rows/Min | Rows/Hour |
|-----------|------------|-----------|----------|-----------|
| **disk** | 2 | 파티션 수 (C:, D:) | 2 | 120 |
| **temperature** | 3 | 센서 수 (CPU Package, Core#0, Core#1) | 3 | 180 |
| **cpu_process** | 10 | TopN=10 프로세스 | 10 | 600 |
| **memory_process** | 10 | TopN=10 프로세스 | 10 | 600 |
| **fan** | 2 | 센서 수 (CPU Fan, System Fan) | 2 | 120 |
| **gpu** | 0 | GPU 없음 (내장 그래픽) | 0 | 0 |
| **voltage** | 3 | 센서 수 (CPU Core, +3.3V, +5V) | 3 | 180 |
| **motherboard_temp** | 2 | 센서 수 (System, Auxiliary) | 2 | 120 |
| **소계** | **32** | | **32** | **1,920** |

### 300초 주기 수집기 (0.2 cycles/min)

| Collector | Rows/Cycle | 산출 근거 | Rows/Min | Rows/Hour |
|-----------|------------|-----------|----------|-----------|
| **storage_smart** | 6 | 1드라이브 × 6속성 | 1.2 | 72 |
| **소계** | **6** | | **1.2** | **72** |

### 합계

| | Rows/Min | Rows/Hour | Rows/Day (24h) | Rows/Month (30d) |
|--|----------|-----------|----------------|------------------|
| **1 PC 합계** | **~49** | **2,952** | **70,848** | **2,125,440** |

---

## 수집기별 Document 크기

### EARS Row raw 필드 크기

Grok 파서용 plain text 형식: `{timestamp} category:{c},pid:{p},proc:{n},metric:{m},value:{v}`

| Collector | raw 예시 | raw 크기 |
|-----------|----------|----------|
| cpu | `2026-02-26 10:30:45,000 category:cpu,pid:0,proc:@system,metric:total_used_pct,value:45.5` | ~87 B |
| memory | `...category:memory,...,metric:total_used_size,value:12000000000` | ~90-98 B |
| disk | `...category:disk,...,metric:C:,value:60.0` | ~79 B |
| network | `...category:network,...,proc:Ethernet,...,metric:recv_rate,value:1234.5` | ~85-88 B |
| cpu_process | `...category:cpu,pid:1234,proc:chrome.exe,metric:used_pct,value:5.5` | ~86 B |
| memory_process | `...category:memory,pid:1234,proc:chrome.exe,metric:used,value:500000000` | ~90 B |
| temperature | `...category:temperature,...,metric:CPU Package,value:55.0` | ~92 B |
| fan | `...category:fan,...,metric:CPU Fan,value:1200` | ~82 B |
| voltage | `...category:voltage,...,metric:CPU Core,value:1.2` | ~84 B |
| motherboard_temp | `...category:motherboard_temp,...,metric:System,value:35.0` | ~92 B |
| storage_smart | `...category:storage_smart,...,metric:Samsung SSD 860_temperature,value:35` | ~105 B |

### KafkaValue2 구조 (Kafka record = ES _source)

```json
{
  "process": "PHOTO",
  "line": "A1",
  "eqpid": "EQP-PHOTO-001",
  "model": "ASML",
  "diff": 0,
  "esid": "PHOTO:EQP-PHOTO-001-1708934445000-0",
  "raw": "{위 plain text}"
}
```

| 구성 요소 | 크기 | 비고 |
|-----------|------|------|
| envelope (process~esid) | ~130 B | eqpid 길이에 따라 변동 |
| raw 필드 | ~80-105 B | 수집기별 상이 |
| **Kafka record 합계** | **~210-235 B** | JSON 직렬화 기준 |

### ES 저장 크기 (document당)

| 구성 요소 | 크기 | 비고 |
|-----------|------|------|
| _source (원본 JSON) | ~220 B | Kafka record 그대로 |
| Inverted index | ~100-150 B | 문자열 필드 인덱싱 |
| Doc values | ~50-80 B | 숫자/날짜 필드 columnar |
| _id, metadata | ~50 B | ES 내부 |
| **ES document 합계** | **~400-500 B** | |

---

## 수집기별 일일 데이터량 (1 PC)

| Collector | Interval | Rows/Day | Doc 크기 | **일일 용량** | 비중 |
|-----------|----------|----------|----------|--------------|------|
| cpu | 30s | 2,880 | ~420 B | **1.2 MB** | 4.1% |
| memory | 30s | 8,640 | ~430 B | **3.5 MB** | 12.2% |
| network | 30s | 11,520 | ~425 B | **4.7 MB** | 16.3% |
| disk | 60s | 2,880 | ~410 B | **1.1 MB** | 4.1% |
| temperature | 60s | 4,320 | ~430 B | **1.8 MB** | 6.1% |
| cpu_process | 60s | 14,400 | ~420 B | **5.8 MB** | 20.3% |
| memory_process | 60s | 14,400 | ~430 B | **5.9 MB** | 20.3% |
| fan | 60s | 2,880 | ~415 B | **1.1 MB** | 4.1% |
| gpu | 60s | 0 | - | **0 MB** | 0% |
| voltage | 60s | 4,320 | ~420 B | **1.7 MB** | 6.1% |
| motherboard_temp | 60s | 2,880 | ~430 B | **1.2 MB** | 4.1% |
| storage_smart | 300s | 1,728 | ~440 B | **0.7 MB** | 2.4% |
| **합계** | | **70,848** | **평균 ~425 B** | **~29 MB** | **100%** |

### 비중 분석

```
cpu_process     ████████████████████  20.3%   ← TopN=10, 60초
memory_process  ████████████████████  20.3%   ← TopN=10, 60초
network         ████████████████▎     16.3%   ← 30초 + 4 rows/cycle
memory          ████████████▏         12.2%   ← 30초 + 3 rows/cycle
────────────────────────────────────────────
상위 4개 합계                          69.1%
```

> **핵심**: cpu_process + memory_process (TopN=10, 60초)가 전체의 ~41%를 차지합니다. 추후 TopN 축소 시 추가 절감 가능합니다.

---

## 기간별 데이터량 (1 PC)

| 기간 | Document 수 | 저장 용량 |
|------|-------------|-----------|
| 1분 | ~49 | ~20 KB |
| 1시간 | 2,952 | ~1.2 MB |
| **1일 (24h)** | **70,848** | **~29 MB** |
| 1주 (7d) | 495,936 | ~200 MB |
| **1개월 (30d)** | **2,125,440** | **~860 MB** |
| 1년 (365d) | 25,859,520 | ~10.2 GB |

---

## 대규모 배포 시 데이터량

### PC 대수별 일일 데이터량

| PC 대수 | 일일 Document 수 | 일일 용량 | 월간 용량 |
|---------|-----------------|-----------|-----------|
| 100대 | 7,084,800 | ~2.8 GB | ~84 GB |
| 1,000대 | 70,848,000 | ~28 GB | ~840 GB |
| 5,000대 | 354,240,000 | ~140 GB | ~4.2 TB |
| **10,000대** | **708,480,000** | **~280 GB** | **~8.4 TB** |

### 10,000대 기준 Elasticsearch 클러스터 용량 산정

| 항목 | 산정 | 결과 |
|------|------|------|
| 일일 raw 데이터 | 10,000 × 29 MB | **~280 GB** |
| Replica 포함 (1 replica) | 280 GB × 2 | **~560 GB** |
| 보존 기간 7일 | 560 GB × 7 | **~3.9 TB** |
| 보존 기간 30일 | 560 GB × 30 | **~16.8 TB** |
| 운영 여유분 (+20%) | × 1.2 | |
| **7일 보존 권장 디스크** | | **~4.7 TB** |
| **30일 보존 권장 디스크** | | **~20 TB** |

### 초당 인덱싱 속도

| PC 대수 | Docs/Min | **Docs/Sec** | 비고 |
|---------|----------|-------------|------|
| 1,000대 | 49,200 | ~820 | 단일 노드 가능 |
| 5,000대 | 246,000 | ~4,100 | 3 노드 권장 |
| 10,000대 | 492,000 | **~8,200** | 3+ 노드 권장 |

---

## 하드웨어 변동에 따른 차이

가정 하드웨어 구성은 PC마다 다를 수 있습니다. 주요 변동 요인:

| 변동 항목 | 기준 | 최소 | 최대 | Rows/Day 변화 |
|-----------|------|------|------|---------------|
| 디스크 파티션 수 | 2 | 1 | 4 | -1,440 ~ +2,880 |
| NIC 수 | 1 | 1 | 3 | 0 ~ +5,760 |
| CPU 온도 센서 수 | 3 | 1 | 8 | -2,880 ~ +7,200 |
| Fan 센서 수 | 2 | 0 | 5 | -2,880 ~ +4,320 |
| Voltage 센서 수 | 3 | 2 | 8 | -1,440 ~ +7,200 |
| Motherboard 센서 수 | 2 | 1 | 5 | -1,440 ~ +4,320 |
| GPU 유무 | 없음 | 0 | 1 (7메트릭) | 0 ~ +10,080 |
| SMART 드라이브 수 | 1 | 1 | 3 | 0 ~ +3,456 |
| SMART 속성 수/드라이브 | 6 | 3 | 7 | -864 ~ +288 |

### GPU 장착 PC의 추가 데이터량

GPU 1대 (7개 메트릭: temperature, core_load, memory_load, fan_speed, power, core_clock, memory_clock):

| | GPU 미장착 | GPU 1대 장착 | 차이 |
|--|-----------|-------------|------|
| Rows/Day | 70,848 | 80,928 | +10,080 (+14.2%) |
| 일일 용량 | ~29 MB | ~33 MB | +4 MB |

---

## 추가 절감 방안

현재 설정에서 추가 절감이 필요한 경우:

### 방안 1: Process TopN 축소 (10 → 5)

| | TopN=10 | TopN=5 | 절감 |
|--|---------|--------|------|
| cpu_process Rows/Day | 14,400 | 7,200 | -7,200 |
| memory_process Rows/Day | 14,400 | 7,200 | -7,200 |
| **합계 절감** | | | **-14,400 (-20.3%)** |
| 일일 용량 | ~29 MB | ~23 MB | **-6 MB** |

### 방안 2: 전체 주기 2배 (30s→60s, 60s→120s, 300s→600s)

| | 기본 | 2배 | 절감 |
|--|------|-----|------|
| Rows/Day | 70,848 | 35,424 | **-35,424 (-50%)** |
| 일일 용량 | ~29 MB | ~14.5 MB | **-14.5 MB** |

### 방안 3: LhmHelper 미사용 (센서 수집 비활성화)

temperature, fan, gpu, voltage, motherboard_temp, storage_smart 비활성화 시:

| | 전체 | gopsutil만 | 절감 |
|--|------|-----------|------|
| 활성 Collector | 12개 | 6개 | -6개 |
| Rows/Day | 70,848 | 54,720 | **-16,128 (-22.8%)** |
| 일일 용량 | ~29 MB | ~22 MB | **-7 MB** |

---

## 요약

### 1 PC 기준 (현재 설정)

| | 값 |
|--|---|
| **일일 Document 수** | **~71,000** |
| **일일 저장 용량** | **~29 MB** |
| **월간 저장 용량** | **~860 MB** |

### 10,000대 기준

| | 값 |
|--|---|
| **일일 Document 수** | **~7.1억** |
| **일일 저장 용량** | **~280 GB** |
| **초당 인덱싱** | **~8,200 docs/sec** |
| **7일 보존 권장 디스크 (replica 포함)** | **~4.7 TB** |
| **30일 보존 권장 디스크 (replica 포함)** | **~20 TB** |
