package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func parseHostPort(t *testing.T, serverURL string) (string, int) {
	t.Helper()
	addr := strings.TrimPrefix(serverURL, "http://")
	parts := strings.Split(addr, ":")
	host := parts[0]
	port := 0
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &port)
	}
	return host, port
}

func TestClient_FetchServices_Success(t *testing.T) {
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
			"KafkaRest":           "192.168.0.100",
			"EARSInterfaceServer": "192.168.0.200",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	host, port := parseHostPort(t, server.URL)

	client := NewClient(nil)
	defer client.Close()

	services, err := client.FetchServices(context.Background(), host, port, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if services["KafkaRest"] != "192.168.0.100" {
		t.Errorf("expected kafkaRest=192.168.0.100, got %q", services["KafkaRest"])
	}
}

func TestClient_FetchServices_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	host, port := parseHostPort(t, server.URL)

	client := NewClient(nil)
	defer client.Close()

	_, err := client.FetchServices(context.Background(), host, port, "42")
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestClient_FetchServices_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	host, port := parseHostPort(t, server.URL)

	client := NewClient(nil)
	defer client.Close()

	_, err := client.FetchServices(context.Background(), host, port, "42")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestClient_FetchServices_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	host, port := parseHostPort(t, server.URL)

	client := NewClient(nil)
	defer client.Close()

	_, err := client.FetchServices(ctx, host, port, "42")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestClient_FetchServices_ConnectionRefused(t *testing.T) {
	client := NewClient(nil)
	defer client.Close()

	_, err := client.FetchServices(context.Background(), "127.0.0.1", 1, "42")
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

// 핵심: Client는 transport를 호출 간 재사용해야 함 (TCP 커넥션 keep-alive 발생)
func TestClient_ReusesTransportAcrossCalls(t *testing.T) {
	var stateMu sync.Mutex
	connStates := make(map[string][]http.ConnState)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]string{"KafkaRest": "1.2.3.4"}
		json.NewEncoder(w).Encode(resp)
	}))
	server.Config.ConnState = func(c net.Conn, state http.ConnState) {
		stateMu.Lock()
		key := c.RemoteAddr().String()
		connStates[key] = append(connStates[key], state)
		stateMu.Unlock()
	}
	server.Start()
	defer server.Close()

	host, port := parseHostPort(t, server.URL)

	client := NewClient(nil)
	defer client.Close()

	for i := 0; i < 3; i++ {
		_, err := client.FetchServices(context.Background(), host, port, "42")
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}

	// 같은 client를 재사용하면 keep-alive로 connection 1개만 생성되어야 함
	stateMu.Lock()
	defer stateMu.Unlock()
	if len(connStates) > 1 {
		t.Errorf("expected at most 1 unique connection (keep-alive reuse), got %d: %v", len(connStates), connStates)
	}
}

func TestClient_Close_Idempotent(t *testing.T) {
	client := NewClient(nil)

	if err := client.Close(); err != nil {
		t.Fatalf("first Close error: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("second Close panicked: %v", r)
		}
	}()
	if err := client.Close(); err != nil {
		t.Fatalf("second Close error: %v", err)
	}
}

func TestGetKafkaRestAddress_Success(t *testing.T) {
	services := map[string]string{
		"KafkaRest":           "192.168.0.100",
		"EARSInterfaceServer": "192.168.0.200",
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

func TestClient_FetchExternalIP_Success(t *testing.T) {
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

	client := NewClient(nil)
	defer client.Close()

	ip, err := client.FetchExternalIP(context.Background(), addr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "11.97.12.34" {
		t.Errorf("expected 11.97.12.34, got %q", ip)
	}
}

func TestClient_FetchExternalIP_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	addr := strings.TrimPrefix(server.URL, "http://")

	client := NewClient(nil)
	defer client.Close()

	_, err := client.FetchExternalIP(context.Background(), addr)
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestClient_FetchExternalIP_WithDialFunc(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("10.0.0.1"))
	}))
	defer server.Close()

	var dialCount int32
	customDial := func(network, addr string) (net.Conn, error) {
		atomic.AddInt32(&dialCount, 1)
		return net.Dial(network, addr)
	}

	addr := strings.TrimPrefix(server.URL, "http://")

	client := NewClient(customDial)
	defer client.Close()

	ip, err := client.FetchExternalIP(context.Background(), addr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %q", ip)
	}
	if atomic.LoadInt32(&dialCount) == 0 {
		t.Error("expected custom dialFunc to be called at least once")
	}
}
