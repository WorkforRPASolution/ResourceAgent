package sender

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
)

// TestClockMockAdvancesTime은 mock clock이 가상 시간 진행 시 timer를 발화시키는지 검증.
// Step 5(Phase 2-1) production 코드 교체 전 인프라 자기 검증.
func TestClockMockAdvancesTime(t *testing.T) {
	mock := clock.NewMock()
	timer := mock.Timer(5 * time.Second)
	mock.Add(5 * time.Second)
	select {
	case <-timer.C:
		// PASS
	case <-time.After(100 * time.Millisecond):
		t.Fatal("mock timer did not fire after Add")
	}
}

// TestDefaultClockIsRealClock은 production용 defaultClock이 실시간임을 확인.
func TestDefaultClockIsRealClock(t *testing.T) {
	start := defaultClock.Now()
	time.Sleep(20 * time.Millisecond)
	elapsed := defaultClock.Since(start)
	if elapsed < 15*time.Millisecond {
		t.Fatalf("defaultClock should track real time, got elapsed=%v", elapsed)
	}
}
