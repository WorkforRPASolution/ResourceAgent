# Coverage Baseline

- 측정일: 2026-04-26
- Go: go1.25.6
- 명령: `go test -coverprofile=c.out -covermode=atomic ./...`
- **Overall: 58.5%**

## Phase별 목표
- Phase 2 완료 후: baseline + 2%p
- Phase 3 완료 후: baseline + 5%p
- 절대 하한: 70%

## 패키지별 합계 (cover/statements 비율)

| 패키지 | Statements | Covered | % |
|--------|-----------|---------|---|
| resourceagent/cmd/resourceagent | 12 functions | - | 8.3% (function avg) |
| resourceagent/internal/collector | 123 functions | - | 46.4% (function avg) |
| resourceagent/internal/config | 33 functions | - | 64.6% (function avg) |
| resourceagent/internal/discovery | 14 functions | - | 73.3% (function avg) |
| resourceagent/internal/eqpinfo | 4 functions | - | 82.6% (function avg) |
| resourceagent/internal/heartbeat | 10 functions | - | 92.5% (function avg) |
| resourceagent/internal/logger | 19 functions | - | 55.5% (function avg) |
| resourceagent/internal/metainfo | 2 functions | - | 85.7% (function avg) |
| resourceagent/internal/network | 7 functions | - | 82.8% (function avg) |
| resourceagent/internal/scheduler | 8 functions | - | 92.3% (function avg) |
| resourceagent/internal/sender | 68 functions | - | 78.9% (function avg) |
| resourceagent/internal/service | 6 functions | - | 14.6% (function avg) |
| resourceagent/internal/timediff | 7 functions | - | 89.9% (function avg) |

## 미커버 핫스팟 (보강 우선순위)

```
resourceagent/cmd/resourceagent/main.go:34:			main				0.0%
resourceagent/cmd/resourceagent/main.go:140:			setupInfrastructure		0.0%
resourceagent/cmd/resourceagent/main.go:194:			setupWithProxy			0.0%
resourceagent/cmd/resourceagent/main.go:271:			setupWithoutProxy		0.0%
resourceagent/cmd/resourceagent/main.go:329:			applyEqpInfo			0.0%
resourceagent/cmd/resourceagent/main.go:351:			startTimeDiffSyncer		0.0%
resourceagent/cmd/resourceagent/main.go:365:			buildCandidatesProxy		0.0%
resourceagent/cmd/resourceagent/main.go:400:			setupCollectors			0.0%
resourceagent/cmd/resourceagent/main.go:407:			setupSender			0.0%
resourceagent/cmd/resourceagent/main.go:442:			setupWatchers			0.0%
resourceagent/cmd/resourceagent/main.go:525:			run				0.0%
resourceagent/internal/collector/cpu.go:19:			NewCPUCollector			0.0%
resourceagent/internal/collector/cpu.go:26:			Configure			0.0%
resourceagent/internal/collector/cpu.go:35:			Collect				0.0%
resourceagent/internal/collector/cpu_process.go:58:		DefaultConfig			0.0%
resourceagent/internal/collector/disk.go:20:			NewDiskCollector		0.0%
resourceagent/internal/collector/disk.go:27:			DefaultConfig			0.0%
resourceagent/internal/collector/disk.go:34:			Configure			0.0%
resourceagent/internal/collector/disk.go:44:			Collect				0.0%
resourceagent/internal/collector/disk.go:113:			shouldInclude			0.0%
resourceagent/internal/collector/disk.go:144:			getDeviceName			0.0%
resourceagent/internal/collector/fan.go:25:			DefaultConfig			0.0%
resourceagent/internal/collector/fan.go:43:			Collect				0.0%
resourceagent/internal/collector/fan_unix.go:16:		collectFanSpeeds		0.0%
resourceagent/internal/collector/fan_unix.go:91:		getHwmonDeviceName		0.0%
resourceagent/internal/collector/fan_unix.go:102:		getFanLabel			0.0%
resourceagent/internal/collector/gpu.go:25:			DefaultConfig			0.0%
resourceagent/internal/collector/gpu.go:43:			Collect				0.0%
resourceagent/internal/collector/gpu_unix.go:11:		collectGpuMetrics		0.0%
resourceagent/internal/collector/lhm_provider_unix.go:11:	Start				0.0%
```

## 비고

- Windows-only 코드(`*_windows.go`)는 macOS 측정에서 제외됨. Windows VM에서 별도 측정 시 `coverage-baseline-windows.txt` 로 저장 권장.
- 통합 테스트(`-tags=integration`) 미포함.

전체 함수별 상세는 `coverage-baseline.txt` 참조.
