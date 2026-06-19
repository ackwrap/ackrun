package model

type InstallStatus string

const (
	InstallIdle        InstallStatus = "idle"
	InstallDownloading InstallStatus = "downloading"
	InstallExtracting  InstallStatus = "extracting"
	InstallDone        InstallStatus = "done"
	InstallFailed      InstallStatus = "failed"
)

type InstallStateResponse struct {
	Status        InstallStatus `json:"status"`
	Version       string        `json:"version,omitempty"`
	LatestVersion string        `json:"latest_version,omitempty"`
	Progress      float64       `json:"progress,omitempty"`
	Message       string        `json:"message,omitempty"`
	Error         string        `json:"error,omitempty"`
}
