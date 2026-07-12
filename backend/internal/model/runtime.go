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
