package metainfo

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"resourceagent/internal/config"
)

func TestBuildKey(t *testing.T) {
	got := BuildKey("ARS", "MODEL_A")
	want := "ResourceAgentMetaInfo:ARS-MODEL_A"
	if got != want {
		t.Errorf("BuildKey() = %q, want %q", got, want)
	}
}

func TestWriteVersion(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 0, Password: ""}
	WriteVersion(context.Background(), mr.Addr(), cfg, nil,
		"ARS", "M1", "EQP01", "1.3.0")

	key := "ResourceAgentMetaInfo:ARS-M1"
	got := mr.HGet(key, "EQP01")
	if got != "1.3.0" {
		t.Errorf("version = %q, want %q", got, "1.3.0")
	}
}

func TestWriteVersion_OverwritesExisting(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 0, Password: ""}
	WriteVersion(context.Background(), mr.Addr(), cfg, nil,
		"ARS", "M1", "EQP01", "1.2.0")
	WriteVersion(context.Background(), mr.Addr(), cfg, nil,
		"ARS", "M1", "EQP01", "1.3.0")

	got := mr.HGet("ResourceAgentMetaInfo:ARS-M1", "EQP01")
	if got != "1.3.0" {
		t.Errorf("version = %q, want %q", got, "1.3.0")
	}
}

func TestWriteVersion_MultipleEqpIds(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 0, Password: ""}
	WriteVersion(context.Background(), mr.Addr(), cfg, nil,
		"ARS", "M1", "EQP01", "1.3.0")
	WriteVersion(context.Background(), mr.Addr(), cfg, nil,
		"ARS", "M1", "EQP02", "1.4.0")

	key := "ResourceAgentMetaInfo:ARS-M1"
	v1 := mr.HGet(key, "EQP01")
	v2 := mr.HGet(key, "EQP02")

	if v1 != "1.3.0" {
		t.Errorf("EQP01 version = %q, want %q", v1, "1.3.0")
	}
	if v2 != "1.4.0" {
		t.Errorf("EQP02 version = %q, want %q", v2, "1.4.0")
	}
}
