package session

import (
	"strconv"
	"time"
)

type Status string

const (
	StatusActive Status = "active"
	StatusEnded  Status = "ended"
	StatusError  Status = "error"
)

type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	AgentID      string    `json:"agent_id"`
	ConnectionID string    `json:"connection_id"`
	Status       Status    `json:"status"`
	StartedAt    time.Time `json:"started_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

func (s *Session) RedisKey() string {
	return "session:" + s.ID
}

type Metrics struct {
	AgentID     string `json:"agent_id"`
	Date        string `json:"date"`
	Hour        int    `json:"hour"`
	Sessions    int64  `json:"sessions"`
	Utterances  int64  `json:"utterances"`
	Responses   int64  `json:"responses"`
	UniqueUsers int64  `json:"unique_users"`
	AvgLatencyMs int64 `json:"avg_latency_ms"`
	ErrorCount  int64  `json:"error_count"`
	NewInstalls int64  `json:"new_installs"`
	Uninstalls  int64  `json:"uninstalls"`
}

func MetricsRedisKey(agentID, date string, hour int) string {
	return "agent:" + agentID + ":metrics:" + date + ":" + strconv.Itoa(hour)
}
