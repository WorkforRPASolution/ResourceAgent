package collector

import (
	"testing"

	"resourceagent/internal/config"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Parentheses removed
		{"Intel(R) HD Graphics 530_power", "IntelR_HD_Graphics_530_power"},
		// Spaces → _, hash removed
		{"CPU Core #1 Distance to TjMax", "CPU_Core_1_Distance_to_TjMax"},
		// Spaces → _
		{"Intel Core i7-6700 - Core Max", "Intel_Core_i7-6700_-_Core_Max"},
		// Spaces → _
		{"Samsung SSD 860 PRO 256GB_temperature", "Samsung_SSD_860_PRO_256GB_temperature"},
		// Drive letter unchanged
		{"C:", "C:"},
		// Already clean name unchanged
		{"total_used_pct", "total_used_pct"},
		// @ preserved (used in @system)
		{"@system", "@system"},
		// Dot preserved
		{"python3.11", "python3.11"},
		// Consecutive special chars → single _
		{"Fan  ##2", "Fan_2"},
		// Empty string
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShouldInclude_SanitizedMatch(t *testing.T) {
	// TemperatureCollector: config에 sanitized name을 써도 raw name과 매칭되어야 함
	tc := NewTemperatureCollector()
	tc.Configure(config.CollectorConfig{
		Enabled:      true,
		IncludeZones: []string{"CPU_Core_1"},
	})
	if !tc.shouldInclude("CPU Core #1") {
		t.Error("shouldInclude should match sanitized config 'CPU_Core_1' against raw 'CPU Core #1'")
	}

	// FanCollector
	fc := NewFanCollector()
	fc.Configure(config.CollectorConfig{
		Enabled:      true,
		IncludeZones: []string{"Fan_2"},
	})
	if !fc.shouldInclude("Fan  ##2") {
		t.Error("shouldInclude should match sanitized config 'Fan_2' against raw 'Fan  ##2'")
	}

	// GpuCollector
	gc := NewGpuCollector()
	gc.Configure(config.CollectorConfig{
		Enabled:      true,
		IncludeZones: []string{"IntelR_HD_Graphics_530"},
	})
	if !gc.shouldInclude("Intel(R) HD Graphics 530") {
		t.Error("shouldInclude should match sanitized config against raw GPU name")
	}

	// VoltageCollector
	vc := NewVoltageCollector()
	vc.Configure(config.CollectorConfig{
		Enabled:      true,
		IncludeZones: []string{"CPU_VCore"},
	})
	if !vc.shouldInclude("CPU VCore") {
		t.Error("shouldInclude should match sanitized config against raw voltage sensor name")
	}

	// MotherboardTempCollector
	mc := NewMotherboardTempCollector()
	mc.Configure(config.CollectorConfig{
		Enabled:      true,
		IncludeZones: []string{"CPU_Core_1_Distance_to_TjMax"},
	})
	if !mc.shouldInclude("CPU Core #1 Distance to TjMax") {
		t.Error("shouldInclude should match sanitized config against raw motherboard temp name")
	}
}

func TestShouldInclude_RawConfigStillWorks(t *testing.T) {
	// 기존 raw name config도 여전히 동작해야 함 (하위호환)
	tc := NewTemperatureCollector()
	tc.Configure(config.CollectorConfig{
		Enabled:      true,
		IncludeZones: []string{"CPU Core #1"},
	})
	if !tc.shouldInclude("CPU Core #1") {
		t.Error("shouldInclude should still match raw config 'CPU Core #1' against raw 'CPU Core #1'")
	}
}

func TestShouldInclude_NoMatch(t *testing.T) {
	tc := NewTemperatureCollector()
	tc.Configure(config.CollectorConfig{
		Enabled:      true,
		IncludeZones: []string{"CPU_Core_1"},
	})
	if tc.shouldInclude("GPU Core #1") {
		t.Error("shouldInclude should not match unrelated sensor")
	}
}
