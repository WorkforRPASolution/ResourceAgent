package collector

import "time"

// MetricData is the common wrapper for all collected metrics.
type MetricData struct {
	Type      string            `json:"type"`
	Timestamp time.Time         `json:"timestamp"`
	AgentID  string      `json:"agent_id"`
	Hostname string      `json:"hostname"`
	Data     interface{} `json:"data"`
}

// CPUData contains overall CPU usage metrics.
type CPUData struct {
	UsagePercent float64   `json:"usage_percent"`
	User         float64   `json:"user"`
	System       float64   `json:"system"`
	Idle         float64   `json:"idle"`
	IOWait       float64   `json:"iowait,omitempty"`
	Irq          float64   `json:"irq,omitempty"`
	SoftIrq      float64   `json:"softirq,omitempty"`
	Steal        float64   `json:"steal,omitempty"`
	Guest        float64   `json:"guest,omitempty"`
	CoreCount    int       `json:"core_count"`
	PerCore      []float64 `json:"per_core,omitempty"`
}

// MemoryData contains memory usage metrics.
type MemoryData struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
	SwapTotalBytes uint64  `json:"swap_total_bytes"`
	SwapUsedBytes  uint64  `json:"swap_used_bytes"`
	SwapFreeBytes  uint64  `json:"swap_free_bytes"`
	SwapPercent    float64 `json:"swap_percent"`
	Cached         uint64  `json:"cached,omitempty"`
	Buffers        uint64  `json:"buffers,omitempty"`
}

// DiskData contains disk usage and I/O metrics.
type DiskData struct {
	Partitions []DiskPartition `json:"partitions"`
}

// DiskPartition contains metrics for a single disk partition.
type DiskPartition struct {
	Device        string  `json:"device"`
	Mountpoint    string  `json:"mountpoint"`
	FSType        string  `json:"fs_type"`
	TotalBytes    uint64  `json:"total_bytes"`
	UsedBytes     uint64  `json:"used_bytes"`
	FreeBytes     uint64  `json:"free_bytes"`
	UsagePercent  float64 `json:"usage_percent"`
	InodesTotal   uint64  `json:"inodes_total,omitempty"`
	InodesUsed    uint64  `json:"inodes_used,omitempty"`
	InodesFree    uint64  `json:"inodes_free,omitempty"`
	InodesPercent float64 `json:"inodes_percent,omitempty"`
	ReadBytes     uint64  `json:"read_bytes,omitempty"`
	WriteBytes    uint64  `json:"write_bytes,omitempty"`
	ReadCount     uint64  `json:"read_count,omitempty"`
	WriteCount    uint64  `json:"write_count,omitempty"`
	ReadTime      uint64  `json:"read_time_ms,omitempty"`
	WriteTime     uint64  `json:"write_time_ms,omitempty"`
}

// NetworkData contains network interface metrics.
type NetworkData struct {
	Interfaces       []NetworkInterface `json:"interfaces"`
	TCPInboundCount  int                `json:"tcp_inbound_count"`
	TCPOutboundCount int                `json:"tcp_outbound_count"`
}

// NetworkInterface contains metrics for a single network interface.
type NetworkInterface struct {
	Name          string  `json:"name"`
	BytesSent     uint64  `json:"bytes_sent"`
	BytesRecv     uint64  `json:"bytes_recv"`
	PacketsSent   uint64  `json:"packets_sent"`
	PacketsRecv   uint64  `json:"packets_recv"`
	ErrorsIn      uint64  `json:"errors_in"`
	ErrorsOut     uint64  `json:"errors_out"`
	DropsIn       uint64  `json:"drops_in"`
	DropsOut      uint64  `json:"drops_out"`
	BytesSentRate float64 `json:"bytes_sent_rate,omitempty"`
	BytesRecvRate float64 `json:"bytes_recv_rate,omitempty"`
}

// TemperatureData contains system temperature metrics.
type TemperatureData struct {
	Sensors []TemperatureSensor `json:"sensors"`
}

// TemperatureSensor contains metrics from a single temperature sensor.
type TemperatureSensor struct {
	Name        string  `json:"name"`
	Temperature float64 `json:"temperature_celsius"`
	High        float64 `json:"high_celsius,omitempty"`
	Critical    float64 `json:"critical_celsius,omitempty"`
}

// FanData contains system fan speed metrics.
type FanData struct {
	Sensors []FanSensor `json:"sensors"`
}

