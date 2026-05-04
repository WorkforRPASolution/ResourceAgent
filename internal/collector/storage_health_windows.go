//go:build windows

package collector

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yusufpapurcu/wmi"

	"resourceagent/internal/logger"
)

// Win32DiskDriveHealth maps WMI Win32_DiskDrive fields for health status.
type Win32DiskDriveHealth struct {
	Model         string
	Status        string
	Size          uint64
	MediaType     string
	InterfaceType string
}

// platformConfigure is a no-op on Windows (no smartctl caching needed).
func (c *StorageHealthCollector) platformConfigure() {}

func (c *StorageHealthCollector) collectHealthStatus(ctx context.Context) ([]StorageHealthDisk, error) {
	wmiDisks, err := queryWMIDiskDrive(ctx)
	if err != nil {
		return nil, err
	}

	var disks []StorageHealthDisk
	for _, d := range wmiDisks {
		select {
		case <-ctx.Done():
			return disks, ctx.Err()
		default:
		}

		// Skip removable/external media
		if isRemovableMedia(d.MediaType) {
			continue
		}

		name := strings.TrimSpace(d.Model)
		if name == "" {
			name = "Unknown Disk"
		}

		if len(c.includeDrives) > 0 && !c.shouldInclude(name) {
			continue
		}

		disks = append(disks, StorageHealthDisk{
			Name:      name,
			Status:    normalizeHealthStatus(d.Status),
			RawStatus: d.Status,
			DiskType:  classifyDiskType(d.InterfaceType, d.Model),
		})
	}

	return disks, nil
}

// wmiQueryFunc is the seam through which queryWMIDiskDrive reaches the WMI
// runtime. Tests swap it for a mock that simulates timeouts / hangs without
// requiring a real WMI service.
var wmiQueryFunc = func(query string, dst interface{}, connectServerArgs ...interface{}) error {
	return wmi.Query(query, dst, connectServerArgs...)
}

// wmiQueryStateData tracks one outstanding wmi.Query call across the package.
//
// Background: wmi.Query has no cancellation API. The previous implementation
// spawned a fresh goroutine per Collect (every 5 minutes by default) and
// raced it against ctx.Done() — but the goroutine itself stayed blocked in
// wmi.Query forever when WMI hung, leaking one goroutine per cycle.
//
// Strategy: option C from the memory-leak plan. Track an in-flight flag at
// package scope. The first caller spawns the worker; subsequent callers
// observe inFlight=true and return the most recent cached response (or an
// empty result) without spawning anything. Worst case is a single
// permanently-stuck goroutine — bounded, not accumulating.
type wmiQueryStateData struct {
	inFlight      atomic.Bool
	inFlightSince atomic.Int64 // unix-nanos of the in-flight start
	cacheMu       sync.RWMutex
	cache         *wmiCacheEntry
}

type wmiCacheEntry struct {
	disks     []Win32DiskDriveHealth
	err       error
	timestamp time.Time
}

var wmiQueryState wmiQueryStateData

const wmiQuery = "SELECT Model, Status, Size, MediaType, InterfaceType FROM Win32_DiskDrive"

// queryWMIDiskDrive runs the disk-drive WMI query with a bounded-leak
// protocol. See wmiQueryStateData for the mechanism.
func queryWMIDiskDrive(ctx context.Context) ([]Win32DiskDriveHealth, error) {
	log := logger.WithComponent("storage-health")

	// Fast path: if a prior worker is still running we never spawn a new one.
	// Either return cached data (preferred) or surface a synthetic error so
	// the caller can decide what to do. Either way no extra goroutine.
	if !wmiQueryState.inFlight.CompareAndSwap(false, true) {
		stuckFor := time.Since(time.Unix(0, wmiQueryState.inFlightSince.Load()))
		log.Warn().
			Dur("inflight_for", stuckFor).
			Msg("WMI_QUERY_INFLIGHT prior WMI query still running; serving cached response to avoid goroutine accumulation")

		if cached := loadWMICache(); cached != nil {
			return cached.disks, cached.err
		}
		return nil, fmt.Errorf("WMI query already in flight for %v and no cached response available", stuckFor)
	}

	wmiQueryState.inFlightSince.Store(time.Now().UnixNano())

	type result struct {
		disks []Win32DiskDriveHealth
		err   error
	}
	ch := make(chan result, 1)

	go func() {
		startedAt := time.Now()
		var dst []Win32DiskDriveHealth
		err := wmiQueryFunc(wmiQuery, &dst)

		// Always update the cache so future inflight-callers see fresh data
		// (even if it's an error — operators want to know).
		storeWMICache(&wmiCacheEntry{
			disks:     dst,
			err:       err,
			timestamp: time.Now(),
		})
		// Clear the inflight flag BEFORE sending on ch so a newly arriving
		// caller observes the cleared state on its next attempt.
		wmiQueryState.inFlight.Store(false)

		// If this worker took unusually long it means the previous timeout
		// path leaked us as a goroutine; surface that we recovered.
		if elapsed := time.Since(startedAt); elapsed > 5*time.Second {
			log.Info().
				Dur("elapsed", elapsed).
				Bool("query_error", err != nil).
				Msg("WMI_QUERY_RECOVERED long-running WMI query finally returned")
		}

		ch <- result{dst, err}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			log.Warn().Err(r.err).Msg("WMI_QUERY_ERROR wmi.Query returned error")
		}
		return r.disks, r.err

	case <-ctx.Done():
		// Caller's ctx (typically driven by Scheduler timeout) expired before
		// wmi.Query came back. The worker keeps running in the background:
		// it will update the cache and clear inFlight once it eventually
		// returns. Until then, future calls see inFlight=true and return
		// cached data instead of spawning more workers.
		stuckFor := time.Since(time.Unix(0, wmiQueryState.inFlightSince.Load()))
		log.Warn().
			Err(ctx.Err()).
			Dur("inflight_for", stuckFor).
			Msg("WMI_QUERY_TIMEOUT context expired; worker still running in background until WMI responds")
		return nil, fmt.Errorf("WMI query timed out: %w", ctx.Err())
	}
}

func loadWMICache() *wmiCacheEntry {
	wmiQueryState.cacheMu.RLock()
	defer wmiQueryState.cacheMu.RUnlock()
	return wmiQueryState.cache
}

func storeWMICache(entry *wmiCacheEntry) {
	wmiQueryState.cacheMu.Lock()
	wmiQueryState.cache = entry
	wmiQueryState.cacheMu.Unlock()
}

// resetWMIQueryStateForTest restores wmiQueryState to a clean state. Tests
// only — production callers must not invoke this. A function rather than a
// raw struct assignment so callers cannot accidentally bypass mutex acquisition.
func resetWMIQueryStateForTest() {
	wmiQueryState.inFlight.Store(false)
	wmiQueryState.inFlightSince.Store(0)
	wmiQueryState.cacheMu.Lock()
	wmiQueryState.cache = nil
	wmiQueryState.cacheMu.Unlock()
}

func isRemovableMedia(mediaType string) bool {
	lower := strings.ToLower(mediaType)
	return strings.Contains(lower, "removable") || strings.Contains(lower, "external")
}

// classifyDiskType attempts to determine HDD/SSD/NVMe from WMI fields.
// Win32_DiskDrive.MediaType cannot distinguish SSD from HDD; this is best-effort.
func classifyDiskType(interfaceType, model string) string {
	upper := strings.ToUpper(model)
	if strings.Contains(upper, "NVME") {
		return "NVMe"
	}
	if strings.Contains(upper, "SSD") {
		return "SSD"
	}
	return ""
}
