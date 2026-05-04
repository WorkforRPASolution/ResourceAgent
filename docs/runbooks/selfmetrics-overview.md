# SelfMetrics 운영 가이드 (Phase 2.5-1)

ResourceAgent가 자기 자신의 runtime 상태를 1분마다 emit하는 메트릭 모음입니다. 일반 collector(CPU/Memory/Disk 등)와 같은 sender pipeline(Kafka/KafkaRest/File)을 통해 흘러갑니다.

## 6개 지표

EARS row category는 **`agent`** 입니다. proc는 `@system`, pid=0.

| metric | 단위 | 의미 | 정상 범위 (참고) | 이상 신호 |
|--------|------|------|-----------------|----------|
| `goroutine_count` | count | `runtime.NumGoroutine()` | 20~50 | 시간 따라 단조 증가 → goroutine leak |
| `rss_bytes` | bytes | OS-level Resident Set Size | 25~50MB | 단조 증가 → 메모리 누수 |
| `heap_alloc_bytes` | bytes | Go heap 활성 객체 (`runtime.MemStats.Alloc`) | 변동 | 평소 대비 ×3 이상 + GC 후도 안 줄어듬 |
| `heap_sys_bytes` | bytes | Go runtime이 OS로부터 받은 총 heap (`MemStats.Sys`) | RSS와 비슷한 추세 | RSS와 격차 크면 OS-level 누수 가능 |
| `buffer_count` | records | BufferedHTTPTransport 현재 buffer (Phase 2-1) | 평소 0~수십 | `MaxBufferedRecords` (기본 10,000) 근처 → KafkaRest unreachable |
| `buffer_dropped_total` | records | 프로세스 lifetime 누적 drop | 0 | 시간 따라 빠르게 증가 → KafkaRest 단절 + drop 진행 중 |

## 출력 예시

`Format=grok` (file sender 또는 KafkaRest pre-format) 기준:

```
2026-05-04 14:00:00,123 category:agent,pid:0,proc:@system,metric:goroutine_count,value:42
2026-05-04 14:00:00,123 category:agent,pid:0,proc:@system,metric:rss_bytes,value:31457280
2026-05-04 14:00:00,123 category:agent,pid:0,proc:@system,metric:heap_alloc_bytes,value:1048576
2026-05-04 14:00:00,123 category:agent,pid:0,proc:@system,metric:heap_sys_bytes,value:8388608
2026-05-04 14:00:00,123 category:agent,pid:0,proc:@system,metric:buffer_count,value:0
2026-05-04 14:00:00,123 category:agent,pid:0,proc:@system,metric:buffer_dropped_total,value:0
```

JSON (kafka sender) 기준 1줄:

```json
{
  "iso_timestamp": "2026-05-04T14:00:00.123Z",
  "parsed": [
    {"field": "EARS_PROCESS", "value": "<process>", "dataformat": "String"},
    {"field": "EARS_CATEGORY", "value": "agent", "dataformat": "String"},
    {"field": "EARS_PID", "value": "0", "dataformat": "Integer"},
    {"field": "EARS_PROCNAME", "value": "@system", "dataformat": "String"},
    {"field": "EARS_METRIC", "value": "goroutine_count", "dataformat": "String"},
    {"field": "EARS_VALUE", "value": "42", "dataformat": "Double"}
  ]
}
```

## 운영 체크리스트

### 정상 확인

```bash
# 최근 1분 내 SelfMetrics 6 records 정상 emit?
grep "metric:goroutine_count" log/ResourceAgent/metrics.log | tail -5
```

기대: 60s 간격으로 정확히 1줄씩.

### Q1. "goroutine_count가 계속 증가하는가?"

```bash
grep "metric:goroutine_count" log/ResourceAgent/metrics.log | awk -F'value:' '{print $2}' | tail -60
```

- 안정 (±5 진동) → 정상
- 1시간에 +10 이상 단조 증가 → goroutine leak. C1 (LhmProvider) / C2 (WMI Query) 회귀 가능성. `lhm-provider-timeout-monitoring.md`, `wmi-query-monitoring.md` 점검

### Q2. "RSS가 계속 증가하는가?"

```bash
grep "metric:rss_bytes" log/ResourceAgent/metrics.log | awk -F'value:' '{print $2}' | tail -60
```

- 안정 → 정상
- 단조 증가 → Phase A SLO-1 Paged Pool 증가와 연관 가능 (커널 leak). EPS 화이트리스트 등록 여부 확인 (`project_win7_eps_paged_pool_root_cause` 메모)

### Q3. "buffer가 차고 있는가?"

```bash
grep "metric:buffer_count" log/ResourceAgent/metrics.log | awk -F'value:' '{print $2}' | tail -10
```

- 평소 0~수십 → 정상
- `MaxBufferedRecords` (기본 10,000) 근처 → KafkaRest 단절 임박 또는 발생 중. `buffered-http-transport-monitoring.md` 참조
- 동시에 `BUFFER_DROP_OLDEST` 로그 발생하면 이미 drop 중

### Q4. "drop이 빠르게 누적되는가?"

```bash
grep "metric:buffer_dropped_total" log/ResourceAgent/metrics.log | awk -F'value:' '{print $2}' | tail -10
```

- 0 유지 → 정상
- 시간 따라 빠르게 증가 → KafkaRest 회복 안 됨. endpoint 가용성 점검

## 비활성화 (긴급 시)

`Monitor.json`:
```json
"SelfMetrics": {
  "Enabled": false
}
```

Hot reload — 즉시 emit 중단. 다른 collector 영향 없음.

## 알려진 한계

- **자기 교란 방어 없음**: drop 발생 시 SelfMetrics 자체도 같은 buffer로 흐르므로 같이 drop될 수 있음. drop 신호는 별도로 `BUFFER_DROP_OLDEST` 로그(BasicSampler{N:10})에 기록되므로 보완 가능.
- **paged_pool_bytes 미포함**: 커널 Paged Pool은 별도 phase (2.5-1.5, pdh.dll syscall 필요). 현재는 외부 RAMMap/typeperf로 측정.
- **handle_count 미포함**: Windows handle 추적은 별도 phase (2.5-1.6).
- **goroutine baseline drift 미계산**: 절대값만 emit. 60s baseline 대비 ±10 SLO 판정은 외부(grafana 등)에서 수행.

## 관련 문서

- `docs/plans/memory-leak-mitigation-plan.md` §Phase 2.5
- `docs/runbooks/buffered-http-transport-monitoring.md` (Phase 2-1)
- `docs/runbooks/lhm-provider-timeout-monitoring.md` (Phase 1-1)
- `docs/runbooks/wmi-query-monitoring.md` (Phase 1-2)
