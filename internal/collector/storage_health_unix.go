//go:build linux || darwin

package collector

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"resourceagent/internal/logger"
)

// smartctlPath is cached during Configure to avoid repeated LookPath calls.
var smartctlPathCached string
var smartctlChecked bool

// platformConfigure runs platform-specific setup during Configure.
func (c *StorageHealthCollector) platformConfigure() {
	if smartctlChecked {
		return
	}
	smartctlChecked = true
	path, err := exec.LookPath("smartctl")
	if err != nil {
		log := logger.WithComponent("collector")
		log.Warn().Str("collector", c.Name()).Msg("smartctl not found, StorageHealth will return empty data on Linux")
		smartctlPathCached = ""
		return
	}
	smartctlPathCached = path
}

func (c *StorageHealthCollector) collectHealthStatus(ctx context.Context) ([]StorageHealthDisk, error) {
	if runtime.GOOS == "darwin" {
		return nil, nil
	}

	if smartctlPathCached == "" {
		return nil, nil
	}

	devices, err := enumerateBlockDevices()
	if err != nil {
		return nil, err
	}

	var disks []StorageHealthDisk

	for _, dev := range devices {
		select {
		case <-ctx.Done():
			return disks, ctx.Err()
		default:
		}

		if len(c.includeDrives) > 0 && !c.shouldInclude(dev) {
			continue
		}

		devPath := "/dev/" + dev
		status, rawStatus := querySmartctlHealth(ctx, devPath)
		diskType := detectDiskType(dev)

		disks = append(disks, StorageHealthDisk{
			Name:      dev,
			Status:    status,
			RawStatus: rawStatus,
			DiskType:  diskType,
		})
	}

	return disks, nil
}

// enumerateBlockDevices scans /sys/block for physical block devices.
func enumerateBlockDevices() ([]string, error) {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, err
	}

	var devices []string
	for _, entry := range entries {
		name := entry.Name()
		if isVirtualDevice(name) {
			continue
		}
		devices = append(devices, name)
	}
	return devices, nil
}

func isVirtualDevice(name string) bool {
	prefixes := []string{"loop", "dm-", "ram", "sr", "fd", "zram"}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// querySmartctlHealth runs "smartctl -H /dev/sdX" and parses the result.
// smartctl exit code is a bitmask:
//
//	bit 0 (0x01): command parse error
//	bit 1 (0x02): device open failure
//	bit 2 (0x04): SMART command failed
//	bit 3 (0x08): DISK FAILING
//	bit 4-7: past errors/warnings (not immediate failure)
func querySmartctlHealth(ctx context.Context, devPath string) (string, string) {
	execCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, smartctlPathCached, "-H", devPath)
	output, err := cmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// exec itself failed (e.g. binary deleted since LookPath)
			return "UNKNOWN", ""
		}
	}

	// bit 1: device open failure (permission denied, device not found)
	if exitCode&0x02 != 0 {
		return "UNKNOWN", ""
	}

	// bit 3: DISK FAILING — immediate hardware failure
	if exitCode&0x08 != 0 {
		return "FAIL", "FAILING"
	}

	// Parse output for the health assessment line
	raw := parseSmartctlOutput(string(output))
	if raw != "" {
		return normalizeHealthStatus(raw), raw
	}

	return "UNKNOWN", ""
}

// parseSmartctlOutput extracts the health result from smartctl -H output.
// Looks for: "SMART overall-health self-assessment test result: PASSED"
func parseSmartctlOutput(output string) string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "self-assessment test result") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// detectDiskType determines HDD/SSD/NVMe from /sys/block metadata.
func detectDiskType(deviceName string) string {
	if strings.HasPrefix(deviceName, "nvme") {
		return "NVMe"
	}

	rotPath := filepath.Join("/sys/block", deviceName, "queue/rotational")
	data, err := os.ReadFile(rotPath)
	if err != nil {
		return ""
	}

	switch strings.TrimSpace(string(data)) {
	case "0":
		return "SSD"
	case "1":
		return "HDD"
	default:
		return ""
	}
}
