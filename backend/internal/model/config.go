package model

type ConfigStatusResponse struct {
	HasConfig bool   `json:"has_config"`
	Valid     bool   `json:"valid"`
	FileName  string `json:"file_name,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}
