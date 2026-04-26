package sender

import "github.com/benbjohnson/clock"

// defaultClock는 production 기본 시간 소스.
// 테스트는 clock.NewMock() 으로 가상 시간 주입.
// production 코드의 time.After/Sleep 교체는 Step 5(Phase 2-1)에서 수행.
var defaultClock clock.Clock = clock.New()
