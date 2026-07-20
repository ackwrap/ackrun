package model

type ConfigStatusResponse struct {
	HasConfig bool   `json:"has_config"`
	Valid     bool   `json:"valid"`
	FileName  string `json:"file_name,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

type ConfigActiveRequest struct {
	FileName string `json:"file_name" binding:"required"`
}

type ConfigFileItem struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Active    bool   `json:"active"`
	SizeBytes int64  `json:"size_bytes"`
	UpdatedAt int64  `json:"updated_at"`
	Valid     bool   `json:"valid"`
	Error     string `json:"error,omitempty"`
}

type ConfigBackup struct {
	ID         int64  `json:"id"`
	ConfigName string `json:"config_name"`
	FileName   string `json:"file_name"`
	Path       string `json:"path"`
	BackupDate string `json:"backup_date"`
	SizeBytes  int64  `json:"size_bytes"`
	CreatedAt  int64  `json:"created_at"`
}
