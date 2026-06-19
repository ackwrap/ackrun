package model

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type ErrorResponse struct {
	Error APIError `json:"error"`
}

type ActionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
