package discovery

import (
	"context"
	"encoding/json"
	"fmt"
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
