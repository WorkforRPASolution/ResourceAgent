package sender

import (
	"os"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/IBM/sarama.(*asyncProducer).dispatcher"),
		goleak.IgnoreTopFunction("github.com/IBM/sarama.(*asyncProducer).retryHandler"),
		goleak.IgnoreTopFunction("github.com/IBM/sarama.(*asyncProducer).newPartitionProducer.func1"),
		goleak.IgnoreTopFunction("github.com/IBM/sarama.(*Broker).responseReceiver"),
		goleak.IgnoreTopFunction("github.com/IBM/sarama.withRecover"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop"),
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
		goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"),
	)
	os.Exit(m.Run())
}
