# 알려진 Race Conditions (Step 0 race detector 발견)

`go test -race` 도입(Step 0) 시점에 발견된 race condition 목록. **별도 PR로 fix 진행**.

## R-1: FileSender.drainConsole vs 테스트 stdout 교체

- **발견일**: 2026-04-26 (Step 0 PR)
- **위치**:
  - Goroutine A (drainConsole): `internal/sender/file.go:169` (`fmt.Println` → `os.Stdout` 읽기)
  - Goroutine B (test): `internal/sender/file_test.go:694` (`os.Stdout = origStdout` 쓰기)
- **영향 테스트**:
  - `TestFileSender_SetConsole_EnablesOutput`
  - `TestFileSender_FileWriteNotBlockedByConsole`
- **분류**: 테스트 코드의 `os.Stdout` 전역 mutate 패턴이 살아있는 drainConsole goroutine과 race
- **운영 영향**: 프로덕션에서는 `os.Stdout`이 변하지 않으므로 실제 race 미발생. 테스트 한정.
- **fix 방향 (별도 PR)**:
  - 옵션 A: 테스트가 `s.Close()` 호출하여 drainConsole 종료 후 stdout 복원
  - 옵션 B: `FileSender` 가 자체 `io.Writer` 필드 보유 + 테스트는 그것만 swap
- **임시 우회**: ci.sh basic 모드는 `-race` 없음. `ci.sh --full` 또는 `go test -race` 직접 호출 시에만 발생.

## 정책

- ci.sh **basic 모드**: race 없이 (빠른 PR 검증, PASS 보장)
- ci.sh **--full 모드**: race 포함 (머지 전 강한 검증)
- 새 race 발견 시 본 문서에 기록 + 별도 fix PR
- 본 문서의 항목이 0개가 되면 ci.sh basic 모드에 race 포함 검토
