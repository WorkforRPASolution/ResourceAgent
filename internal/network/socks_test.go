package network

import (
	"context"
	"net"
	"sync/atomic"
	"testing"

	"resourceagent/internal/config"
)

func TestNewSOCKS5Dialer_CreatesDialer(t *testing.T) {
	dialer, err := NewSOCKS5Dialer("127.0.0.1", 1080)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dialer == nil {
		t.Fatal("expected non-nil dialer")
	}
}

func TestDialerFunc_EmptyHost_ReturnsNil(t *testing.T) {
	fn, err := DialerFunc("", 1080)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fn != nil {
		t.Fatal("expected nil function for empty host")
	}
}

func TestDialerFunc_NonEmptyHost_ReturnsFunction(t *testing.T) {
	fn, err := DialerFunc("127.0.0.1", 1080)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fn == nil {
		t.Fatal("expected non-nil function for non-empty host")
	}
}

func TestNewHTTPTransportWithDialer_NilDialer(t *testing.T) {
	transport := NewHTTPTransportWithDialer(nil)
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Errorf("expected MaxIdleConnsPerHost=10, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.DialContext != nil {
		t.Error("expected nil DialContext when dialer is nil")
	}
}

func TestNewHTTPTransportWithDialer_CustomDialer(t *testing.T) {
	var dialCount int32
	customDialer := func(network, addr string) (net.Conn, error) {
		atomic.AddInt32(&dialCount, 1)
		return nil, net.ErrClosed
	}
	transport := NewHTTPTransportWithDialer(customDialer)
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
	if transport.DialContext == nil {
		t.Fatal("expected DialContext to be set")
	}
	// 호출 시 customDialer가 실제로 invoke 되는지 확인
	_, _ = transport.DialContext(context.Background(), "tcp", "example.com:80")
	if atomic.LoadInt32(&dialCount) != 1 {
		t.Errorf("expected dialer to be called once, got %d", dialCount)
	}
}

func TestNewHTTPTransport_NoProxy(t *testing.T) {
	transport, err := NewHTTPTransport(config.SOCKSConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Errorf("expected MaxIdleConnsPerHost=10, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.DialContext != nil {
		t.Error("expected nil DialContext when no SOCKS proxy configured")
	}
}

func TestNewHTTPTransport_WithProxy(t *testing.T) {
	transport, err := NewHTTPTransport(config.SOCKSConfig{Host: "127.0.0.1", Port: 1080})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transport == nil {
		t.Fatal("expected non-nil transport")
	}
	if transport.DialContext == nil {
		t.Error("expected non-nil DialContext when SOCKS proxy configured")
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Errorf("expected MaxIdleConnsPerHost=10, got %d", transport.MaxIdleConnsPerHost)
	}
}
