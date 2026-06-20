package model

type ConfigStatusResponse struct {
	HasConfig bool   `json:"has_config"`
	Valid     bool   `json:"valid"`
	FileName  string `json:"file_name,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
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
