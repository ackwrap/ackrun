package model

type SSHMonitorSnapshot struct {
	Hostname                string           `json:"hostname"`
	Username                string           `json:"username"`
	CPUTotal                int64            `json:"cpu_total"`
	CPUIdle                 int64            `json:"cpu_idle"`
	MemoryTotalBytes        int64            `json:"memory_total_bytes"`
	MemoryAvailableBytes    int64            `json:"memory_available_bytes"`
	NetworkReceivedBytes    int64            `json:"network_received_bytes"`
	NetworkTransmittedBytes int64            `json:"network_transmitted_bytes"`
	UptimeSeconds           int64            `json:"uptime_seconds"`
	LoginSessions           int64            `json:"login_sessions"`
	Disks                   []SSHMonitorDisk `json:"disks"`
	CollectedAt             int64            `json:"collected_at"`
}

type SSHDeviceDetails struct {
	CPUModel             string        `json:"cpu_model"`
	CPUCores             int64         `json:"cpu_cores"`
	MemoryTotalBytes     int64         `json:"memory_total_bytes"`
	MemoryAvailableBytes int64         `json:"memory_available_bytes"`
	SwapTotalBytes       int64         `json:"swap_total_bytes"`
	SwapAvailableBytes   int64         `json:"swap_available_bytes"`
	UptimeSeconds        int64         `json:"uptime_seconds"`
	LoadAverage1         float64       `json:"load_average_1"`
	LoadAverage5         float64       `json:"load_average_5"`
	LoadAverage15        float64       `json:"load_average_15"`
	Software             []SSHSoftware `json:"software"`
	CollectedAt          int64         `json:"collected_at"`
}

type SSHSoftware struct {
	Key       string `json:"key"`
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
}

type SSHMonitorDisk struct {
	MountPoint     string `json:"mount_point"`
	TotalBytes     int64  `json:"total_bytes"`
	UsedBytes      int64  `json:"used_bytes"`
	AvailableBytes int64  `json:"available_bytes"`
	UsagePercent   int    `json:"usage_percent"`
}
