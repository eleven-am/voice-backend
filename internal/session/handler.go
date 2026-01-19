package session

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/auth"
	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	store      *Store
	agentStore *agent.Store
	userStore  *user.Store
	logger     *slog.Logger
}

func NewHandler(store *Store, agentStore *agent.Store, userStore *user.Store, logger *slog.Logger) *Handler {
	return &Handler{
		store:      store,
		agentStore: agentStore,
		userStore:  userStore,
		logger:     logger,
	}
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/agents/:id", h.GetMetrics)
	g.GET("/agents/:id/summary", h.GetSummary)
}

func (h *Handler) requireDeveloperOwnership(c echo.Context, agentID string) error {
	userID, err := auth.RequireAuth(c)
	if err != nil {
		return err
	}

	u, err := h.userStore.GetByID(c.Request().Context(), userID)
	if err != nil {
		return shared.NotFound("user_not_found", "user not found")
	}

	if !u.IsDeveloper {
		return shared.Forbidden("not_developer", "developer access required")
	}

	a, err := h.agentStore.GetByID(c.Request().Context(), agentID)
	if err != nil {
		if err == shared.ErrNotFound {
			return shared.NotFound("agent_not_found", "agent not found")
		}
		return shared.InternalError("get_failed", "failed to get agent")
	}

	if a.DeveloperID != userID {
		return shared.Forbidden("not_owner", "you don't own this agent")
	}

	return nil
}

func metricsToResponse(m *Metrics) dto.MetricsResponse {
	return dto.MetricsResponse{
		AgentID:      m.AgentID,
		Date:         m.Date,
		Hour:         m.Hour,
		Sessions:     m.Sessions,
		Utterances:   m.Utterances,
		Responses:    m.Responses,
		UniqueUsers:  m.UniqueUsers,
		AvgLatencyMs: m.AvgLatencyMs,
		ErrorCount:   m.ErrorCount,
		NewInstalls:  m.NewInstalls,
		Uninstalls:   m.Uninstalls,
	}
}

func (h *Handler) GetMetrics(c echo.Context) error {
	agentID := c.Param("id")

	if err := h.requireDeveloperOwnership(c, agentID); err != nil {
		return err
	}

	hoursStr := c.QueryParam("hours")
	hours := 24
	if hoursStr != "" {
		if hr, err := strconv.Atoi(hoursStr); err == nil && hr > 0 && hr <= 168 {
			hours = hr
		}
	}

	metrics, err := h.store.GetMetrics(c.Request().Context(), agentID, hours)
	if err != nil {
		h.logger.Error("failed to get metrics", "error", err, "agent_id", agentID)
		return shared.InternalError("get_metrics_failed", "failed to get metrics")
	}

	response := make([]dto.MetricsResponse, len(metrics))
	for i, m := range metrics {
		response[i] = metricsToResponse(m)
	}

	return c.JSON(http.StatusOK, dto.MetricsListResponse{
		AgentID: agentID,
		Hours:   hours,
		Metrics: response,
	})
}

func (h *Handler) GetSummary(c echo.Context) error {
	agentID := c.Param("id")

	if err := h.requireDeveloperOwnership(c, agentID); err != nil {
		return err
	}

	metrics, err := h.store.GetMetricsForLast7Days(c.Request().Context(), agentID)
	if err != nil {
		h.logger.Error("failed to get metrics summary", "error", err, "agent_id", agentID)
		return shared.InternalError("get_metrics_failed", "failed to get metrics")
	}

	var summary dto.SummaryResponse
	summary.AgentID = agentID
	summary.Period = "7d"

	var totalLatency int64
	var latencyCount int64

	for _, m := range metrics {
		summary.TotalSessions += m.Sessions
		summary.TotalUtterances += m.Utterances
		summary.TotalResponses += m.Responses
		summary.UniqueUsers += m.UniqueUsers
		summary.NetInstalls += m.NewInstalls - m.Uninstalls

		if m.AvgLatencyMs > 0 {
			totalLatency += m.AvgLatencyMs
			latencyCount++
		}

		summary.ErrorRate += float64(m.ErrorCount)
	}

	if latencyCount > 0 {
		summary.AvgLatencyMs = totalLatency / latencyCount
	}

	if summary.TotalResponses > 0 {
		summary.ErrorRate = summary.ErrorRate / float64(summary.TotalResponses) * 100
	} else {
		summary.ErrorRate = 0
	}

	return c.JSON(http.StatusOK, summary)
}
