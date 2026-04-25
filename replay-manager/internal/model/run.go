package model

import "time"

type Meta struct {
	RunID     string            `json:"run_id"`
	Start     time.Time         `json:"start"`
	End       time.Time         `json:"end"`
	CreatedAt time.Time         `json:"created_at"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type RunInfo struct {
	Meta
	SizeBytes int64 `json:"size_bytes"`
	Loaded    bool  `json:"loaded"`
}

type CreateRunRequest struct {
	Start  time.Time         `json:"start"`
	End    time.Time         `json:"end"`
	Labels map[string]string `json:"labels,omitempty"`
}
