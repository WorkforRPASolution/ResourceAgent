package network

import (
	"fmt"
	"net"

	"golang.org/x/net/proxy"
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
