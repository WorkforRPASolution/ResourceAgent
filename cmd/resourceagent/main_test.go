package main

import (
	"reflect"
	"testing"

	"resourceagent/internal/eqpinfo"
)

func TestBuildCandidatesNoProxy_BothEmpty(t *testing.T) {
	got := buildCandidatesNoProxy("", nil)
	if len(got) != 0 {
		t.Errorf("expected empty candidates, got %v", got)
	}
}

func TestBuildCandidatesNoProxy_DetectedOnly(t *testing.T) {
	got := buildCandidatesNoProxy("11.97.211.92", nil)
	want := []eqpinfo.IPCandidate{{IPAddr: "11.97.211.92", IPAddrLocal: "_"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildCandidatesNoProxy_AllIPsOnly_DetectedEmpty(t *testing.T) {
	got := buildCandidatesNoProxy("", []string{"11.97.211.92", "11.97.211.80", "192.168.30.39"})
	want := []eqpinfo.IPCandidate{
		{IPAddr: "11.97.211.92", IPAddrLocal: "_"},
		{IPAddr: "11.97.211.80", IPAddrLocal: "_"},
		{IPAddr: "192.168.30.39", IPAddrLocal: "_"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// 핵심: 사용자 시나리오 — detectedIP는 .92인데 EQP_INFO는 .80에 등록됨.
// detectedIP를 첫 번째로 시도하고, 미스일 경우 NIC의 다른 IP들을 fallback으로 시도해야 함.
func TestBuildCandidatesNoProxy_DetectedFirstThenFallback(t *testing.T) {
	got := buildCandidatesNoProxy("11.97.211.92", []string{"11.97.211.92", "11.97.211.80", "192.168.30.39"})
	want := []eqpinfo.IPCandidate{
		{IPAddr: "11.97.211.92", IPAddrLocal: "_"},  // detectedIP가 항상 첫 번째
		{IPAddr: "11.97.211.80", IPAddrLocal: "_"},  // fallback (다른 NIC)
		{IPAddr: "192.168.30.39", IPAddrLocal: "_"}, // fallback (다른 NIC)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// detectedIP가 allIPs에 들어 있을 때 중복 후보를 만들면 안 됨.
func TestBuildCandidatesNoProxy_DedupesDetectedFromAllIPs(t *testing.T) {
	got := buildCandidatesNoProxy("11.97.211.80", []string{"11.97.211.92", "11.97.211.80", "192.168.30.39"})
	want := []eqpinfo.IPCandidate{
		{IPAddr: "11.97.211.80", IPAddrLocal: "_"},
		{IPAddr: "11.97.211.92", IPAddrLocal: "_"},
		{IPAddr: "192.168.30.39", IPAddrLocal: "_"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// 중복 검증
	seen := make(map[string]int)
	for _, c := range got {
		seen[c.IPAddr]++
	}
	for ip, count := range seen {
		if count > 1 {
			t.Errorf("IP %s appears %d times in candidates", ip, count)
		}
	}
}

// detectedIP가 allIPs에 없는 외부 주소인 경우 (이론적이지만 안전성 보장).
func TestBuildCandidatesNoProxy_DetectedNotInAllIPs(t *testing.T) {
	got := buildCandidatesNoProxy("10.0.0.1", []string{"11.97.211.92", "11.97.211.80"})
	want := []eqpinfo.IPCandidate{
		{IPAddr: "10.0.0.1", IPAddrLocal: "_"},
		{IPAddr: "11.97.211.92", IPAddrLocal: "_"},
		{IPAddr: "11.97.211.80", IPAddrLocal: "_"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// allIPs 내부에 중복이 있는 경우 (방어적 검증).
func TestBuildCandidatesNoProxy_DedupesAllIPsInternalDuplicates(t *testing.T) {
	got := buildCandidatesNoProxy("", []string{"11.97.211.92", "11.97.211.92", "11.97.211.80"})
	want := []eqpinfo.IPCandidate{
		{IPAddr: "11.97.211.92", IPAddrLocal: "_"},
		{IPAddr: "11.97.211.80", IPAddrLocal: "_"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
