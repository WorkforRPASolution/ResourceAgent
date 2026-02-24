package network

import (
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
