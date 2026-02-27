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
		BaseCollector: NewBaseCollector("Network"),
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
	// OS syscalls outside lock
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return nil, err
	}

	conns, err := net.ConnectionsWithContext(ctx, "inet")
	if err != nil {
		// Non-fatal: return metrics without TCP counts
		conns = nil
	}

	// Snapshot previous state under lock
	c.mu.Lock()
	prevStats := c.lastStats
	prevCollect := c.lastCollect
	c.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(prevCollect).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}

	// Rate calculation outside lock
	var interfaces []NetworkInterface
	newStats := make(map[string]net.IOCountersStat, len(counters))

	for _, counter := range counters {
		if len(c.interfaces) > 0 && !c.shouldInclude(counter.Name) {
			continue
		}

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

		if prev, ok := prevStats[counter.Name]; ok && prevCollect.Unix() > 0 {
			bytesSentDiff := float64(counter.BytesSent - prev.BytesSent)
			bytesRecvDiff := float64(counter.BytesRecv - prev.BytesRecv)

			if bytesSentDiff >= 0 {
				iface.BytesSentRate = bytesSentDiff / elapsed
			}
			if bytesRecvDiff >= 0 {
				iface.BytesRecvRate = bytesRecvDiff / elapsed
			}
		}

		newStats[counter.Name] = counter
		interfaces = append(interfaces, iface)
	}

	// Update state under lock
	c.mu.Lock()
	c.lastStats = newStats
	c.lastCollect = now
	c.mu.Unlock()

	inbound, outbound := classifyTCPConnections(conns)

	return &MetricData{
		Type:      c.Name(),
		Timestamp: now,
		Data: NetworkData{
			Interfaces:       interfaces,
			TCPInboundCount:  inbound,
			TCPOutboundCount: outbound,
		},
	}, nil
}

// classifyTCPConnections classifies TCP connections into inbound (server) and outbound (client).
// Inbound connections have a local port that matches a LISTEN port.
// Outbound connections have a local port that does not match any LISTEN port.
// LISTEN connections themselves are excluded from the count.
func classifyTCPConnections(conns []net.ConnectionStat) (inbound, outbound int) {
	// Step 1: collect all LISTEN ports
	listenPorts := make(map[uint32]struct{})
	for _, c := range conns {
		if c.Status == "LISTEN" {
			listenPorts[c.Laddr.Port] = struct{}{}
		}
	}

	// Step 2: classify non-LISTEN connections
	for _, c := range conns {
		if c.Status == "LISTEN" {
			continue
		}
		if _, ok := listenPorts[c.Laddr.Port]; ok {
			inbound++
		} else {
			outbound++
		}
	}
	return
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
