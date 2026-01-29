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
