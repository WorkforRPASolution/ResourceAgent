package sender

import "testing"

func TestExtractHost(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{"192.168.0.100", "192.168.0.100"},
		{"192.168.0.100:8082", "192.168.0.100"},
		{"http://192.168.0.100", "192.168.0.100"},
		{"http://192.168.0.100:8082", "192.168.0.100"},
		{"https://192.168.0.100:8082", "192.168.0.100"},
		{"kafka-broker.local", "kafka-broker.local"},
		{"kafka-broker.local:9092", "kafka-broker.local"},
		{"http://kafka-broker.local:8082", "kafka-broker.local"},
	}
	for _, tt := range tests {
		got := extractHost(tt.addr)
		if got != tt.want {
			t.Errorf("extractHost(%q) = %q, want %q", tt.addr, got, tt.want)
		}
	}
}

func TestResolveBrokerAddr(t *testing.T) {
	tests := []struct {
		name          string
		kafkaRestAddr string
		brokerPort    int
		wantAddr      string
		wantErr       bool
	}{
		{
			name:          "IP with port",
			kafkaRestAddr: "http://10.20.30.40:8082",
			brokerPort:    9092,
			wantAddr:      "10.20.30.40:9092",
		},
		{
			name:          "IP without scheme",
			kafkaRestAddr: "10.20.30.40:8082",
			brokerPort:    9092,
			wantAddr:      "10.20.30.40:9092",
		},
		{
			name:          "hostname with scheme and port",
			kafkaRestAddr: "http://kafka-node.local:8082",
			brokerPort:    9093,
			wantAddr:      "kafka-node.local:9093",
		},
		{
			name:          "IP only no port",
			kafkaRestAddr: "10.20.30.40",
			brokerPort:    9092,
			wantAddr:      "10.20.30.40:9092",
		},
		{
			name:          "zero brokerPort defaults to 9092",
			kafkaRestAddr: "10.20.30.40:8082",
			brokerPort:    0,
			wantAddr:      "10.20.30.40:9092",
		},
		{
			name:          "empty KafkaRestAddress returns error",
			kafkaRestAddr: "",
			brokerPort:    9092,
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveBrokerAddr(tt.kafkaRestAddr, tt.brokerPort)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ResolveBrokerAddr(%q, %d) expected error, got %q", tt.kafkaRestAddr, tt.brokerPort, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveBrokerAddr(%q, %d) unexpected error: %v", tt.kafkaRestAddr, tt.brokerPort, err)
			}
			if got != tt.wantAddr {
				t.Errorf("ResolveBrokerAddr(%q, %d) = %q, want %q", tt.kafkaRestAddr, tt.brokerPort, got, tt.wantAddr)
			}
		})
	}
}
