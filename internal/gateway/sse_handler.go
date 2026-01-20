package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/labstack/echo/v4"
)

type AgentHandler struct {
	auth   *Authenticator
	bridge *Bridge
	logger *slog.Logger
}

func NewAgentHandler(auth *Authenticator, bridge *Bridge, logger *slog.Logger) *AgentHandler {
	return &AgentHandler{
		auth:   auth,
		bridge: bridge,
		logger: logger.With("component", "agent_handler"),
	}
}

func (h *AgentHandler) RegisterRoutes(g *echo.Group) {
	authMiddleware := APIKeyAuth(h.auth)
	rateLimiter := RateLimiter(DefaultRateLimiterConfig())

	g.GET("", h.HandleConnect, authMiddleware)
	g.POST("", h.HandleEvent, authMiddleware, rateLimiter)
	g.GET("/status", h.HandleStatus)
}

func (h *AgentHandler) HandleConnect(c echo.Context) error {
	accept := c.Request().Header.Get("Accept")
	if !strings.Contains(accept, "text/event-stream") {
		return h.handleWebSocket(c)
	}

	return h.handleSSE(c)
}

func (h *AgentHandler) handleSSE(c echo.Context) error {
	key := GetAPIKey(c)
	agentID := key.OwnerID

	if h.bridge.IsOnline(agentID) {
		return shared.Conflict("agent_already_connected", "agent already connected")
	}

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	conn, err := NewSSEAgentConn(c.Response(), agentID, key.OwnerID)
	if err != nil {
		h.logger.Error("failed to create SSE connection", "error", err)
		return shared.InternalError("sse_connection_failed", "failed to create SSE connection")
	}

	if err := h.bridge.RegisterAgent(conn); err != nil {
		h.logger.Error("failed to register agent", "error", err)
		return shared.InternalError("registration_failed", "failed to register agent")
	}
	defer h.bridge.UnregisterAgent(agentID)

	h.logger.Info("agent connected (SSE)", "agent_id", agentID, "owner_id", key.OwnerID)

	_ = conn.Run(c.Request().Context())

	h.logger.Info("agent disconnected (SSE)", "agent_id", agentID)
	return nil
}

func (h *AgentHandler) HandleEvent(c echo.Context) error {
	key := GetAPIKey(c)
	agentID := key.OwnerID

	var event AgentEvent
	if err := c.Bind(&event); err != nil {
		return shared.BadRequest("invalid_payload", "invalid event payload")
	}

	event.AgentID = agentID

	switch event.Type {
	case MessageTypeResponseDelta:
		if p, ok := event.Payload.(map[string]any); ok {
			delta, _ := p["delta"].(string)
			if delta != "" && event.SessionID != "" {
				h.publishDelta(c.Request().Context(), event.SessionID, agentID, delta)
			}
		}
	case MessageTypeResponseDone:
		if p, ok := event.Payload.(map[string]any); ok {
			text, _ := p["text"].(string)
			if event.SessionID != "" {
				h.publishDone(c.Request().Context(), event.SessionID, agentID, text)
			}
		}
	}

	return c.NoContent(http.StatusAccepted)
}

func (h *AgentHandler) HandleStatus(c echo.Context) error {
	agents := h.bridge.ListAgents()
	return c.JSON(http.StatusOK, map[string]any{
		"agents": agents,
		"count":  len(agents),
	})
}

func (h *AgentHandler) publishDelta(ctx context.Context, sessionID, agentID, delta string) {
	msg := &GatewayMessage{
		Type:      MessageTypeResponseDelta,
		SessionID: sessionID,
		AgentID:   agentID,
		Payload: map[string]any{
			"delta": delta,
		},
	}
	if err := h.bridge.PublishResponse(ctx, msg); err != nil {
		h.logger.Error("failed to publish delta", "error", err, "session_id", sessionID)
	}
}

func (h *AgentHandler) publishDone(ctx context.Context, sessionID, agentID, text string) {
	msg := &GatewayMessage{
		Type:      MessageTypeResponseDone,
		SessionID: sessionID,
		AgentID:   agentID,
		Payload: map[string]any{
			"text": text,
		},
	}
	if err := h.bridge.PublishResponse(ctx, msg); err != nil {
		h.logger.Error("failed to publish done", "error", err, "session_id", sessionID)
	}
}

func extractAPIKey(r *http.Request) string {
	if authHeader, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); found {
		return authHeader
	}

	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return apiKey
	}

	return r.URL.Query().Get("api_key")
}
