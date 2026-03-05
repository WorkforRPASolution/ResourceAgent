package eqpinfo

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"resourceagent/internal/config"
)

// --- ParseEqpInfoValue tests ---

func TestParseEqpInfoValue_ValidSixSegments(t *testing.T) {
	info, err := ParseEqpInfoValue("PROCESS:MODEL:EQP001:LINE1:LineDesc1:42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil EqpInfo")
	}
	if info.Process != "PROCESS" {
		t.Errorf("expected Process=PROCESS, got %s", info.Process)
	}
	if info.EqpModel != "MODEL" {
		t.Errorf("expected EqpModel=MODEL, got %s", info.EqpModel)
	}
	if info.EqpID != "EQP001" {
		t.Errorf("expected EqpID=EQP001, got %s", info.EqpID)
	}
	if info.Line != "LINE1" {
		t.Errorf("expected Line=LINE1, got %s", info.Line)
	}
	if info.LineDesc != "LineDesc1" {
		t.Errorf("expected LineDesc=LineDesc1, got %s", info.LineDesc)
	}
	if info.Index != "42" {
		t.Errorf("expected Index=42, got %s", info.Index)
	}
}

func TestParseEqpInfoValue_ValidWithEmptySegments(t *testing.T) {
	info, err := ParseEqpInfoValue(":::EQP002:::")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil EqpInfo")
	}
	if info.Process != "" {
		t.Errorf("expected Process='', got %q", info.Process)
	}
	if info.EqpModel != "" {
		t.Errorf("expected EqpModel='', got %q", info.EqpModel)
	}
	if info.EqpID != "" {
		t.Errorf("expected EqpID='', got %q", info.EqpID)
	}
	if info.Line != "EQP002" {
		t.Errorf("expected Line=EQP002, got %s", info.Line)
	}
	if info.LineDesc != "" {
		t.Errorf("expected LineDesc='', got %q", info.LineDesc)
	}
	if info.Index != "" {
		t.Errorf("expected Index='', got %q", info.Index)
	}
}

func TestParseEqpInfoValue_MoreThanSixSegments(t *testing.T) {
	info, err := ParseEqpInfoValue("a:b:c:d:e:f:g")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil EqpInfo")
	}
	if info.Process != "a" {
		t.Errorf("expected Process=a, got %s", info.Process)
	}
	if info.EqpModel != "b" {
		t.Errorf("expected EqpModel=b, got %s", info.EqpModel)
	}
	if info.EqpID != "c" {
		t.Errorf("expected EqpID=c, got %s", info.EqpID)
	}
	if info.Line != "d" {
		t.Errorf("expected Line=d, got %s", info.Line)
	}
	if info.LineDesc != "e" {
		t.Errorf("expected LineDesc=e, got %s", info.LineDesc)
	}
	if info.Index != "f" {
		t.Errorf("expected Index=f, got %s", info.Index)
	}
}

func TestParseEqpInfoValue_TooFewSegments(t *testing.T) {
	_, err := ParseEqpInfoValue("a:b:c")
	if err == nil {
		t.Fatal("expected error for too few segments, got nil")
	}
}

func TestParseEqpInfoValue_EmptyString(t *testing.T) {
	_, err := ParseEqpInfoValue("")
	if err == nil {
		t.Fatal("expected error for empty string, got nil")
	}
}

func TestParseEqpInfoValue_FiveSegments(t *testing.T) {
	_, err := ParseEqpInfoValue("a:b:c:d:e")
	if err == nil {
		t.Fatal("expected error for five segments, got nil")
	}
}

// --- FetchEqpInfo tests (using miniredis) ---

func TestFetchEqpInfo_Success(t *testing.T) {
	mr := miniredis.RunT(t)

	// Seed data in DB 10
	mr.Select(10)
	mr.HSet("EQP_INFO", "192.168.1.100:10.0.0.1", "PROCESS:MODEL:EQP001:LINE1:Desc:42")

	cfg := config.RedisConfig{
		Port:    6379,
		DB:      10,
	}

	info, err := FetchEqpInfo(context.Background(), mr.Addr(), cfg, nil, "192.168.1.100", "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil EqpInfo")
	}
	if info.Process != "PROCESS" {
		t.Errorf("expected Process=PROCESS, got %s", info.Process)
	}
	if info.EqpModel != "MODEL" {
		t.Errorf("expected EqpModel=MODEL, got %s", info.EqpModel)
	}
	if info.EqpID != "EQP001" {
		t.Errorf("expected EqpID=EQP001, got %s", info.EqpID)
	}
	if info.Line != "LINE1" {
		t.Errorf("expected Line=LINE1, got %s", info.Line)
	}
	if info.LineDesc != "Desc" {
		t.Errorf("expected LineDesc=Desc, got %s", info.LineDesc)
	}
	if info.Index != "42" {
		t.Errorf("expected Index=42, got %s", info.Index)
	}
}

