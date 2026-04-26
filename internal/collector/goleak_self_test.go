package collector

import (
	"os"
	"testing"
	"time"
)

// TestGoleakDetectsLeak는 goleak 인프라가 의도된 leak을 검출하는지 자기 검증.
//
// 환경 변수 가드:
//   - 일반 실행: skip (의도적 leak이라 정상 PASS 시나리오 아님)
//   - 인프라 검증 시: VERIFY_GOLEAK_INFRA=1 로 실행. TestMain 종료 시 goleak이
//     본 함수에서 leak된 goroutine을 보고해야 함 (test process exit code 1).
//
// 사용법:
//
//	VERIFY_GOLEAK_INFRA=1 go test -run TestGoleakDetectsLeak ./internal/collector/...
func TestGoleakDetectsLeak(t *testing.T) {
	if os.Getenv("VERIFY_GOLEAK_INFRA") != "1" {
		t.Skip("set VERIFY_GOLEAK_INFRA=1 to verify goleak infrastructure")
	}
	go func() { time.Sleep(time.Hour) }()
}
