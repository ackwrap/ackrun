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
	Status    RuntimeStatus `json:"status"`
	PID       int           `json:"pid,omitempty"`
	Version   string        `json:"version,omitempty"`
	Platform  string        `json:"platform,omitempty"`
	ProxyPort int           `json:"proxy_port,omitempty"`
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

type CoreLogSummary struct {
	Total      int `json:"total"`
	Stdout     int `json:"stdout"`
	Stderr     int `json:"stderr"`
	ErrorLines int `json:"error_lines"`
}

type CoreDiagnosticsResponse struct {
	GeneratedAt   int64                    `json:"generated_at"`
	Platform      string                   `json:"platform"`
	Architecture  string                   `json:"architecture"`
	Version       string                   `json:"version,omitempty"`
	Running       bool                     `json:"running"`
	PID           int                      `json:"pid,omitempty"`
	BinaryPath    string                   `json:"binary_path"`
	ConfigPath    string                   `json:"config_path,omitempty"`
	ConfigPresent bool                     `json:"config_present"`
	ConfigValid   bool                     `json:"config_valid"`
	Network       MaintenanceCheckResponse `json:"network"`
	Logs          CoreLogSummary           `json:"logs"`
}
