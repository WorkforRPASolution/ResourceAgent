//go:build windows

package collector

import (
	"context"
	"fmt"
	"strings"

	"github.com/yusufpapurcu/wmi"
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

// queryWMIDiskDrive runs WMI query with a goroutine timeout wrapper.
// wmi.Query() does not accept context.Context, so we wrap it to prevent hanging.
func queryWMIDiskDrive(ctx context.Context) ([]Win32DiskDriveHealth, error) {
	type result struct {
		disks []Win32DiskDriveHealth
		err   error
	}
	ch := make(chan result, 1)

	go func() {
		var dst []Win32DiskDriveHealth
		err := wmi.Query("SELECT Model, Status, Size, MediaType, InterfaceType FROM Win32_DiskDrive", &dst)
		ch <- result{dst, err}
	}()

	select {
	case r := <-ch:
		return r.disks, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("WMI query timed out: %w", ctx.Err())
	}
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
