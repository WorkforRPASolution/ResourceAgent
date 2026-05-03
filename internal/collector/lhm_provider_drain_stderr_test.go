//go:build windows

package collector

import (
	"io"
	"runtime"
	"testing"
	"testing/iotest"
	"time"
)

// panicReader는 Read 호출 시 panic 을 발생시키는 io.Reader.
// drainStderr 의 panic recovery 검증용.
type panicReader struct{}

func (panicReader) Read(p []byte) (int, error) {
	panic("test: panicReader.Read invoked")
}

// runDrainStderr 은 drainStderr 를 별도 goroutine 으로 실행하고
// 정상 종료 또는 timeout 을 감지한다.
// panic recovery 가 안 되어 있으면 테스트 프로세스 자체가 죽는다.
func runDrainStderr(t *testing.T, p *LhmProvider, r io.Reader, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.drainStderr(r)
	}()
	select {
	case <-done:
		// 정상 종료
	case <-time.After(timeout):
		t.Fatal("drainStderr did not return within timeout")
	}
}

func TestDrainStderr_PanicRecovery(t *testing.T) {
	p := &LhmProvider{}
	// panicReader 가 panic 을 던져도 drainStderr 는 잡아내고 정상 리턴해야 한다.
	runDrainStderr(t, p, panicReader{}, 2*time.Second)
}

func TestDrainStderr_ScannerError(t *testing.T) {
	p := &LhmProvider{}
	// iotest.ErrReader 는 scanner.Err() 가 nil 아닌 값을 반환하도록 한다.
	// drainStderr 는 정상 종료해야 한다 (행어 안 됨).
	r := iotest.ErrReader(io.ErrUnexpectedEOF)
	runDrainStderr(t, p, r, 2*time.Second)
}

func TestDrainStderr_NoGoroutineLeak(t *testing.T) {
	p := &LhmProvider{}
	before := runtime.NumGoroutine()

	// 100회 panic 반복. recovery 가 잘 동작해도 goroutine 이 누적되면 fail.
	for i := 0; i < 100; i++ {
		runDrainStderr(t, p, panicReader{}, 1*time.Second)
	}

	// goroutine 정리 시간 약간 부여
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after-before > 2 {
		t.Errorf("goroutine leak detected: before=%d after=%d (delta=%d)", before, after, after-before)
	}
}
