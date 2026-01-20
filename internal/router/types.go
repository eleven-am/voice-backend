package router

import (
	"context"
	"time"
)

type Router interface {
	Route(ctx context.Context, request string, agents []AgentInfo) []string
}

type HealthAwareRouter interface {
	SetHealth(map[string]HealthMetrics)
}

type IndexedRouter interface {
	Router
	HealthAwareRouter
	Index(agents []AgentInfo)
}

type AgentInfo struct {
	ID            string
	Model         string
	Name          string
	Description   string
	Keywords      []string
	Capabilities  []string
	Examples      []string
	GrantedScopes []string
}

type HealthMetrics struct {
	LatencyMs int64
	Load      float64
	Healthy   bool
	UpdatedAt time.Time
}

type RouteContext struct {
	RequestText string
	Agents      []AgentInfo
	Health      map[string]HealthMetrics
}

func selectBestByHealth(agents []AgentInfo, health map[string]HealthMetrics) string {
	var best string
	var bestLatency int64 = -1

	for _, a := range agents {
		h, ok := health[a.ID]
		if !ok || !h.Healthy {
			continue
		}
		if best == "" || (h.LatencyMs >= 0 && (bestLatency == -1 || h.LatencyMs < bestLatency)) {
			best = a.ID
			bestLatency = h.LatencyMs
		}
	}
	return best
}
