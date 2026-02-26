package network

import (
	"net"
	"strings"
	"testing"
)

func TestDetectIPs_EmptyPattern_ReturnsUnderscore(t *testing.T) {
	info, err := DetectIPs("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IPAddrLocal != "_" {
		t.Errorf("expected IPAddrLocal='_', got %q", info.IPAddrLocal)
	}
}

func TestDetectIPs_InvalidPattern_ReturnsError(t *testing.T) {
	_, err := DetectIPs("[invalid", "")
	if err == nil {
		t.Fatal("expected error for invalid regex pattern, got nil")
	}
	if !strings.Contains(err.Error(), "invalid private IP pattern") {
		t.Errorf("error message should mention invalid pattern, got: %v", err)
	}
}

func TestDetectIPs_OverrideIP(t *testing.T) {
	override := "10.99.99.99"
	info, err := DetectIPs("", override)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.IPAddr != override {
		t.Errorf("expected IPAddr=%q, got %q", override, info.IPAddr)
	}
}

func TestDetectIPs_ReturnsNonLoopbackIPs(t *testing.T) {
	info, err := DetectIPs("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ip := range info.AllIPs {
		if strings.HasPrefix(ip, "127.") {
			t.Errorf("AllIPs should not contain loopback address, found %q", ip)
		}
	}
}

func TestDetectIPv4Addresses(t *testing.T) {
	ips, err := detectIPv4Addresses()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Most systems should have at least one non-loopback IPv4 address.
	// CI/container environments may not, so we just check no loopback.
	for _, ip := range ips {
		if strings.HasPrefix(ip, "127.") {
			t.Errorf("detectIPv4Addresses should not return loopback, found %q", ip)
		}
	}
	// Log for visibility
	t.Logf("Detected %d IPv4 address(es): %v", len(ips), ips)
}

func TestDetectIPByDial_ReturnsLocalIP(t *testing.T) {
	// Start a local TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer ln.Close()

	// Accept one connection in background to complete the handshake
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	ip, err := DetectIPByDial(ln.Addr().String(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip == "" {
		t.Fatal("expected non-empty IP, got empty string")
	}
	// Connecting to 127.0.0.1 should return 127.0.0.1
	if ip != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1, got %q", ip)
	}
	t.Logf("DetectIPByDial returned: %s", ip)
}

func TestDetectIPByDial_WithDialFunc(t *testing.T) {
	// Start a local TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	// Custom dialFunc that just uses net.Dial
	customDial := func(network, addr string) (net.Conn, error) {
		return net.Dial(network, addr)
	}

	ip, err := DetectIPByDial(ln.Addr().String(), customDial)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip == "" {
		t.Fatal("expected non-empty IP, got empty string")
	}
	t.Logf("DetectIPByDial with custom dialFunc returned: %s", ip)
}

func TestDetectIPByDial_UnreachableAddr(t *testing.T) {
	// Use an address that should fail to connect (RFC 5737 TEST-NET)
	_, err := DetectIPByDial("192.0.2.1:1", nil)
	if err == nil {
		t.Fatal("expected error for unreachable address, got nil")
	}
	t.Logf("Got expected error: %v", err)
}
