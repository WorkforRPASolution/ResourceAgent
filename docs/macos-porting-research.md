# ResourceAgent macOS 포팅 검토서

> 작성일: 2026-02-27 | 팀 검토 반영: 2026-02-28
> 상태: 리서치 완료, 실행 대기
> 대상: MacBook / Mac Mini (Apple Silicon M1-M4)

---

## 1. 배경

ResourceAgent는 공장 PC 10,000대에서 실행되는 경량 모니터링 에이전트로,
현재 **Windows + Linux**만 지원한다.
Windows에서 하드웨어 센서(온도/팬/전압/GPU/SMART)는 **LhmHelper**(LibreHardwareMonitor 기반 C# 헬퍼)로 수집한다.

macOS에는 LibreHardwareMonitor가 없으므로, 동등한 데이터를 수집하는 대안이 필요하다.

---

## 2. macOS 센서 데이터 소스

### 2.1 SMC (System Management Controller)

모든 Mac에 탑재된 하드웨어 관리 칩. IOKit의 `AppleSMC` 서비스를 통해 접근.

- **접근 방식**: `IOServiceOpen()` + `IOConnectCallStructMethod()` (4글자 FourCC 키)
- **sudo 불필요** (읽기 전용)
- **데이터 포맷**: Intel Mac = `sp78` (signed fixed-point 7.8), Apple Silicon = IEEE 754 float

| 키 패턴 | 센서 종류 | 예시 키 |
|---------|----------|---------|
| `TC*` | CPU 온도 | `TC0P` (proximity), `TC0D` (die), `TCxC` (core x) |
| `TG*` | GPU 온도 | `TG0D` (die), `TG0P` (proximity) |
| `TM*` | 메모리 온도 | `TM0P` |
| `TA*` | 주변/보드 온도 | `TA0P` (ambient) |
| `TP*` | PCH 온도 | `TP0P` |
| `F*Ac` | 팬 실제 RPM | `F0Ac` (팬 0), `F1Ac` (팬 1) |
| `F*Tg` | 팬 목표 RPM | `F0Tg` (설정에 root 필요) |
| `VC*` | CPU 전압 | `VC0C` (core) |
| `VG*` | GPU 전압 | `VG0C` |
| `IC*` | CPU 전류 | `IC0R`, `IC0C` |
| `PCPC` | CPU 패키지 전력 | |
| `PCPG` | GPU 패키지 전력 | |
| `PSTR` | 시스템 총 전력 | |

### 2.2 IOHIDEventSystemClient (Apple Silicon 온도)

M1 이후 Apple Silicon에서 온도 센서는 HID 센서 허브로도 노출된다.

- **API**: `IOHIDEventSystemClientCreate()` (private API, 공식 헤더 없음)
- **센서 이름**: `eACC*` (효율 코어), `pACC*` (성능 코어), `PMU tdev*` (패키지)
- **sudo 불필요**
- **macOS 14+**: SMC로도 다시 노출됨 (두 경로 모두 사용 가능)

### 2.3 IOReport (GPU/전력 메트릭 — Apple Silicon 전용)

Apple private 프레임워크. CPU/GPU 전력, 주파수, 활용률 데이터를 **sudo 없이** 제공.

- **Subscription 기반**: 채널 구독 -> 샘플 생성 -> 델타 계산
- **주요 채널**:
  - `"Energy Model"` — 전력 소비 (CPU, GPU, ANE, DRAM)
  - `"CPU Stats"` / `"CPU Core Performance States"` — CPU 주파수/활용률
  - `"GPU Stats"` / `"GPU Performance States"` — GPU 주파수/활용률

### 2.4 powermetrics (Apple CLI 도구)

```bash
sudo powermetrics --samplers cpu_power,gpu_power,thermal -n 1
```

- **sudo 필수** — 에이전트 용도로는 부적합
- Apple Silicon에서 `smc` sampler 미지원
- 텍스트 출력 파싱 필요

### 2.5 smartctl (SMART 데이터)

```bash
brew install smartmontools
sudo smartctl -a /dev/disk0 -j   # JSON 출력
```

- **sudo 필수**
- 온도, 전원 켜진 시간, 전원 사이클, 안전하지 않은 종료, 미디어 오류, 잔여 수명 등
- Apple 내장 NVMe SSD: 일부 비표준 속성

---

## 3. Intel Mac vs Apple Silicon 차이

| 항목 | Intel Mac | Apple Silicon (M1-M4) |
|------|-----------|----------------------|
| 온도 API | SMC (`sp78` 고정소수점) | IOHIDEventSystem + SMC (macOS 14+, float) |
| 팬 속도 | SMC `F*Ac` | SMC `F*Ac` (float 포맷) |
| CPU 전력 | SMC `PCPC` | IOReport "Energy Model" (sudo 불필요) |
| GPU 전력 | SMC `PCPG` | IOReport "Energy Model" (sudo 불필요) |
| GPU 주파수 | 제한적 | IOReport per-cluster DVFS |
| GPU 메모리 로드 | 디스크릿 GPU일 때 가능 | **N/A** (통합 메모리 아키텍처) |
| 전압 | SMC `VC*`/`VG*` (다양) | SMC (제한된 키만 노출) |

### 팬 유무 참고

| 모델 | 팬 |
|------|---|
| MacBook Air M1-M3 | **없음** (팬리스) |
| MacBook Pro M1-M4 | 있음 |
| Mac Mini M1-M4 | 1개 |
| Mac Studio | 여러 개 |
| Mac Pro | 여러 개 |

---

## 4. Windows LhmHelper vs macOS 달성 가능 비교

| LhmHelper 메트릭 | Windows | macOS | macOS 방법 | Root |
|------------------|:---:|:---:|------------|:---:|
| CPU 온도 | O | **O** | SMC / IOHIDEvent | No |
| CPU 코어별 온도 | O | **O** | SMC `TCxC` / IOHIDEvent `eACC`/`pACC` | No |
| GPU 온도 | O | **△** | SMC `TG*` (모델 의존적) | No |
| GPU Core Load % | O | **O** (AS) | IOReport "GPU Stats" | No |
| GPU Memory Load % | O | **X** | 통합 메모리 — 개념 자체 해당 없음 | — |
| GPU Fan Speed RPM | O | **X** | 내장 GPU — 별도 팬 없음 | — |
| GPU 전력 (W) | O | **O** (AS) | IOReport "Energy Model" | No |
| GPU Core Clock MHz | O | **O** (AS) | IOReport GPU Performance States | No |
| GPU Memory Clock MHz | O | **X** | 통합 메모리 — N/A | — |
| 시스템 팬 RPM | O | **O** | SMC `F*Ac` | No |
| CPU 전압 | O | **△** | SMC `VC0C` (Intel 양호, AS 제한) | No |
| GPU 전압 | O | **△** | SMC `VG0C` (Intel 양호, AS 제한) | No |
| 마더보드 온도 | O | **△** | SMC `TP0P`, `TA0P` (모델 의존) | No |
| SMART 온도 | O | **O** | smartctl | **Yes** |
| SMART 잔여 수명 | O | **O** | smartctl | **Yes** |
| SMART 미디어 오류 | O | **O** | smartctl | **Yes** |
| SMART 전원 사이클 | O | **O** | smartctl | **Yes** |
| CPU 전력 (W) | — | **O** (AS) | IOReport (macOS 추가 메트릭) | No |
| ANE 전력 (W) | — | **O** (AS) | IOReport (macOS 추가 메트릭) | No |
| DRAM 전력 (W) | — | **O** (AS) | IOReport (macOS 추가 메트릭) | No |

**커버리지**: Windows 대비 **약 80%** (GPU 메모리 관련 메트릭만 N/A)

---

## 5. Go 라이브러리/도구 평가

### 5.1 iSMC (Go 라이브러리)

- **Repo**: https://github.com/dkorunic/iSMC
- **언어**: Go 83.8% + C 14.2%
- **기능**: 온도, 팬, 배터리, 전력, 전압, 전류
- **패키지 구조**: `smc/` (core), `hid/` (Apple Silicon), `gosmc/` (Go 바인딩)
- **라이브러리로 사용 가능**: `import "github.com/dkorunic/iSMC/smc"` 가능

#### Apple Silicon 세대별 호환성 (팀 검토 결과)

| Generation | 상태 | 상세 |
|---|---|---|
| M1 | Good | HID 센서 허브 지원 |
| M2 | **Partial** | Mac14,2 등 CPU core 키 누락 |
| M3 | **Partial** | Mac15,6/15,7 키 누락 |
| M4/M4 Pro | 키 정의됨, 미검증 | E-core(`Te05` 등), P-core(`Tf04`~`Tf4E` 12코어), GPU(`Tf14`~`Tf2A` 8구성) |

**Issue #23 상태**: Closed이지만, 모델별 센서 키 갭 지속.
메인테이너 평가: "SMC 키 스펙이 비공개라 키는 감지 가능하나, 어떤 센서에 매핑되는지 알 수 없음."
최신 릴리스 v0.11.1 (2025-02-16): 의존성 업데이트만, M4 전용 개선 없음.

#### Apple Silicon 온도 버킷 문제 (핵심 발견)

Apple Silicon(M1~M4)은 정밀한 섭씨 온도가 아닌 **열 압력 버킷**을 보고:
- 버킷: **Nominal / Fair / Serious / Critical**
- Intel Mac의 `TC0P` 정밀 온도와는 근본적으로 다름
- `powermetrics`도 Apple Silicon에서는 thermal pressure level만 보고
- **설계 결정 필요**: 버킷 기반 온도를 EARS 포맷에 어떻게 매핑할지

### 5.2 mactop (참조 구현 — 권장)

- **Repo**: https://github.com/metaspartan/mactop
- **언어**: Go + CGO (Objective-C 브릿지)
- **기능**: CPU/GPU 사용률, 전력, 온도, 팬, 주파수, 메모리, 네트워크, 디스크
- **API 사용**: SMC + IOReport + IOKit + IOHIDEventSystemClient
- **sudo 불필요**
- **M4 Pro 지원 확인됨**: `brew install mactop`으로 즉시 검증 가능
- 2025-12-31: gopsutil 의존 제거, macOS 네이티브 API 직접 사용으로 전환
- **IOReport Objective-C 브릿지 코드의 최적 참조**

### 5.3 추천 전략: mactop 하이브리드 (Option B+D)

| 옵션 | 설명 |
|------|------|
| Option A: iSMC 단독 | 전력 소비 메트릭 부재 (IOReport 필요) |
| Option B: mactop 참조 | 검증됨, 활발히 유지보수, M4 호환, 하이브리드 API |
| Option C: 처음부터 작성 | CGO 복잡도 높음; mactop이 이미 해결 |
| Option D: 하이브리드 fallback | iSMC 우선, 불완전 시 IOReport 대체 |
| **권장: B+D** | **mactop을 참조 + 클린 추상화, iSMC를 보조로 사용** |

즉시 M4 Pro 검증 명령:
```bash
brew install mactop && mactop          # IOReport 기반 전력/주파수
CGO_ENABLED=1 go install github.com/dkorunic/iSMC@latest
iSMC temp && iSMC power && iSMC fans  # SMC 기반 온도/전력/팬
```

### 5.4 기타 참고 도구

| 도구 | 언어 | Sudo | Apple Silicon | 용도 |
|------|------|:---:|:---:|------|
| [macmon](https://github.com/vladkens/macmon) | Rust | No | M1-M4 | CPU/GPU 전력/온도/주파수 (M4 주파수 버그 존재) |
| [socpowerbud](https://github.com/dehydratedpotato/socpowerbud) | Obj-C | No | Yes | CPU/GPU 주파수/전압/DVFS |
| [apple_sensors](https://github.com/fermion-star/apple_sensors) | Obj-C | No | M1+ | IOHIDEventSystem 온도 참조 |
| [Stats](https://github.com/exelban/stats) | Swift | No | Yes | macOS 메뉴바 시스템 모니터 (커뮤니티 M3 키 발견) |
| [test-ioreport](https://github.com/freedomtan/test-ioreport) | Obj-C | No | Yes | IOReport 참조 구현 |
| [gosmc](https://github.com/charlie0129/gosmc) | Go | No | Yes | Go SMC 라이브러리 |

### 5.5 gopsutil 센서 — macOS에서 사용 금지

- Apple Silicon에서 **온도 전부 0 반환** 또는 **SIGBUS/SIGSEGV 크래시**
- 이슈: https://github.com/shirou/gopsutil/issues/1832
- 현재 `temperature_unix.go`의 `host.SensorsTemperaturesWithContext()`는 Apple Silicon에서 작동 안 함
- **Linux에서만 사용, macOS에서는 반드시 대체 필요**

---

## 6. 권한 요구사항 정리

| 작업 | Root 필요 | SIP 관련 | 비고 |
|------|:---:|:---:|------|
| SMC 온도 읽기 | No | No | |
| SMC 팬 RPM 읽기 | No | No | |
| IOHIDEventSystem 센서 읽기 | No | No | |
| IOReport 전력/주파수 읽기 | No | No | |
| 팬 속도 설정 | **Yes** | No | privileged helper + Developer ID 필요 |
| powermetrics 실행 | **Yes** | No | |
| smartctl 실행 | **Yes** | No | |
| kext/드라이버 접근 | **Yes** | **Yes** | Apple 서명 필요 |

---

## 7. 현재 코드 macOS 호환성 분석

> 팀 검토 핵심 발견: ResourceAgent는 **코드 변경 없이** macOS에서 빌드·실행된다.

### 7.1 무변경 빌드 결과

```bash
go build ./cmd/resourceagent  # → 15MB arm64 Mach-O 바이너리 (M4 Pro)
```

- `SenderType: "file"` 설정 시 Redis/Kafka 없이 단독 실행 가능
- **14개 collector 중 8개가 macOS에서 정상 동작** (CPU, Memory, Disk, Network, CPUProcess, MemoryProcess, Uptime, ProcessWatch)
- 나머지 6개는 빈 데이터를 graceful하게 반환 (크래시 없음)
- 기존 단위 테스트 20+ 전부 PASS (M4 Pro)

**패러다임 전환**: ~~"macOS에서 동작하게 만들기"~~ → **"이미 동작함; 빈 센서 데이터만 채우기"**

### 7.2 파일별 상세 분석

| 파일 | 빌드 태그 | macOS 컴파일 | macOS 런타임 | 분리 필요? |
|------|----------|:---:|------------|----------|
| `temperature_unix.go` (42줄) | `linux \|\| darwin` | Yes | 빈 데이터 (gopsutil이 nil 반환, 크래시 없음) | 개선 시에만 |
| `fan_unix.go` (120줄) | `linux \|\| darwin` | Yes | `runtime.GOOS == "darwin"` 체크로 `nil, nil` 반환 (line 18-20) | 개선 시에만 |
| `gpu_unix.go` (15줄) | `linux \|\| darwin` | Yes | Stub: `return nil, nil` 무조건 반환 | 불필요 |
| `voltage_unix.go` (15줄) | `linux \|\| darwin` | Yes | Stub: `return nil, nil` 무조건 반환 | 불필요 |
| `motherboard_temp_unix.go` (15줄) | `linux \|\| darwin` | Yes | Stub: `return nil, nil` 무조건 반환 | 불필요 |
| `storage_smart_unix.go` (15줄) | `linux \|\| darwin` | Yes | Stub: `return nil, nil` 무조건 반환 | 불필요 |
| `lhm_provider_unix.go` (109줄) | `linux \|\| darwin` | Yes | 모든 메서드 no-op, `&LhmData{}` 반환 | 불필요 |

### 7.3 주요 코드 참조

`temperature_unix.go`:
```go
// Lines 14-18: gopsutil 호출
temps, err := host.SensorsTemperaturesWithContext(ctx)
// macOS Apple Silicon에서 빈 배열 반환 (gopsutil issue #1832)
```

`fan_unix.go`:
```go
// Lines 16-20: 이미 Darwin 체크 존재
if runtime.GOOS == "darwin" {
    return nil, nil
}
// 이후 코드는 Linux sysfs 가정
```

Stub 파일들 (`gpu_unix.go`, `voltage_unix.go`, `motherboard_temp_unix.go`, `storage_smart_unix.go`):
```go
func (c *XxxCollector) collectXxx(ctx context.Context) ([]XxxSensor, error) {
    return nil, nil  // 모든 플랫폼에서 동일
}
```

### 7.4 registry.go — 14개 collector 등록 현황

```go
func DefaultRegistry() *Registry {
    r := NewRegistry()
    _ = r.Register(NewCPUCollector())           // macOS 동작 ✓
    _ = r.Register(NewMemoryCollector())        // macOS 동작 ✓
    _ = r.Register(NewDiskCollector())          // macOS 동작 ✓
    _ = r.Register(NewNetworkCollector())       // macOS 동작 ✓
    _ = r.Register(NewTemperatureCollector())   // 빈 데이터 (gopsutil)
    _ = r.Register(NewCPUProcessCollector())    // macOS 동작 ✓
    _ = r.Register(NewMemoryProcessCollector()) // macOS 동작 ✓
    _ = r.Register(NewFanCollector())           // 빈 데이터 (sysfs only)
    _ = r.Register(NewGpuCollector())           // Stub, 빈 데이터
    _ = r.Register(NewStorageSmartCollector())  // Stub, 빈 데이터
    _ = r.Register(NewVoltageCollector())       // Stub, 빈 데이터
    _ = r.Register(NewMotherboardTempCollector()) // Stub, 빈 데이터
    _ = r.Register(NewUptimeCollector())        // macOS 동작 ✓
    _ = r.Register(NewProcessWatchCollector())  // macOS 동작 ✓
    return r
}
```

> **참고**: `fan_unix.go`에 이미 `runtime.GOOS == "darwin"` 분기가 있으므로, 정식 분리 시 이 체크를 제거하고 `fan_linux.go` + `fan_darwin.go`로 분리.

---

## 8. IOReport CGO 브릿지 구현 패턴

### 8.1 CGO 디렉티브

```go
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreFoundation -framework IOKit -framework Foundation -lIOReport
```

추가 include (mactop 참조):
```c
#include <mach/mach_host.h>
#include <mach/processor_info.h>
#include <mach/mach_init.h>
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <stdint.h>
#include <string.h>
#include <stdlib.h>
```

선택 사항: `-framework CoreWLAN` (WiFi 메트릭용).

### 8.2 IOReport C 함수 시그니처

```c
// 채널 관리
extern CFDictionaryRef IOReportCopyChannelsInGroup(
    CFStringRef group,        // "Energy Model", "GPU Stats", "CPU Stats"
    CFStringRef subgroup,     // NULL 또는 특정 서브그룹
    uint64_t a, uint64_t b, uint64_t c  // 예약 (0 전달)
);

extern void IOReportMergeChannels(
    CFDictionaryRef a, CFDictionaryRef b, CFTypeRef unused
);

// 구독 & 샘플링
extern IOReportSubscriptionRef IOReportCreateSubscription(
    void* a,                           // NULL (예약)
    CFMutableDictionaryRef channels,   // 병합된 채널 dict
    CFMutableDictionaryRef* out,       // 출력 구독 채널
    uint64_t d, CFTypeRef e            // 예약 (0, NULL)
);

extern CFDictionaryRef IOReportCreateSamples(
    IOReportSubscriptionRef sub,
    CFMutableDictionaryRef channels,
    CFTypeRef unused
);

extern CFDictionaryRef IOReportCreateSamplesDelta(
    CFDictionaryRef sample1, CFDictionaryRef sample2, CFTypeRef unused
);

// 데이터 추출
extern int64_t IOReportSimpleGetIntegerValue(CFDictionaryRef item, int32_t idx);

// 채널 메타데이터
extern CFStringRef IOReportChannelGetGroup(CFDictionaryRef item);
extern CFStringRef IOReportChannelGetSubGroup(CFDictionaryRef item);
extern CFStringRef IOReportChannelGetChannelName(CFDictionaryRef item);
extern CFStringRef IOReportChannelGetUnitLabel(CFDictionaryRef item);

// 상태 잔류 (주파수 히스토그램용)
extern int32_t IOReportStateGetCount(CFDictionaryRef item);
extern CFStringRef IOReportStateGetNameForIndex(CFDictionaryRef item, int32_t idx);
extern int64_t IOReportStateGetResidency(CFDictionaryRef item, int32_t idx);
```

### 8.3 채널 그룹

| 그룹 | 데이터 | 단위 |
|------|--------|------|
| `"Energy Model"` | CPU/GPU/ANE/DRAM 전력 | mJ/uJ/nJ → W 변환 |
| `"CPU Stats"` / `"CPU Core Performance States"` | E/P 클러스터 주파수/활용률 | Hz, % |
| `"GPU Stats"` / `"GPU Performance States"` | GPU 주파수/활용률 | Hz, % |

### 8.4 데이터 흐름

```
Go struct ← CGO ← Obj-C samplePowerMetrics() ← IOReport delta ← IOReport samples
```

### 8.5 Go 래퍼 패턴 (mactop 참조)

```go
type SocMetrics struct {
    CPUPower  float64 `json:"cpu_power"`
    GPUPower  float64 `json:"gpu_power"`
    ANEPower  float64 `json:"ane_power"`
    DRAMPower float64 `json:"dram_power"`
}

func sampleSocMetrics(durationMs int) SocMetrics {
    pm := C.samplePowerMetrics(C.int(durationMs))
    return SocMetrics{
        CPUPower:  float64(pm.cpuPower),
        GPUPower:  float64(pm.gpuPower),
        ANEPower:  float64(pm.anePower),
        DRAMPower: float64(pm.dramPower),
    }
}
```

### 8.6 Objective-C PowerMetrics 구조체

```c
typedef struct {
    double cpuPower;
    double gpuPower;
    double anePower;
    double dramPower;
    double gpuSramPower;
    double systemPower;
    int gpuFreqMHz;
    double gpuActive;
    double eClusterActive;
    double pClusterActive;
    int eClusterFreqMHz;
    int pClusterFreqMHz;
    float socTemp;
    float cpuTemp;
    float gpuTemp;
} PowerMetrics;
```

### 8.7 에너지→전력 변환

```c
double energyToWatts(int64_t energy, CFStringRef unitLabel, double elapsedSeconds) {
    double energyInJ = 0.0;
    if (CFStringCompare(unitLabel, CFSTR("mJ"), 0) == kCFCompareEqualTo)
        energyInJ = energy / 1000.0;          // millijoules → joules
    else if (/* "uJ" */)
        energyInJ = energy / 1000000.0;       // microjoules → joules
    else if (/* "nJ" */)
        energyInJ = energy / 1000000000.0;    // nanojoules → joules
    return energyInJ / elapsedSeconds;
}
```

### 8.8 IOHIDEventSystemClient API (온도 Fallback)

```c
extern IOHIDEventSystemClientRef IOHIDEventSystemClientCreate(CFAllocatorRef allocator);
extern int IOHIDEventSystemClientSetMatching(IOHIDEventSystemClientRef client, CFDictionaryRef matching);
extern CFArrayRef IOHIDEventSystemClientCopyServices(IOHIDEventSystemClientRef client);
extern CFStringRef IOHIDServiceClientCopyProperty(IOHIDServiceClientRef service, CFStringRef key);
extern IOHIDEventRef IOHIDServiceClientCopyEvent(IOHIDServiceClientRef service, int64_t type, int32_t options, int64_t timeout);
extern double IOHIDEventGetFloatValue(IOHIDEventRef event, int64_t field);
```

**온도 Fallback 계층**:
1. SMC (macOS 14+ Apple Silicon도 지원)
2. IOHIDEventSystemClient (M1+, sudo 불필요)
3. 둘 다 실패 → graceful 빈 반환

### 8.9 메모리 관리 규칙

- `IOReportCopy*` 반환값은 `CFRelease()` 필수
- Subscription: **한 번 생성, 반복 재사용** (샘플마다 새로 만들지 않음)
- Go `runtime.SetFinalizer`로 CF 객체 자동 해제
- CFString 변환: `C.GoString(C.CFStringGetCStringPtr(cfStr, 0))`

---

## 9. 구현 전략

### 9.1 패러다임 전환

- **기존 가정** (오류): "2개 파일 분리 필수, 상당한 코드 변경 필요"
- **수정된 결론**: "분리는 개선 사항이지, 필수가 아님. 이미 빌드·실행됨."

### 9.2 구현 Phase

| Phase | 작업 | M4 Pro 로컬 | 외부 의존성 |
|-------|------|:---:|:---:|
| **Phase A** | 무변경 빌드 & file sender 테스트 | Yes | 없음 |
| **Phase B** | 온도 센서 (SMC / IOHIDEvent) | Yes, CGO | 없음 |
| **Phase C** | 팬 속도 (SMC `F*Ac`) | Yes, CGO | 없음 |
| **Phase D** | GPU/전력 (IOReport Obj-C 브릿지) | Yes, CGO | 없음 |
| **Phase E** | 전압/마더보드 (SMC 키) | Yes, CGO | 없음 |
| **Phase F** | SMART (smartctl) | Yes | `brew install smartmontools` |

**모든 Phase가 M4 Pro 로컬에서 완료 가능, 외부 인프라 불필요.**

### 9.3 라이브러리 선택 변경

- ~~iSMC 단독~~ → **mactop 참조 + 커스텀 추상화** (Option B+D)
- mactop의 `ioreport.m` + 전력 수집 코드에서 센서 수집 레이어만 추출
- iSMC는 SMC 키 발견/확인용 보조 도구로 유지
- mactop TUI 코드는 복사하지 않음; headless 센서 레이어만 추출

### 9.4 파일 구조

기존 `*_unix.go` 파일은 그대로 유지하며, 센서 구현 시에만 `_darwin.go` 파일 2~3개 추가:

```
internal/collector/
  temperature_unix.go         -- 기존 유지 (Linux gopsutil)
  temperature_darwin.go       -- 신규: SMC/IOHIDEvent (CGO)
  fan_unix.go                 -- 기존 유지 (Linux sysfs + Darwin nil 분기)
  fan_darwin.go               -- 신규: SMC F*Ac 키 (CGO)
  gpu_darwin.go               -- 신규: IOReport Obj-C 브릿지 (CGO)
  # 나머지는 기존 stub 유지 (voltage, motherboard_temp, storage_smart)
```

### 9.5 빌드 명령

```bash
# Phase A (CGO 불필요, 현재 코드)
go build -o ResourceAgent ./cmd/resourceagent

# Phase B~F (CGO 필요)
CGO_ENABLED=1 go build -o ResourceAgent ./cmd/resourceagent

# macOS Intel 빌드
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build ./cmd/resourceagent

# Universal 바이너리
lipo -create arm64-binary amd64-binary -output ResourceAgent-universal
```

---

## 10. 빌드/CI/테스트 전략

### 10.1 현재 M4 Pro 환경

- Go 1.25.6 darwin/arm64 설치됨
- `go build ./cmd/resourceagent` → 15MB Mach-O arm64 바이너리 (성공)
- 기존 단위 테스트 전부 PASS

### 10.2 크로스 컴파일 제약

Linux → Darwin은 **비실용적** (IOReport private 프레임워크가 osxcross에 없음).
**네이티브 macOS 빌드 필수.**

### 10.3 CI/CD GitHub Actions Runner

| Runner | OS | 칩 | 비용 |
|--------|----|----|------|
| `macos-14` | Sonoma | M1/M2 | ~$0.16/min |
| `macos-15` | Sequoia | M2-M4 | ~$0.16/min |

- IOReport/IOKit/CoreFoundation 프레임워크 CI runner에서 사용 가능
- **macOS runner는 Linux의 10배 비용** — 빠른 테스트는 Linux에서, 최종 빌드만 macOS

### 10.4 무료 CI 옵션

| 방법 | 조건 | macOS 빌드 |
|------|------|-----------|
| GitHub Actions (public repo) | 레포 공개 | 무제한 무료 |
| GitHub Actions (private repo) | 2,000분/월 무료 | macOS 10배 과금 → 실질 200분 |
| Self-hosted runner | M4 Pro를 runner로 등록 | 무료, 무제한 |

### 10.5 M4 Pro 로컬 테스트 플랜 (file sender, 인프라 불필요)

**Phase 1: 설정 (5분)**

```bash
mkdir -p /tmp/resourceagent-test/log/ResourceAgent
```

최소 `ResourceAgent.json`:
```json
{
  "Agent": { "ID": "M4-TEST", "Hostname": "" },
  "SenderType": "file",
  "File": {
    "FilePath": "/tmp/resourceagent-test/log/ResourceAgent/metrics.jsonl",
    "MaxSizeMB": 10, "MaxBackups": 2,
    "Console": true, "Pretty": false, "Format": "legacy"
  }
}
```

최소 `Monitor.json` (확인된 동작 collector만 활성화):
```json
{
  "Collectors": {
    "CPU": { "Enabled": true, "Interval": "5s" },
    "Memory": { "Enabled": true, "Interval": "5s" },
    "Disk": { "Enabled": true, "Interval": "10s", "Disks": [] },
    "Network": { "Enabled": true, "Interval": "5s", "Interfaces": [] },
    "CPUProcess": { "Enabled": true, "Interval": "10s", "TopN": 5 },
    "MemoryProcess": { "Enabled": true, "Interval": "10s", "TopN": 5 },
    "Uptime": { "Enabled": true, "Interval": "30s" },
    "Temperature": { "Enabled": true, "Interval": "10s" },
    "Fan": { "Enabled": false },
    "GPU": { "Enabled": false },
    "Voltage": { "Enabled": false },
    "MotherboardTemp": { "Enabled": false },
    "StorageSmart": { "Enabled": false },
    "ProcessWatch": { "Enabled": false }
  }
}
```

**Phase 2: 빌드 & 실행**
```bash
cd /path/to/ResourceAgent
go build -o /tmp/resourceagent-test/ResourceAgent ./cmd/resourceagent
cd /tmp/resourceagent-test
./ResourceAgent -config ./ResourceAgent.json -monitor ./Monitor.json -logging ./Logging.json
```

**Phase 3: 검증**
```bash
grep "category:cpu" /tmp/resourceagent-test/log/ResourceAgent/metrics.jsonl
grep "category:memory" /tmp/resourceagent-test/log/ResourceAgent/metrics.jsonl
grep "category:disk" /tmp/resourceagent-test/log/ResourceAgent/metrics.jsonl
grep "category:network" /tmp/resourceagent-test/log/ResourceAgent/metrics.jsonl
```

### 10.6 테스트 전략 계층

| 계층 | 목적 | 하드웨어 필요 |
|------|------|:---:|
| **단위 테스트** | 센서 데이터 파싱/필터링 로직 (mock 데이터) | No |
| **통합 테스트** | CI macOS runner, 센서 없어도 패닉 없음 확인 | CI runner |
| **모델별 테스트** | 실제 Mac 하드웨어, 수동 검증 | Yes (CI 불가) |

### 10.7 빌드 매트릭스

- `darwin/arm64` (Apple Silicon) — 주요 타겟 (공장 Mac 대부분 M-series)
- `darwin/amd64` (Intel Mac) — 필요 시 추후 추가
- Universal 바이너리: `lipo -create arm64 amd64 -output universal`

---

## 11. 리스크 및 제약

| 리스크 | 영향 | 대응 |
|--------|------|------|
| Apple Silicon 온도 = 버킷 (섭씨 아님) | EARS 포맷 호환성 문제 | 버킷→수치 매핑 또는 별도 카테고리 정의 |
| CI macOS runner 비용 (Linux 10배) | 비용 증가 | 빌드 매트릭스 최소화, 가능한 테스트 Linux에서 실행 |
| 크로스 컴파일 불가 | macOS 빌드 인프라 필요 | 네이티브 빌드 runner만 사용 |
| mactop M4 CPU 주파수 버그 (1MHz) | 부정확한 메트릭 | v0.6.0+ 검증, 자체 구현에서 검증 |
| macmon M4 주파수 버그 미해결 | 제한된 참조 구현 | mactop을 주요 참조로 사용 |
| iSMC M2/M3/M4 지원 불완전 (#23) | 일부 센서 누락 | mactop 하이브리드 접근법으로 대체 |
| IOReport은 private API | Apple이 언제든 변경 가능 | macOS 버전별 분기 + 방어적 코딩 |
| SMART에 sudo 필요 | 에이전트 권한 문제 | Windows LhmHelper 패턴과 동일한 privileged helper 도입 |
| Apple 내장 SSD의 비표준 SMART | 일부 속성 누락 가능 | smartctl JSON에서 가능한 필드만 수집 |
| 팬리스 모델 (MacBook Air) | 팬 데이터 없음 | 팬 collector는 graceful 미지원 처리 |
| CGO 크로스 컴파일 어려움 | CI/CD 복잡도 증가 | macOS 네이티브 runner 사용 |

---

## 12. 참고 자료

- [iSMC](https://github.com/dkorunic/iSMC) — Go SMC 라이브러리
- [iSMC Issue #23](https://github.com/dkorunic/iSMC/issues/23) — M2/M3 지원 (caveats 있음)
- [mactop](https://github.com/metaspartan/mactop) — Go Apple Silicon 모니터 (IOReport 참조, M4 Pro 확인됨)
- [Homebrew mactop Formula](https://formulae.brew.sh/formula/mactop)
- [macmon](https://github.com/vladkens/macmon) — Rust sudoless 모니터
- [socpowerbud](https://github.com/dehydratedpotato/socpowerbud) — Obj-C 전력/주파수
- [apple_sensors](https://github.com/fermion-star/apple_sensors) — M1 온도 센서 참조
- [test-ioreport](https://github.com/freedomtan/test-ioreport) — Obj-C IOReport 참조 구현
- [Stats](https://github.com/exelban/stats) — Swift macOS 메뉴바 시스템 모니터 (커뮤니티 M3 키 발견)
- [gosmc](https://github.com/charlie0129/gosmc) — Go SMC 라이브러리
- [SMC Sensor Codes](https://logi.wiki/index.php/SMC_Sensor_Codes) — SMC 키 목록
- [gopsutil #1832](https://github.com/shirou/gopsutil/issues/1832) — macOS 센서 크래시 이슈
- [smartmontools](https://formulae.brew.sh/formula/smartmontools) — SMART 도구
- [GitHub Actions macOS Runner Specs](https://github.com/actions/runner-images/blob/main/images/macos/README.md) — CI runner 사양