func TestFetchEqpInfo_KeyNotFound(t *testing.T) {
	mr := miniredis.RunT(t)

	// No data seeded - Redis is running but key doesn't exist
	cfg := config.RedisConfig{
		Port:    6379,
		DB:      10,
	}

	info, err := FetchEqpInfo(context.Background(), mr.Addr(), cfg, nil, "192.168.1.100", "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil info for missing key, got %+v", info)
	}
}

func TestFetchEqpInfo_InvalidValue(t *testing.T) {
	mr := miniredis.RunT(t)

	// Seed malformed data (only 2 segments)
	mr.Select(10)
	mr.HSet("EQP_INFO", "192.168.1.100:10.0.0.1", "bad:data")

	cfg := config.RedisConfig{
		Port:    6379,
		DB:      10,
	}

	_, err := FetchEqpInfo(context.Background(), mr.Addr(), cfg, nil, "192.168.1.100", "10.0.0.1")
	if err == nil {
		t.Fatal("expected error for malformed value, got nil")
	}
}

// --- FetchEqpInfoMulti tests ---

func TestFetchEqpInfoMulti_FirstMatch(t *testing.T) {
	mr := miniredis.RunT(t)
	mr.Select(10)
	mr.HSet("EQP_INFO", "11.97.12.34:192.168.10.3", "PROC:MDL:EQP001:LN:DESC:1")

	cfg := config.RedisConfig{DB: 10}
	candidates := []IPCandidate{
		{IPAddr: "11.97.12.34", IPAddrLocal: "192.168.10.3"},
		{IPAddr: "11.97.12.34", IPAddrLocal: "192.168.20.5"},
	}

	info, matched, err := FetchEqpInfoMulti(context.Background(), mr.Addr(), cfg, nil, candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil EqpInfo")
	}
	if info.EqpID != "EQP001" {
		t.Errorf("expected EqpID=EQP001, got %s", info.EqpID)
	}
	if matched.IPAddr != "11.97.12.34" || matched.IPAddrLocal != "192.168.10.3" {
		t.Errorf("unexpected matched candidate: %+v", matched)
	}
}

func TestFetchEqpInfoMulti_SecondMatch(t *testing.T) {
	mr := miniredis.RunT(t)
	mr.Select(10)
	// First candidate not in Redis, second is
	mr.HSet("EQP_INFO", "11.97.12.34:192.168.20.5", "PROC:MDL:EQP002:LN:DESC:2")

	cfg := config.RedisConfig{DB: 10}
	candidates := []IPCandidate{
		{IPAddr: "11.97.12.34", IPAddrLocal: "192.168.10.3"},
		{IPAddr: "11.97.12.34", IPAddrLocal: "192.168.20.5"},
	}

	info, matched, err := FetchEqpInfoMulti(context.Background(), mr.Addr(), cfg, nil, candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil EqpInfo")
	}
	if info.EqpID != "EQP002" {
		t.Errorf("expected EqpID=EQP002, got %s", info.EqpID)
	}
	if matched.IPAddrLocal != "192.168.20.5" {
		t.Errorf("expected matched IPAddrLocal=192.168.20.5, got %s", matched.IPAddrLocal)
	}
}

func TestFetchEqpInfoMulti_NoMatch(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{DB: 10}
	candidates := []IPCandidate{
		{IPAddr: "11.97.12.34", IPAddrLocal: "192.168.10.3"},
	}

	info, matched, err := FetchEqpInfoMulti(context.Background(), mr.Addr(), cfg, nil, candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil info for no match, got %+v", info)
	}
	if matched != nil {
		t.Errorf("expected nil matched for no match, got %+v", matched)
	}
}

func TestFetchEqpInfoMulti_EmptyCandidates(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{DB: 10}

	info, matched, err := FetchEqpInfoMulti(context.Background(), mr.Addr(), cfg, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil info for empty candidates, got %+v", info)
	}
	if matched != nil {
		t.Errorf("expected nil matched for empty candidates, got %+v", matched)
	}
}

func TestFetchEqpInfo_WithUnderscoreIPAddrLocal(t *testing.T) {
	mr := miniredis.RunT(t)

	// The underscore "_" is used as ipAddrLocal when local IP is unknown
	mr.Select(10)
	mr.HSet("EQP_INFO", "192.168.1.1:_", "PROC:MDL:EQP999:LN:DESC:0")

	cfg := config.RedisConfig{
		Port:    6379,
		DB:      10,
	}

	info, err := FetchEqpInfo(context.Background(), mr.Addr(), cfg, nil, "192.168.1.1", "_")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil EqpInfo")
	}
	if info.EqpID != "EQP999" {
		t.Errorf("expected EqpID=EQP999, got %s", info.EqpID)
	}
	if info.Index != "0" {
		t.Errorf("expected Index=0, got %s", info.Index)
	}
}
