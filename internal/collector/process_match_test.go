package collector

import (
	"runtime"
	"testing"
)

func TestProcessMatcher_IsWatched(t *testing.T) {
	tests := []struct {
		name           string
		watchProcesses []string
		processName    string
		want           bool
	}{
		{
			name:           "exact match",
			watchProcesses: []string{"chrome.exe", "java.exe"},
			processName:    "chrome.exe",
			want:           true,
		},
		{
			name:           "not in list",
			watchProcesses: []string{"chrome.exe", "java.exe"},
			processName:    "notepad.exe",
			want:           false,
		},
		{
			name:           "empty watch list",
			watchProcesses: []string{},
			processName:    "chrome.exe",
			want:           false,
		},
		{
			name:           "nil watch list",
			watchProcesses: nil,
			processName:    "chrome.exe",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewProcessMatcher(tt.watchProcesses)
			got := m.IsWatched(tt.processName)
			if got != tt.want {
				t.Errorf("IsWatched(%q) = %v, want %v", tt.processName, got, tt.want)
			}
		})
	}
}

func TestProcessMatcher_CaseSensitivity(t *testing.T) {
	m := NewProcessMatcher([]string{"Chrome.exe", "JAVA.exe"})

	if runtime.GOOS == "windows" {
		// Windows: case-insensitive
		tests := []struct {
			name string
			want bool
		}{
			{"chrome.exe", true},
			{"CHROME.EXE", true},
			{"Chrome.exe", true},
			{"java.exe", true},
			{"Java.Exe", true},
		}
		for _, tt := range tests {
			if got := m.IsWatched(tt.name); got != tt.want {
				t.Errorf("Windows: IsWatched(%q) = %v, want %v", tt.name, got, tt.want)
			}
		}
	} else {
		// Linux/macOS: case-sensitive
		tests := []struct {
			name string
			want bool
		}{
			{"Chrome.exe", true},  // exact match
			{"JAVA.exe", true},    // exact match
			{"chrome.exe", false}, // different case
			{"java.exe", false},   // different case
		}
		for _, tt := range tests {
			if got := m.IsWatched(tt.name); got != tt.want {
				t.Errorf("Linux/macOS: IsWatched(%q) = %v, want %v", tt.name, got, tt.want)
			}
		}
	}
}

func TestProcessMatcher_HasWatchList(t *testing.T) {
	tests := []struct {
		name           string
		watchProcesses []string
		want           bool
	}{
		{
			name:           "with processes",
			watchProcesses: []string{"chrome.exe"},
			want:           true,
		},
		{
			name:           "empty list",
			watchProcesses: []string{},
			want:           false,
		},
		{
			name:           "nil list",
			watchProcesses: nil,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewProcessMatcher(tt.watchProcesses)
			if got := m.HasWatchList(); got != tt.want {
				t.Errorf("HasWatchList() = %v, want %v", got, tt.want)
			}
		})
	}
}
