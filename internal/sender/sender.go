// Package sender provides interfaces and implementations for sending metrics.
package sender

import (
	"context"

	"resourceagent/internal/collector"
)

// Sender defines the interface for sending collected metrics.
type Sender interface {
	// Send transmits the metric data to the destination.
	Send(ctx context.Context, data *collector.MetricData) error

	// SendBatch transmits multiple metric data items.
	SendBatch(ctx context.Context, data []*collector.MetricData) error

	// Close releases any resources held by the sender.
	Close() error
}

// BufferStatsProvider exposes transport-level buffer observability.
// Implemented by BufferedHTTPTransport (Phase 2-1) and surfaced through
// KafkaSender. The matching contract in the collector package
// (collector.BufferStatsProvider) is satisfied via duck typing so that
// the collector package does not depend on sender.
type BufferStatsProvider interface {
	BufferStats() (count, dropped, hwm int64)
}
