package model

type AppUpdateStatus struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	CanInstall      bool   `json:"can_install"`
	Platform        string `json:"platform"`
	Architecture    string `json:"architecture"`
	ReleaseURL      string `json:"release_url,omitempty"`
	PublishedAt     string `json:"published_at,omitempty"`
	AssetName       string `json:"asset_name,omitempty"`
	Message         string `json:"message,omitempty"`
	Updating        bool   `json:"updating"`
	UpdateError     string `json:"update_error,omitempty"`
	InstallLog      string `json:"install_log,omitempty"`
}

type AppUpdateInstallResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Version string `json:"version"`
}

type AppUpdateInstallStatus struct {
	CurrentVersion string `json:"current_version"`
	Message        string `json:"message,omitempty"`
	Updating       bool   `json:"updating"`
	UpdateError    string `json:"update_error,omitempty"`
	InstallLog     string `json:"install_log,omitempty"`
}
