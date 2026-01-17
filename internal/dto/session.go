package dto

type MetricsResponse struct {
	AgentID      string `json:"agent_id" example:"agt_abc123"`
	Date         string `json:"date" example:"2024-01-15"`
	Hour         int    `json:"hour" example:"14"`
	Sessions     int64  `json:"sessions" example:"100"`
	Utterances   int64  `json:"utterances" example:"500"`
	Responses    int64  `json:"responses" example:"450"`
	UniqueUsers  int64  `json:"unique_users" example:"25"`
	AvgLatencyMs int64  `json:"avg_latency_ms" example:"150"`
	ErrorCount   int64  `json:"error_count" example:"5"`
	NewInstalls  int64  `json:"new_installs" example:"10"`
	Uninstalls   int64  `json:"uninstalls" example:"2"`
}

type MetricsListResponse struct {
	AgentID string            `json:"agent_id" example:"agt_abc123"`
	Hours   int               `json:"hours" example:"24"`
	Metrics []MetricsResponse `json:"metrics"`
}

type SummaryResponse struct {
	AgentID         string  `json:"agent_id" example:"agt_abc123"`
	Period          string  `json:"period" example:"7d"`
	TotalSessions   int64   `json:"total_sessions" example:"1000"`
	TotalUtterances int64   `json:"total_utterances" example:"5000"`
	TotalResponses  int64   `json:"total_responses" example:"4500"`
	UniqueUsers     int64   `json:"unique_users" example:"200"`
	AvgLatencyMs    int64   `json:"avg_latency_ms" example:"145"`
	ErrorRate       float64 `json:"error_rate" example:"1.5"`
	NetInstalls     int64   `json:"net_installs" example:"50"`
}
