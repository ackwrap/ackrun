package model

type SimpleSetupRequest struct {
	SubscriptionURL string `json:"subscription_url"`
}

type SimpleSetupStatus struct {
	Status            string `json:"status"`
	Stage             string `json:"stage"`
	Error             string `json:"error,omitempty"`
	Configured        bool   `json:"configured"`
	Supported         bool   `json:"supported"`
	HasExistingConfig bool   `json:"has_existing_config"`
	CanRetry          bool   `json:"can_retry"`
}
