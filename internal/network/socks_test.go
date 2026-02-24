package network

import (
	"testing"
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
	fn := DialerFunc("", 1080)
	if fn != nil {
		t.Fatal("expected nil function for empty host")
	}
}

func TestDialerFunc_NonEmptyHost_ReturnsFunction(t *testing.T) {
	fn := DialerFunc("127.0.0.1", 1080)
	if fn == nil {
		t.Fatal("expected non-nil function for non-empty host")
	}
}
