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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

// FetchExternalIP queries EARSInterfaceSrv to get the external IP visible from the proxy.
// It sends a POST request with category=ip header and expects a plain text IP response.
func FetchExternalIP(ctx context.Context, earsIfAddr string,
	dialFunc func(string, string) (net.Conn, error)) (string, error) {

	url := fmt.Sprintf("http://%s/EARS/Interface", earsIfAddr)

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
		return "", fmt.Errorf("failed to create FetchExternalIP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Header.Set()은 "Category"로 자동 대문자 변환하지만,
	// akka-http 서버가 소문자 "category"만 인식하므로 직접 설정
	req.Header["category"] = []string{"ip"}

	resp, err := client.Do(req)
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
