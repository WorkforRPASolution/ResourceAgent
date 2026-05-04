# 알려진 Race Conditions

`go test -race` 실행 시 발견된 race condition 목록. 별도 PR로 fix 진행.

## R-1: FileSender.drainConsole vs 테스트 stdout 교체 — **RESOLVED (2026-05-04)**

- **발견일**: 2026-04-26
- **해결일**: 2026-05-04
- **해결 전략**: Option B — `FileSender.out io.Writer` 필드 도입, 생성자에서 `os.Stdout` 캡처. 테스트는 `s.out = w` 직접 주입으로 `os.Stdout` 전역 변수를 건드리지 않음.
- **검증**: `go test -race -count=3 ./internal/sender/...` PASS
- **변경 파일**:
  - `internal/sender/file.go` (struct 필드 + 생성자 + drainConsole)
  - `internal/sender/file_test.go` (3개 테스트 재작성: SetConsole_DisablesOutput, SetConsole_EnablesOutput, FileWriteNotBlockedByConsole)

## 정책

- 새 race 발견 시 본 문서에 기록 + 별도 fix PR
- race 검증이 필요할 때는 `go test -race ./...` 수동 실행
