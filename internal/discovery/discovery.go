// Package discovery provides a client for the EARS ServiceDiscovery HTTP endpoint.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const httpTimeout = 5 * time.Second

// FetchServices calls the ServiceDiscovery HTTP endpoint and returns a service → address map.
// dialFunc is optional — if non-nil, used as custom dialer (e.g., SOCKS proxy).
func FetchServices(ctx context.Context, virtualIP string, port int, eqpIndex string,
	dialFunc func(network, addr string) (net.Conn, error)) (map[string]string, error) {

	url := fmt.Sprintf("http://%s:%d/EARS/Service/Multi?index=%s", virtualIP, port, eqpIndex)

	transport := &http.Transport{}
	if dialFunc != nil {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialFunc(network, addr)
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   httpTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create ServiceDiscovery request: %w", err)
	}

	resp, err := client.Do(req)
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

// GetKafkaRestAddress extracts the "kafkaRest" address from the services map.
func GetKafkaRestAddress(services map[string]string) (string, error) {
	addr, ok := services["kafkaRest"]
	if !ok || addr == "" {
		return "", fmt.Errorf("ServiceDiscovery response does not contain 'kafkaRest' key")
	}
	return addr, nil
}
