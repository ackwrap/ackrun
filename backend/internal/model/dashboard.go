package model

type Dashboard struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Installed       bool   `json:"installed"`
	Selected        bool   `json:"selected"`
	LocalPath       string `json:"local_path,omitempty"`
	UpdatedAt       int64  `json:"updated_at,omitempty"`
	CurrentVersion  string `json:"current_version,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	CheckError      string `json:"check_error,omitempty"`
}
