package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/proxy"

	"resourceagent/internal/config"
)

// NewSOCKS5Dialer creates a SOCKS5 proxy dialer.
func NewSOCKS5Dialer(host string, port int) (proxy.Dialer, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer for %s: %w", addr, err)
	}
	return dialer, nil
}

// DialerFunc creates a dial function from SOCKS5 proxy settings.
// If host is empty, returns (nil, nil) — no proxy.
// The underlying SOCKS5 dialer is created once and captured by the closure,
// avoiding per-dial allocation.
func DialerFunc(host string, port int) (func(network, addr string) (net.Conn, error), error) {
	if host == "" {
		return nil, nil
	}
	dialer, err := NewSOCKS5Dialer(host, port)
	if err != nil {
		return nil, err
	}
	return func(network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}, nil
}

// NewHTTPTransportWithDialer creates an http.Transport with shared connection pool
// settings and an optional custom dialer (e.g. SOCKS5).
// If dialFunc is nil, the transport uses the default Go dialer.
func NewHTTPTransportWithDialer(dialFunc func(network, addr string) (net.Conn, error)) *http.Transport {
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	if dialFunc != nil {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialFunc(network, addr)
		}
	}
	return transport
}

// NewHTTPTransport creates an http.Transport with connection pool settings
// and optional SOCKS5 proxy support derived from socksCfg.
func NewHTTPTransport(socksCfg config.SOCKSConfig) (*http.Transport, error) {
	var dialFunc func(network, addr string) (net.Conn, error)
	if socksCfg.Host != "" && socksCfg.Port > 0 {
		df, err := DialerFunc(socksCfg.Host, socksCfg.Port)
		if err != nil {
			return nil, fmt.Errorf("SOCKS5 dialer: %w", err)
		}
		dialFunc = df
	}
	return NewHTTPTransportWithDialer(dialFunc), nil
}
