package collector

import (
	"runtime"
	"strings"
)

// ProcessMatcher handles platform-aware process name matching.
// Windows: case-insensitive (matches tasklist behavior)
// Linux: case-sensitive
type ProcessMatcher struct {
	watchSet        map[string]struct{} // for O(1) lookup
	caseInsensitive bool
}

// NewProcessMatcher creates a new ProcessMatcher from a list of process names.
func NewProcessMatcher(watchProcesses []string) *ProcessMatcher {
	m := &ProcessMatcher{
		watchSet:        make(map[string]struct{}),
		caseInsensitive: runtime.GOOS == "windows",
	}

	for _, name := range watchProcesses {
		if m.caseInsensitive {
			m.watchSet[strings.ToLower(name)] = struct{}{}
		} else {
			m.watchSet[name] = struct{}{}
		}
	}

	return m
}

// IsWatched returns true if the process name is in the watch list.
func (m *ProcessMatcher) IsWatched(processName string) bool {
	if len(m.watchSet) == 0 {
		return false
	}

	key := processName
	if m.caseInsensitive {
		key = strings.ToLower(processName)
	}

	_, exists := m.watchSet[key]
	return exists
}

// HasWatchList returns true if there are any processes in the watch list.
func (m *ProcessMatcher) HasWatchList() bool {
	return len(m.watchSet) > 0
}
