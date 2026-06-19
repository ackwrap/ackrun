package model

type WSEvent struct {
	Type string `json:"type"`
	Time int64  `json:"time"`
	Data any    `json:"data"`
}
