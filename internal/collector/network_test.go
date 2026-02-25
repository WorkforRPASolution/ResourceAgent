package collector

import (
	"context"
	"testing"

	"github.com/shirou/gopsutil/v3/net"
)

func TestClassifyTCPConnections_InboundOutbound(t *testing.T) {
	// Simulate: port 80 and 443 are listening
	// Connections on those ports from remote clients = inbound
	// Connections to remote ports = outbound
	conns := []net.ConnectionStat{
		// LISTEN on port 80
		{Status: "LISTEN", Laddr: net.Addr{IP: "0.0.0.0", Port: 80}, Raddr: net.Addr{}},
		// LISTEN on port 443
		{Status: "LISTEN", Laddr: net.Addr{IP: "0.0.0.0", Port: 443}, Raddr: net.Addr{}},
		// Inbound: remote client connected to local port 80
		{Status: "ESTABLISHED", Laddr: net.Addr{IP: "192.168.1.10", Port: 80}, Raddr: net.Addr{IP: "10.0.0.5", Port: 50000}},
		// Inbound: remote client connected to local port 443
		{Status: "ESTABLISHED", Laddr: net.Addr{IP: "192.168.1.10", Port: 443}, Raddr: net.Addr{IP: "10.0.0.6", Port: 50001}},
		// Outbound: local client connected to remote port 3306
		{Status: "ESTABLISHED", Laddr: net.Addr{IP: "192.168.1.10", Port: 55000}, Raddr: net.Addr{IP: "10.0.0.100", Port: 3306}},
		// Outbound: local client connected to remote port 6379
		{Status: "ESTABLISHED", Laddr: net.Addr{IP: "192.168.1.10", Port: 55001}, Raddr: net.Addr{IP: "10.0.0.101", Port: 6379}},
		// Inbound CLOSE_WAIT (server-side close wait)
		{Status: "CLOSE_WAIT", Laddr: net.Addr{IP: "192.168.1.10", Port: 80}, Raddr: net.Addr{IP: "10.0.0.7", Port: 50002}},
		// Outbound TIME_WAIT
		{Status: "TIME_WAIT", Laddr: net.Addr{IP: "192.168.1.10", Port: 55002}, Raddr: net.Addr{IP: "10.0.0.102", Port: 8080}},
	}

	inbound, outbound := classifyTCPConnections(conns)
	// Inbound: 2 ESTABLISHED on port 80/443 + 1 CLOSE_WAIT on port 80 = 3
	if inbound != 3 {
		t.Errorf("expected inbound=3, got %d", inbound)
	}
	// Outbound: 2 ESTABLISHED on non-listen ports + 1 TIME_WAIT on non-listen port = 3
	if outbound != 3 {
		t.Errorf("expected outbound=3, got %d", outbound)
	}
}

func TestClassifyTCPConnections_NoConnections(t *testing.T) {
	inbound, outbound := classifyTCPConnections(nil)
	if inbound != 0 {
		t.Errorf("expected inbound=0, got %d", inbound)
	}
	if outbound != 0 {
		t.Errorf("expected outbound=0, got %d", outbound)
	}
}

func TestClassifyTCPConnections_ListenOnly(t *testing.T) {
	conns := []net.ConnectionStat{
		{Status: "LISTEN", Laddr: net.Addr{IP: "0.0.0.0", Port: 80}},
		{Status: "LISTEN", Laddr: net.Addr{IP: "0.0.0.0", Port: 443}},
	}
	inbound, outbound := classifyTCPConnections(conns)
	// LISTEN connections are excluded from count
	if inbound != 0 {
		t.Errorf("expected inbound=0, got %d", inbound)
	}
	if outbound != 0 {
		t.Errorf("expected outbound=0, got %d", outbound)
	}
}

func TestClassifyTCPConnections_AllOutbound(t *testing.T) {
	// No LISTEN ports → everything is outbound
	conns := []net.ConnectionStat{
		{Status: "ESTABLISHED", Laddr: net.Addr{IP: "192.168.1.10", Port: 55000}, Raddr: net.Addr{IP: "10.0.0.1", Port: 80}},
		{Status: "ESTABLISHED", Laddr: net.Addr{IP: "192.168.1.10", Port: 55001}, Raddr: net.Addr{IP: "10.0.0.2", Port: 443}},
	}
	inbound, outbound := classifyTCPConnections(conns)
	if inbound != 0 {
		t.Errorf("expected inbound=0, got %d", inbound)
	}
	if outbound != 2 {
		t.Errorf("expected outbound=2, got %d", outbound)
	}
}

func TestNetworkCollect_HasTCPCounts(t *testing.T) {
	c := NewNetworkCollector()
	result, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	nd, ok := result.Data.(NetworkData)
	if !ok {
		t.Fatalf("expected NetworkData, got %T", result.Data)
	}

	// On any system, TCP counts should be non-negative
	if nd.TCPInboundCount < 0 {
		t.Errorf("TCPInboundCount should be >= 0, got %d", nd.TCPInboundCount)
	}
	if nd.TCPOutboundCount < 0 {
		t.Errorf("TCPOutboundCount should be >= 0, got %d", nd.TCPOutboundCount)
	}
	// On a dev machine there should be at least some TCP connections
	if nd.TCPInboundCount+nd.TCPOutboundCount == 0 {
		t.Log("warning: no TCP connections detected (may be expected in CI)")
	}
}
