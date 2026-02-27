package collector

import (
	"context"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/disk"

	"resourceagent/internal/config"
)

// DiskCollector collects disk usage and I/O metrics.
type DiskCollector struct {
	BaseCollector
	disks []string // Specific disks to monitor; empty means all
}

// NewDiskCollector creates a new disk collector.
func NewDiskCollector() *DiskCollector {
	return &DiskCollector{
		BaseCollector: NewBaseCollector("Disk"),
	}
}

// DefaultConfig returns the default CollectorConfig for the disk collector.
func (c *DiskCollector) DefaultConfig() config.CollectorConfig {
	cfg := c.BaseCollector.DefaultConfig()
	cfg.Interval = 30 * time.Second
	return cfg
}

// Configure applies the configuration to the collector.
func (c *DiskCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	c.disks = cfg.Disks
	return nil
}

// Collect gathers disk metrics.
func (c *DiskCollector) Collect(ctx context.Context) (*MetricData, error) {
	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil, err
	}

	// Get I/O counters for all disks
	ioCounters, _ := disk.IOCountersWithContext(ctx) // Ignore error, I/O stats may not be available

	var diskPartitions []DiskPartition

	for _, p := range partitions {
		// Skip if specific disks are configured and this one isn't in the list
		if len(c.disks) > 0 && !c.shouldInclude(p.Device, p.Mountpoint) {
			continue
		}

		// Skip pseudo filesystems
		if c.isPseudoFS(p.Fstype) {
			continue
		}

		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			continue // Skip partitions we can't read
		}

		// Skip partitions with zero total bytes (e.g., empty CD-ROM drives)
		if c.shouldSkipPartition(usage.Total) {
			continue
		}

		partition := DiskPartition{
			Device:        p.Device,
			Mountpoint:    p.Mountpoint,
			FSType:        p.Fstype,
			TotalBytes:    usage.Total,
			UsedBytes:     usage.Used,
			FreeBytes:     usage.Free,
			UsagePercent:  usage.UsedPercent,
			InodesTotal:   usage.InodesTotal,
			InodesUsed:    usage.InodesUsed,
			InodesFree:    usage.InodesFree,
			InodesPercent: usage.InodesUsedPercent,
		}

		// Add I/O stats if available
		if ioCounters != nil {
			deviceName := c.getDeviceName(p.Device)
			if io, ok := ioCounters[deviceName]; ok {
				partition.ReadBytes = io.ReadBytes
				partition.WriteBytes = io.WriteBytes
				partition.ReadCount = io.ReadCount
				partition.WriteCount = io.WriteCount
				partition.ReadTime = io.ReadTime
				partition.WriteTime = io.WriteTime
			}
		}

		diskPartitions = append(diskPartitions, partition)
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data:      DiskData{Partitions: diskPartitions},
	}, nil
}

func (c *DiskCollector) shouldInclude(device, mountpoint string) bool {
	for _, d := range c.disks {
		if d == device || d == mountpoint {
			return true
		}
	}
	return false
}

func (c *DiskCollector) isPseudoFS(fstype string) bool {
	pseudoFS := []string{
		"sysfs", "proc", "devtmpfs", "devpts", "tmpfs", "securityfs",
		"cgroup", "cgroup2", "pstore", "debugfs", "hugetlbfs", "mqueue",
		"fusectl", "configfs", "autofs", "binfmt_misc", "fuse.gvfsd-fuse",
		"overlay", "squashfs",
		"cdfs", "udf", // CD-ROM / DVD
	}
	fsLower := strings.ToLower(fstype)
	for _, pfs := range pseudoFS {
		if fsLower == pfs {
			return true
		}
	}
	return false
}

// shouldSkipPartition returns true for partitions with zero total bytes (e.g., empty CD-ROM drives).
func (c *DiskCollector) shouldSkipPartition(totalBytes uint64) bool {
	return totalBytes == 0
}

func (c *DiskCollector) getDeviceName(device string) string {
	// Extract just the device name from the full path
	// e.g., /dev/sda1 -> sda1
	parts := strings.Split(device, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return device
}
