package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchServices_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		if !strings.Contains(r.URL.String(), "/EARS/Service/Multi") {
			t.Errorf("unexpected URL path: %s", r.URL.String())
		}
		if r.URL.Query().Get("index") != "42" {
			t.Errorf("expected index=42, got %s", r.URL.Query().Get("index"))
		}

		resp := map[string]string{
			"KafkaRest":            "192.168.0.100",
			"EARSInterfaceServer":  "192.168.0.200",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Extract host and port from test server
	addr := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(addr, ":")
	host := parts[0]
	port := 0
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	services, err := FetchServices(context.Background(), host, port, "42", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if services["KafkaRest"] != "192.168.0.100" {
		t.Errorf("expected kafkaRest=192.168.0.100, got %q", services["KafkaRest"])
	}
	if services["EARSInterfaceServer"] != "192.168.0.200" {
		t.Errorf("expected EARSInterfaceServer=192.168.0.200, got %q", services["EARSInterfaceServer"])
	}
}

func TestFetchServices_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	addr := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(addr, ":")
	host := parts[0]
	port := 0
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	_, err := FetchServices(context.Background(), host, port, "42", nil)
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestFetchServices_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	addr := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(addr, ":")
	host := parts[0]
	port := 0
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	_, err := FetchServices(context.Background(), host, port, "42", nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestFetchServices_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	addr := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(addr, ":")
	host := parts[0]
	port := 0
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	_, err := FetchServices(ctx, host, port, "42", nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestFetchServices_ConnectionRefused(t *testing.T) {
	_, err := FetchServices(context.Background(), "127.0.0.1", 1, "42", nil)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

func TestGetKafkaRestAddress_Success(t *testing.T) {
	services := map[string]string{
		"KafkaRest":            "192.168.0.100",
		"EARSInterfaceServer":  "192.168.0.200",
	}

	addr, err := GetKafkaRestAddress(services)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != "192.168.0.100" {
		t.Errorf("expected 192.168.0.100, got %q", addr)
	}
}

func TestGetKafkaRestAddress_MissingKey(t *testing.T) {
	services := map[string]string{
		"EARSInterfaceServer": "192.168.0.200",
	}

	_, err := GetKafkaRestAddress(services)
	if err == nil {
		t.Fatal("expected error for missing kafkaRest key, got nil")
	}
}

func TestGetKafkaRestAddress_EmptyValue(t *testing.T) {
	services := map[string]string{
		"KafkaRest": "",
	}

	_, err := GetKafkaRestAddress(services)
	if err == nil {
		t.Fatal("expected error for empty kafkaRest value, got nil")
	}
}

// --- GetEARSInterfaceSrvAddress tests ---

func TestGetEARSInterfaceSrvAddress_Success(t *testing.T) {
	services := map[string]string{
		"KafkaRest":        "192.168.0.100",
		"EARSInterfaceSrv": "192.168.0.200:8080",
	}

	addr, err := GetEARSInterfaceSrvAddress(services)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != "192.168.0.200:8080" {
		t.Errorf("expected 192.168.0.200:8080, got %q", addr)
	}
}

func TestGetEARSInterfaceSrvAddress_MissingKey(t *testing.T) {
	services := map[string]string{
		"KafkaRest": "192.168.0.100",
	}

	_, err := GetEARSInterfaceSrvAddress(services)
	if err == nil {
		t.Fatal("expected error for missing EARSInterfaceSrv key, got nil")
	}
}

func TestGetEARSInterfaceSrvAddress_EmptyValue(t *testing.T) {
	services := map[string]string{
		"EARSInterfaceSrv": "",
	}

	_, err := GetEARSInterfaceSrvAddress(services)
	if err == nil {
		t.Fatal("expected error for empty EARSInterfaceSrv value, got nil")
	}
}

// --- FetchExternalIP tests ---

func TestFetchExternalIP_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/EARS/Interface" {
			t.Errorf("expected /EARS/Interface path, got %s", r.URL.Path)
		}
		if r.Header.Get("category") != "ip" {
			t.Errorf("expected category=ip header, got %q", r.Header.Get("category"))
		}
		w.Write([]byte("  11.97.12.34  \n"))
	}))
	defer server.Close()

	addr := strings.TrimPrefix(server.URL, "http://")
	ip, err := FetchExternalIP(context.Background(), addr, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "11.97.12.34" {
		t.Errorf("expected 11.97.12.34, got %q", ip)
	}
}

func TestFetchExternalIP_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	addr := strings.TrimPrefix(server.URL, "http://")
	_, err := FetchExternalIP(context.Background(), addr, nil)
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestFetchExternalIP_WithDialFunc(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("10.0.0.1"))
	}))
	defer server.Close()

	dialCalled := false
	customDial := func(network, addr string) (net.Conn, error) {
		dialCalled = true
		return net.Dial(network, addr)
	}

	addr := strings.TrimPrefix(server.URL, "http://")
	ip, err := FetchExternalIP(context.Background(), addr, customDial)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %q", ip)
	}
	if !dialCalled {
		t.Error("expected custom dialFunc to be called")
	}
}
