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

type SSHMonitorDisk struct {
	MountPoint     string `json:"mount_point"`
	TotalBytes     int64  `json:"total_bytes"`
	UsedBytes      int64  `json:"used_bytes"`
	AvailableBytes int64  `json:"available_bytes"`
	UsagePercent   int    `json:"usage_percent"`
}
