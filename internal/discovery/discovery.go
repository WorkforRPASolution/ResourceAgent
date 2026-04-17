// Package discovery provides a client for the EARS ServiceDiscovery HTTP endpoint.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"resourceagent/internal/network"
)

const httpTimeout = 5 * time.Second

// Client holds a reusable http.Transport + http.Client for ServiceDiscovery requests.
// Sharing one Client across calls avoids per-call Transport allocation and lets the
// underlying connection pool keep-alive idle connections.
type Client struct {
	transport  *http.Transport
	httpClient *http.Client
	closeOnce  sync.Once
}

// NewClient constructs a Client. dialFunc is optional — if non-nil it is used as
// the underlying dialer (e.g. SOCKS proxy). Pass nil for direct dials.
func NewClient(dialFunc func(network, addr string) (net.Conn, error)) *Client {
	transport := network.NewHTTPTransportWithDialer(dialFunc)
	return &Client{
		transport: transport,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   httpTimeout,
		},
	}
}

// Close releases idle TCP connections held by the Client's transport.
// Safe to call multiple times.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.transport.CloseIdleConnections()
	})
	return nil
}

// FetchServices calls the ServiceDiscovery HTTP endpoint and returns a service → address map.
func (c *Client) FetchServices(ctx context.Context, virtualIP string, port int, eqpIndex string) (map[string]string, error) {
	url := fmt.Sprintf("http://%s:%d/EARS/Service/Multi?index=%s", virtualIP, port, eqpIndex)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create ServiceDiscovery request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ServiceDiscovery request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ServiceDiscovery returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var services map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&services); err != nil {
		return nil, fmt.Errorf("failed to parse ServiceDiscovery response: %w", err)
	}

	return services, nil
}

// FetchExternalIP queries EARSInterfaceSrv to get the external IP visible from the proxy.
// It sends a POST request with category=ip header and expects a plain text IP response.
func (c *Client) FetchExternalIP(ctx context.Context, earsIfAddr string) (string, error) {
	url := fmt.Sprintf("http://%s/EARS/Interface", earsIfAddr)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create FetchExternalIP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Header.Set()은 "Category"로 자동 대문자 변환하지만,
	// akka-http 서버가 소문자 "category"만 인식하므로 직접 설정
	req.Header["category"] = []string{"ip"}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("FetchExternalIP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("FetchExternalIP returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read FetchExternalIP response: %w", err)
	}

	return strings.TrimSpace(string(body)), nil
}

// GetKafkaRestAddress extracts the "kafkaRest" address from the services map.
func GetKafkaRestAddress(services map[string]string) (string, error) {
	addr, ok := services["KafkaRest"]
	if !ok || addr == "" {
		return "", fmt.Errorf("ServiceDiscovery response does not contain 'kafkaRest' key")
	}
	return addr, nil
}

// GetEARSInterfaceSrvAddress extracts the "EARSInterfaceSrv" address from the services map.
func GetEARSInterfaceSrvAddress(services map[string]string) (string, error) {
	addr, ok := services["EARSInterfaceSrv"]
	if !ok || addr == "" {
		return "", fmt.Errorf("ServiceDiscovery response does not contain 'EARSInterfaceSrv' key")
	}
	return addr, nil
}
