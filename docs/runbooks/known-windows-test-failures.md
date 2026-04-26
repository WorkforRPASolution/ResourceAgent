# Windows 환경 알려진 테스트 Skip

`runtime.GOOS == "windows"` 조건으로 skip 처리된 테스트 목록 + 사유.

## 원칙

- skip은 **OS-specific 가정 문제**에만 적용. 코드 버그는 skip 안 하고 fix.
- skip된 테스트는 macOS/Linux에서 정상 작동 — 운영 코드는 OS-aware로 동작.
- 본 목록 0건이 목표. 시간 날 때 OS-agnostic으로 재작성.

## 현재 skip 항목

### W-1: process watch 테스트 (2건)

- `internal/collector/cpu_process_test.go: TestCPUProcessCollector_Collect_WithWatchProcesses`
- `internal/collector/cpu_process_test.go: TestCPUProcessCollector_Collect_WatchedFirst`
- `internal/collector/memory_process_test.go: TestMemoryProcessCollector_Collect_WithWatchProcesses`
- `internal/collector/memory_process_test.go: TestMemoryProcessCollector_Collect_WatchedFirst`

**원인**: 테스트가 `WatchProcesses: []string{"go", "zsh"}` 가정. Windows엔 zsh 없음. `go` 도 보통 `go.exe`로 떠 있어 정확히 매칭 안 됨 (현재 process matcher는 정확 일치 또는 case-insensitive 가정).

**운영 영향**: 없음. Production 코드(`process_match.go`)는 OS 무관하게 동작. 테스트의 fixture 가정만 macOS-specific.

**fix 방향 (별도 작업)**:
- 옵션 A: Windows에서는 `WatchProcesses: []string{"explorer", "svchost"}` 같은 보편 프로세스 사용. 단, Linux에서도 다른 매칭 필요 → 결국 OS별 분기.
- 옵션 B: 테스트 안에서 현재 실행 중인 프로세스 1개 동적 선택 (`process.Processes()` 결과에서 첫 번째). 동적 fixture.
- 옵션 C: process matcher 단위 테스트 + `WatchProcesses` 통합 테스트 분리. 후자만 OS별 skip.

**권장**: 옵션 B (동적 fixture). 머지 후 처리.

## 카테고리 2 (fix 완료, 별도 PR로 처리)

이전에 Windows에서 fail이었으나 옵션 B 작업으로 fix된 항목 — 본 문서에는 이력만:

- `TestDaemonProcessCrash`: cleanup 누락 → `defer p.stopProcess()` 추가
- `TestInit_ConsoleBlockDoesNotBlockFileWrite`: file handle leak → `logger.Close()` API 추가 + `t.Cleanup`
- `TestInit_FileWriteNotBlockedByConsole`: 동일
- `TestInit_ReInitClosesOldWriter`: 동일

→ `logger.Close()` 는 production shutdown에도 사용 가능한 API. 부수 효과로 logger lifecycle 정리.

## 새 항목 추가 시

PR 본문에 다음 명시 필수:
- skip한 테스트 위치 + 함수명
- 운영 영향 (있으면 fix, 없으면 skip 정당화)
- fix 방향 + 추정 작업량
- 본 문서에 항목 추가
