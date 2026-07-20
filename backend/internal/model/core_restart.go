package model

type CoreRestartSettings struct {
	Mode    string `json:"mode"`
	Time    string `json:"time"`
	Weekday int    `json:"weekday"`
}
