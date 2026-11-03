package model

type RuntimeStatus string

const (
	RuntimeNotInstalled RuntimeStatus = "not_installed"
	RuntimeNoConfig     RuntimeStatus = "no_config"
	RuntimeStopped      RuntimeStatus = "stopped"
	RuntimeRunning      RuntimeStatus = "running"
	RuntimeError        RuntimeStatus = "error"
)

type RuntimeResponse struct {
	Status            RuntimeStatus `json:"status"`
	PID               int           `json:"pid,omitempty"`
	Version           string        `json:"version,omitempty"`
	Platform          string        `json:"platform,omitempty"`
	ProxyPort         int           `json:"proxy_port,omitempty"`
	UptimeSeconds     *int64        `json:"uptime_seconds,omitempty"`
	CoreUptimeSeconds *int64        `json:"core_uptime_seconds,omitempty"`
}

type MaintenanceCheck struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type MaintenanceCheckResponse struct {
	Success bool               `json:"success"`
	Checks  []MaintenanceCheck `json:"checks"`
}
