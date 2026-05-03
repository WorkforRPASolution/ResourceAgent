# FileSender drainConsole vs `os.Stdout` 재설정 race

## 현상

`go test -race ./internal/sender/...` 실행 시 `TestFileSender_FileWriteNotBlockedByConsole` 가 race detector 에 의해 실패:

```
WARNING: DATA RACE
Write at <addr> by goroutine 8:
  TestFileSender_FileWriteNotBlockedByConsole
      file_test.go:758 (os.Stdout = origStdout)

Previous read at <addr> by goroutine 9:
  fmt.Println()
  (*FileSender).drainConsole()
      file.go:169
```

- 같은 `os.Stdout` (전역 변수) 에 대해 테스트 메인 goroutine 의 write 와 `drainConsole` goroutine 의 `fmt.Println` 이 동시 접근.
- `--race` 플래그 없이는 검출 안 됨. `go test -race ./internal/sender/...` 직접 실행 시 표면화.

## 원인

`internal/sender/file.go:166-171`:
```go
func (s *FileSender) drainConsole() {
    defer close(s.consoleDone)
    for line := range s.consoleCh {
        fmt.Println(line)   // os.Stdout 읽기
    }
}
```

테스트 `internal/sender/file_test.go:721, 758`:
```go
os.Stdout = w               // line 721: write
// ... s.Send() → consoleCh → drainConsole 가 fmt.Println 호출 (os.Stdout 읽기) ...
os.Stdout = origStdout      // line 758: write (race!)
w.Close()
r.Close()
s.Close()                   // 여기서 비로소 consoleCh close → drainConsole 종료
```

테스트가 `s.Close()` 보다 먼저 `os.Stdout` 을 복원하기 때문에 drainConsole 이 아직 살아있는 상태에서 race 발생.

## 영향 / 우선순위

- **테스트 전용 race**. 프로덕션에서는 `os.Stdout` 재설정이 안 일어나므로 영향 없음.
- 본 race 는 `TestFileSender_FileWriteNotBlockedByConsole` 라는 **다른 race (Quick Edit Mode block) 검증용 테스트** 안에서 일어남. 테스트 의도는 file write 가 console block 에 막히지 않는 것을 검증하는 것.
- 우선순위 낮음 — 프로덕션 영향 없고 race detector flag 가 없으면 검출 안 됨.

## 수정 방향 (제안)

1. `s.Close()` 가 `drainConsole` 완료를 보장하도록 → 테스트에서 stdout 복원 전에 `s.Close()` 호출.
2. 또는 drainConsole 이 `os.Stdout` 캡처 시점을 확정 (e.g. `s.out io.Writer` 필드 도입, 생성자에서 `os.Stdout` 캡처).

옵션 2 가 더 깔끔 — production 코드 의존성을 명확히 하고, 테스트도 `out` 필드를 직접 주입해 `os.Stdout` 우회 가능.

## 발견 컨텍스트

2026-05-03, drainStderr H1 fix 작업 중 `go test -race ./...` 실행에서 검출. 해당 변경(`internal/collector` 만 건드림)과 무관한 사전 결함. 별도 PR 로 처리 예정.

## 관련 파일

- `internal/sender/file.go:78, 166-171`
- `internal/sender/file_test.go:710-770`