// FanSensor contains metrics from a single fan sensor.
type FanSensor struct {
	Name string  `json:"name"`
	RPM  float64 `json:"rpm"`
}

// ProcessCPUData contains per-process CPU usage metrics.
type ProcessCPUData struct {
	Processes []ProcessCPU `json:"processes"`
}

// ProcessCPU contains CPU metrics for a single process.
type ProcessCPU struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name"`
	Username   string  `json:"username,omitempty"`
	CPUPercent float64 `json:"cpu_percent"`
	CreateTime int64   `json:"create_time,omitempty"`
	Watched    bool    `json:"watched,omitempty"`
}

// ProcessMemoryData contains per-process memory usage metrics.
type ProcessMemoryData struct {
	Processes []ProcessMemory `json:"processes"`
}

// ProcessMemory contains memory metrics for a single process.
type ProcessMemory struct {
	PID           int32   `json:"pid"`
	Name          string  `json:"name"`
	Username      string  `json:"username,omitempty"`
	MemoryPercent float64 `json:"memory_percent"`
	RSS           uint64  `json:"rss_bytes"`
	VMS           uint64  `json:"vms_bytes"`
	Swap          uint64  `json:"swap_bytes,omitempty"`
	CreateTime    int64   `json:"create_time,omitempty"`
	Watched       bool    `json:"watched,omitempty"`
}

// GpuData contains GPU metrics.
type GpuData struct {
	Gpus []GpuSensor `json:"gpus"`
}

// GpuSensor contains metrics from a single GPU.
type GpuSensor struct {
	Name        string   `json:"name"`
	Temperature *float64 `json:"temperature_celsius,omitempty"`
	CoreLoad    *float64 `json:"core_load_percent,omitempty"`
	MemoryLoad  *float64 `json:"memory_load_percent,omitempty"`
	FanSpeed    *float64 `json:"fan_speed_rpm,omitempty"`
	Power       *float64 `json:"power_watts,omitempty"`
	CoreClock   *float64 `json:"core_clock_mhz,omitempty"`
	MemoryClock *float64 `json:"memory_clock_mhz,omitempty"`
}

// StorageSmartData contains S.M.A.R.T metrics for storage devices.
type StorageSmartData struct {
	Storages []StorageSmartSensor `json:"storages"`
}

// StorageSmartSensor contains S.M.A.R.T metrics from a single storage device.
type StorageSmartSensor struct {
	Name              string   `json:"name"`
	Type              string   `json:"type"` // NVMe, SSD, HDD
	Temperature       *float64 `json:"temperature_celsius,omitempty"`
	RemainingLife     *float64 `json:"remaining_life_percent,omitempty"`
	MediaErrors       *int64   `json:"media_errors,omitempty"`
	PowerCycles       *int64   `json:"power_cycles,omitempty"`
	UnsafeShutdowns   *int64   `json:"unsafe_shutdowns,omitempty"`
	PowerOnHours      *int64   `json:"power_on_hours,omitempty"`
	TotalBytesWritten *int64   `json:"total_bytes_written,omitempty"`
}

// VoltageData contains voltage sensor metrics.
type VoltageData struct {
	Sensors []VoltageSensor `json:"sensors"`
}

// VoltageSensor contains metrics from a single voltage sensor.
type VoltageSensor struct {
	Name    string  `json:"name"`
	Voltage float64 `json:"voltage"`
}

// MotherboardTempData contains motherboard temperature metrics.
type MotherboardTempData struct {
	Sensors []MotherboardTempSensor `json:"sensors"`
}

// MotherboardTempSensor contains metrics from a single motherboard temperature sensor.
type MotherboardTempSensor struct {
	Name        string  `json:"name"`
	Temperature float64 `json:"temperature_celsius"`
}

// ProcessWatchData contains process watch results for required and forbidden processes.
type ProcessWatchData struct {
	Statuses []ProcessWatchStatus `json:"statuses"`
}

// ProcessWatchStatus represents the status of a single watched process.
type ProcessWatchStatus struct {
	Name    string `json:"name"`
	PID     int32  `json:"pid"`
	Running bool   `json:"running"`
	Type    string `json:"type"` // "required" or "forbidden"
}

// UptimeData contains system uptime and boot time metrics.
type UptimeData struct {
	BootTimeUnix  int64   `json:"boot_time_unix"`
	BootTimeStr   string  `json:"boot_time"`
	UptimeMinutes float64 `json:"uptime_minutes"`
}
