package health

import (
	"context"
	"database/sql"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/gateway"
	"github.com/eleven-am/voice-backend/internal/synthesis"
	"github.com/eleven-am/voice-backend/internal/transcription"
	"github.com/eleven-am/voice-backend/internal/voicesession"
	"github.com/labstack/echo/v4"
	"github.com/qdrant/go-client/qdrant"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

type ComponentStatus struct {
	Status    Status `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

type RuntimeStats struct {
	Goroutines         int    `json:"goroutines"`
	MemoryAllocMB      uint64 `json:"memory_alloc_mb"`
	MemoryTotalAllocMB uint64 `json:"memory_total_alloc_mb"`
	MemorySysMB        uint64 `json:"memory_sys_mb"`
	NumGC              uint32 `json:"num_gc"`
}

type AgentStats struct {
	Total  int `json:"total"`
	Online int `json:"online"`
}

type SessionStats struct {
	ActiveVoiceSessions  int `json:"active_voice_sessions"`
	SessionSubscriptions int `json:"session_subscriptions"`
}

type RequestStats struct {
	TotalRequests     uint64 `json:"total_requests"`
	ActiveConnections int64  `json:"active_connections"`
}

type Stats struct {
	Agents   AgentStats   `json:"agents"`
	Sessions SessionStats `json:"sessions"`
	Requests RequestStats `json:"requests"`
	Runtime  RuntimeStats `json:"runtime"`
}

type HealthResponse struct {
	Status        Status                     `json:"status"`
	Timestamp     time.Time                  `json:"timestamp"`
	Version       string                     `json:"version"`
	UptimeSeconds int64                      `json:"uptime_seconds"`
	Stats         Stats                      `json:"stats"`
	Components    map[string]ComponentStatus `json:"components"`
}

type AgentDetail struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Online       bool     `json:"online"`
	OwnerID      string   `json:"owner_id"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type AgentsResponse struct {
	Total  int           `json:"total"`
	Online int           `json:"online"`
	Agents []AgentDetail `json:"agents"`
}

type SessionDetail struct {
	SessionID   string `json:"session_id"`
	UserID      string `json:"user_id"`
	AgentCount  int    `json:"agent_count"`
	SpeechState string `json:"speech_state"`
}

type SessionsResponse struct {
	Total    int             `json:"total"`
	Sessions []SessionDetail `json:"sessions"`
}

type Handler struct {
	db              *gorm.DB
	redis           *redis.Client
	qdrant          *qdrant.Client
	ttsClient       *synthesis.Client
	sttConfig       transcription.Config
	bridge          *gateway.Bridge
	voiceSessionMgr *voicesession.Manager
	agentStore      *agent.Store
	version         string
	startTime       time.Time

	totalRequests     uint64
	activeConnections int64
}

func NewHandler(
	db *gorm.DB,
	redis *redis.Client,
	qdrant *qdrant.Client,
	ttsClient *synthesis.Client,
	sttConfig transcription.Config,
	bridge *gateway.Bridge,
	voiceSessionMgr *voicesession.Manager,
	agentStore *agent.Store,
	version string,
) *Handler {
	return &Handler{
		db:              db,
		redis:           redis,
		qdrant:          qdrant,
		ttsClient:       ttsClient,
		sttConfig:       sttConfig,
		bridge:          bridge,
		voiceSessionMgr: voiceSessionMgr,
		agentStore:      agentStore,
		version:         version,
		startTime:       time.Now(),
	}
}

func (h *Handler) RegisterRoutes(e *echo.Echo) {
	e.GET("/health", h.Liveness)
	e.GET("/health/ready", h.Readiness)
	e.GET("/health/agents", h.Agents)
	e.GET("/health/sessions", h.Sessions)
}

func (h *Handler) IncrementRequests() {
	atomic.AddUint64(&h.totalRequests, 1)
}

func (h *Handler) IncrementConnections() {
	atomic.AddInt64(&h.activeConnections, 1)
}

func (h *Handler) DecrementConnections() {
	atomic.AddInt64(&h.activeConnections, -1)
}

func (h *Handler) Liveness(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (h *Handler) Readiness(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()

	components := make(map[string]ComponentStatus)
	var mu sync.Mutex
	var wg sync.WaitGroup

	checks := []struct {
		name  string
		check func(context.Context) ComponentStatus
	}{
		{"database", h.checkDatabase},
		{"redis", h.checkRedis},
		{"qdrant", h.checkQdrant},
		{"stt", h.checkSTT},
		{"tts", h.checkTTS},
	}

	wg.Add(len(checks))
	for _, check := range checks {
		go func(name string, fn func(context.Context) ComponentStatus) {
			defer wg.Done()
			status := fn(ctx)
			mu.Lock()
			components[name] = status
			mu.Unlock()
		}(check.name, check.check)
	}
	wg.Wait()

	overallStatus := h.computeOverallStatus(components)

	totalAgents, onlineAgents := h.bridge.AgentCount()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	resp := HealthResponse{
		Status:        overallStatus,
		Timestamp:     time.Now().UTC(),
		Version:       h.version,
		UptimeSeconds: int64(time.Since(h.startTime).Seconds()),
		Stats: Stats{
			Agents: AgentStats{
				Total:  totalAgents,
				Online: onlineAgents,
			},
			Sessions: SessionStats{
				ActiveVoiceSessions:  h.voiceSessionMgr.SessionCount(),
				SessionSubscriptions: h.bridge.SessionSubCount(),
			},
			Requests: RequestStats{
				TotalRequests:     atomic.LoadUint64(&h.totalRequests),
				ActiveConnections: atomic.LoadInt64(&h.activeConnections),
			},
			Runtime: RuntimeStats{
				Goroutines:         runtime.NumGoroutine(),
				MemoryAllocMB:      memStats.Alloc / 1024 / 1024,
				MemoryTotalAllocMB: memStats.TotalAlloc / 1024 / 1024,
				MemorySysMB:        memStats.Sys / 1024 / 1024,
				NumGC:              memStats.NumGC,
			},
		},
		Components: components,
	}

	statusCode := http.StatusOK
	if overallStatus == StatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	}

	return c.JSON(statusCode, resp)
}

func (h *Handler) Agents(c echo.Context) error {
	ctx := c.Request().Context()

	connectedAgents := h.bridge.ListConnectedAgents()

	agents := make([]AgentDetail, 0, len(connectedAgents))
	for _, conn := range connectedAgents {
		detail := AgentDetail{
			ID:      conn.AgentID,
			OwnerID: conn.OwnerID,
			Online:  conn.Online,
		}

		if a, err := h.agentStore.GetByID(ctx, conn.AgentID); err == nil {
			detail.Name = a.Name
			detail.Description = a.Description
			detail.Capabilities = a.Capabilities
		}

		agents = append(agents, detail)
	}

	total, online := h.bridge.AgentCount()
	return c.JSON(http.StatusOK, AgentsResponse{
		Total:  total,
		Online: online,
		Agents: agents,
	})
}

func (h *Handler) Sessions(c echo.Context) error {
	sessions := h.voiceSessionMgr.ListSessions()

	details := make([]SessionDetail, len(sessions))
	for i, s := range sessions {
		details[i] = SessionDetail{
			SessionID:   s.SessionID,
			UserID:      s.UserID,
			AgentCount:  s.AgentCount,
			SpeechState: s.SpeechState,
		}
	}

	return c.JSON(http.StatusOK, SessionsResponse{
		Total:    len(details),
		Sessions: details,
	})
}

func (h *Handler) checkDatabase(ctx context.Context) ComponentStatus {
	start := time.Now()
	if h.db == nil {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "database not configured",
		}
	}

	sqlDB, err := h.db.DB()
	if err != nil {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "failed to get underlying db",
		}
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "ping failed",
		}
	}

	stats := sqlDB.Stats()
	status := h.evaluateDBStats(stats)

	return ComponentStatus{
		Status:    status,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

func (h *Handler) evaluateDBStats(stats sql.DBStats) Status {
	if stats.OpenConnections >= stats.MaxOpenConnections && stats.MaxOpenConnections > 0 {
		return StatusDegraded
	}
	return StatusHealthy
}

func (h *Handler) checkRedis(ctx context.Context) ComponentStatus {
	start := time.Now()
	if h.redis == nil {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "redis not configured",
		}
	}

	if err := h.redis.Ping(ctx).Err(); err != nil {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "ping failed",
		}
	}

	return ComponentStatus{
		Status:    StatusHealthy,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

func (h *Handler) checkQdrant(ctx context.Context) ComponentStatus {
	start := time.Now()
	if h.qdrant == nil {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "qdrant not configured",
		}
	}

	_, err := h.qdrant.ListCollections(ctx)
	if err != nil {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "list collections failed",
		}
	}

	return ComponentStatus{
		Status:    StatusHealthy,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

func (h *Handler) checkSTT(ctx context.Context) ComponentStatus {
	start := time.Now()
	if h.sttConfig.Address == "" {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "stt address not configured",
		}
	}

	conn, err := grpc.NewClient(h.sttConfig.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "dial failed",
		}
	}
	defer conn.Close()

	conn.Connect()
	state := conn.GetState()
	if state != connectivity.Ready && state != connectivity.Idle {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "connection not ready",
		}
	}

	return ComponentStatus{
		Status:    StatusHealthy,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

func (h *Handler) checkTTS(ctx context.Context) ComponentStatus {
	start := time.Now()
	if h.ttsClient == nil {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "tts client not configured",
		}
	}

	if !h.ttsClient.IsConnected() {
		return ComponentStatus{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "not connected",
		}
	}

	_, err := h.ttsClient.ListVoices(ctx)
	if err != nil {
		return ComponentStatus{
			Status:    StatusDegraded,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "list voices failed",
		}
	}

	return ComponentStatus{
		Status:    StatusHealthy,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

func (h *Handler) computeOverallStatus(components map[string]ComponentStatus) Status {
	criticalComponents := []string{"database", "redis"}

	for _, name := range criticalComponents {
		if status, ok := components[name]; ok && status.Status == StatusUnhealthy {
			return StatusUnhealthy
		}
	}

	hasUnhealthy := false
	hasDegraded := false
	for _, status := range components {
		if status.Status == StatusUnhealthy {
			hasUnhealthy = true
		}
		if status.Status == StatusDegraded {
			hasDegraded = true
		}
	}

	if hasUnhealthy || hasDegraded {
		return StatusDegraded
	}

	return StatusHealthy
}
