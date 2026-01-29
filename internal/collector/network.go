package collector

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/net"

	"resourceagent/internal/config"
)

// NetworkCollector collects network interface metrics.
type NetworkCollector struct {
	BaseCollector
	interfaces []string // Specific interfaces to monitor; empty means all

	// For rate calculation
	mu          sync.Mutex
	lastStats   map[string]net.IOCountersStat
	lastCollect time.Time
}

// NewNetworkCollector creates a new network collector.
func NewNetworkCollector() *NetworkCollector {
	return &NetworkCollector{
		BaseCollector: NewBaseCollector("network"),
		lastStats:     make(map[string]net.IOCountersStat),
	}
}

// Configure applies the configuration to the collector.
func (c *NetworkCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	c.interfaces = cfg.Interfaces
	return nil
}

// Collect gathers network metrics.
func (c *NetworkCollector) Collect(ctx context.Context) (*MetricData, error) {
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(c.lastCollect).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}

	var interfaces []NetworkInterface

	for _, counter := range counters {
		// Skip if specific interfaces are configured and this one isn't in the list
		if len(c.interfaces) > 0 && !c.shouldInclude(counter.Name) {
			continue
		}

		// Skip loopback and virtual interfaces
		if c.shouldSkip(counter.Name) {
			continue
		}

		iface := NetworkInterface{
			Name:        counter.Name,
			BytesSent:   counter.BytesSent,
			BytesRecv:   counter.BytesRecv,
			PacketsSent: counter.PacketsSent,
			PacketsRecv: counter.PacketsRecv,
			ErrorsIn:    counter.Errin,
			ErrorsOut:   counter.Errout,
			DropsIn:     counter.Dropin,
			DropsOut:    counter.Dropout,
		}

		// Calculate rates if we have previous data
		if prev, ok := c.lastStats[counter.Name]; ok && c.lastCollect.Unix() > 0 {
			bytesSentDiff := float64(counter.BytesSent - prev.BytesSent)
			bytesRecvDiff := float64(counter.BytesRecv - prev.BytesRecv)

			// Handle counter wraparound
			if bytesSentDiff >= 0 {
				iface.BytesSentRate = bytesSentDiff / elapsed
			}
			if bytesRecvDiff >= 0 {
				iface.BytesRecvRate = bytesRecvDiff / elapsed
			}
		}

		// Store current stats for next rate calculation
		c.lastStats[counter.Name] = counter

		interfaces = append(interfaces, iface)
	}

	c.lastCollect = now

	return &MetricData{
		Type:      c.Name(),
		Timestamp: now.UTC(),
		Data:      NetworkData{Interfaces: interfaces},
	}, nil
}

func (c *NetworkCollector) shouldInclude(name string) bool {
	for _, iface := range c.interfaces {
		if iface == name {
			return true
		}
	}
	return false
}

func (c *NetworkCollector) shouldSkip(name string) bool {
	nameLower := strings.ToLower(name)

	// Skip loopback
	if nameLower == "lo" || nameLower == "loopback" {
		return true
	}

	// Skip common virtual interfaces
	skipPrefixes := []string{
		"veth", "docker", "br-", "virbr", "vbox", "vmnet",
		"flannel", "cni", "calico", "weave",
	}

	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(nameLower, prefix) {
			return true
		}
	}

	return false
}
