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
// If host is empty, returns nil (no proxy).
func DialerFunc(host string, port int) func(network, addr string) (net.Conn, error) {
	if host == "" {
		return nil
	}
	return func(network, addr string) (net.Conn, error) {
		dialer, err := NewSOCKS5Dialer(host, port)
		if err != nil {
			return nil, err
		}
		return dialer.Dial(network, addr)
	}
}

// NewHTTPTransport creates an http.Transport with connection pool settings
// and optional SOCKS5 proxy support.
func NewHTTPTransport(socksCfg config.SOCKSConfig) (*http.Transport, error) {
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	if socksCfg.Host != "" && socksCfg.Port > 0 {
		dialer, err := NewSOCKS5Dialer(socksCfg.Host, socksCfg.Port)
		if err != nil {
			return nil, fmt.Errorf("SOCKS5 dialer: %w", err)
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	}

	return transport, nil
}
