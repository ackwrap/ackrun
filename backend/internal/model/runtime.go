package model

type RuntimeStatus string

const (
	RuntimeNotInstalled RuntimeStatus = "not_installed"
	RuntimeNoConfig     RuntimeStatus = "no_config"
	RuntimeStopped      RuntimeStatus = "stopped"
	RuntimeRunning      RuntimeStatus = "running"
)

type RuntimeResponse struct {
	Status  RuntimeStatus `json:"status"`
	PID     int           `json:"pid,omitempty"`
	Version string        `json:"version,omitempty"`
}
