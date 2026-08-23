package model

import "encoding/json"

type WSEvent struct {
	Type string `json:"type"`
	Time int64  `json:"time"`
	Data any    `json:"data"`
}

type WSCommand struct {
	Type string          `json:"type"`
	Time int64           `json:"time"`
	Data json.RawMessage `json:"data"`
}
