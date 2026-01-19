package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type AgentHandler struct {
	auth     *Authenticator
	registry *AgentRegistry
	bridge   *Bridge
	logger   *slog.Logger
}

func NewAgentHandler(auth *Authenticator, registry *AgentRegistry, bridge *Bridge, logger *slog.Logger) *AgentHandler {
	return &AgentHandler{
		auth:     auth,
		registry: registry,
		bridge:   bridge,
		logger:   logger.With("component", "agent_handler"),
	}
}

func (h *AgentHandler) RegisterRoutes(g *echo.Group) {
	g.GET("", h.HandleConnect)
	g.POST("", h.HandleEvent)
}

func (h *AgentHandler) HandleConnect(c echo.Context) error {
	accept := c.Request().Header.Get("Accept")
	if !strings.Contains(accept, "text/event-stream") {
		return h.handleWebSocket(c)
	}

	return h.handleSSE(c)
}

func (h *AgentHandler) handleWebSocket(c echo.Context) error {
	apiKey := extractAPIKey(c.Request())
	if apiKey == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing api key")
	}

	key, err := h.auth.ValidateAPIKey(c.Request().Context(), apiKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	agentID := c.QueryParam("agent_id")
	if agentID == "" {
		if key.OwnerType == "agent" {
			agentID = key.OwnerID
		} else {
			return echo.NewHTTPError(http.StatusBadRequest, "missing agent_id")
		}
	}

	if err := h.auth.ValidateAgentAccess(key, agentID); err != nil {
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}

	ws, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed", "error", err)
		return err
	}

	conn := newWSAgentConnection(ws, agentID, key.OwnerID, h.logger)

	if err := h.registry.Register(conn); err != nil {
		h.logger.Error("failed to register agent", "error", err)
		_ = ws.Close()
		return nil
	}

	h.bridge.RegisterAgent(conn)

	h.logger.Info("agent connected (WebSocket)", "agent_id", agentID, "owner_id", key.OwnerID)

	go conn.writePump()
	conn.readPump(h.bridge)

	h.bridge.UnregisterAgent(agentID)
	h.registry.Unregister(agentID)

	h.logger.Info("agent disconnected (WebSocket)", "agent_id", agentID)
	return nil
}

func (h *AgentHandler) handleSSE(c echo.Context) error {
	apiKey := extractAPIKey(c.Request())
	if apiKey == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing api key")
	}

	key, err := h.auth.ValidateAPIKey(c.Request().Context(), apiKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	agentID := c.QueryParam("agent_id")
	if agentID == "" {
		if key.OwnerType == "agent" {
			agentID = key.OwnerID
		} else {
			return echo.NewHTTPError(http.StatusBadRequest, "missing agent_id")
		}
	}

	if err := h.auth.ValidateAgentAccess(key, agentID); err != nil {
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}

	if h.registry.IsOnline(agentID) {
		return echo.NewHTTPError(http.StatusConflict, "agent already connected")
	}

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	conn, err := NewSSEAgentConn(c.Response(), agentID, key.OwnerID)
	if err != nil {
		h.logger.Error("failed to create SSE connection", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create SSE connection")
	}

	if err := h.registry.Register(conn); err != nil {
		h.logger.Error("failed to register agent", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to register agent")
	}

	h.bridge.RegisterAgent(conn)
	defer func() {
		h.bridge.UnregisterAgent(agentID)
		h.registry.Unregister(agentID)
	}()

	h.logger.Info("agent connected (SSE)", "agent_id", agentID, "owner_id", key.OwnerID)

	_ = conn.Run(c.Request().Context())

	h.logger.Info("agent disconnected (SSE)", "agent_id", agentID)
	return nil
}

func (h *AgentHandler) HandleEvent(c echo.Context) error {
	apiKey := extractAPIKey(c.Request())
	if apiKey == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing api key")
	}

	key, err := h.auth.ValidateAPIKey(c.Request().Context(), apiKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	agentID := c.QueryParam("agent_id")
	if agentID == "" {
		if key.OwnerType == "agent" {
			agentID = key.OwnerID
		} else {
			return echo.NewHTTPError(http.StatusBadRequest, "missing agent_id")
		}
	}

	if err := h.auth.ValidateAgentAccess(key, agentID); err != nil {
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}

	var event AgentEvent
	if err := c.Bind(&event); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid event payload")
	}

	event.AgentID = agentID

	sseConn, ok := h.registry.GetSSE(agentID)
	if !ok {
		return echo.NewHTTPError(http.StatusConflict, "agent not connected via SSE")
	}

	sseConn.RouteEvent(event)

	if event.Type == AgentEventResponseTextDelta || event.Type == AgentEventResponseTextDone {
		if p, ok := event.Payload.(map[string]any); ok {
			text := ""
			if event.Type == AgentEventResponseTextDelta {
				text, _ = p["delta"].(string)
			} else {
				text, _ = p["text"].(string)
			}
			if text != "" && event.SessionID != "" {
				h.publishTextResponse(c.Request().Context(), event.SessionID, agentID, text)
			}
		}
	}

	return c.NoContent(http.StatusAccepted)
}

func (h *AgentHandler) HandleStatus(c echo.Context) error {
	agents := h.registry.List()
	return c.JSON(http.StatusOK, map[string]any{
		"agents": agents,
		"count":  len(agents),
	})
}

func (h *AgentHandler) publishTextResponse(ctx context.Context, sessionID, agentID, text string) {
	msg := &GatewayMessage{
		Type:      MessageTypeResponse,
		SessionID: sessionID,
		AgentID:   agentID,
		Payload: map[string]any{
			"text": text,
		},
	}

	if err := h.bridge.PublishResponse(ctx, msg); err != nil {
		h.logger.Error("failed to publish response", "error", err, "session_id", sessionID)
	}
}

func extractAPIKey(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return apiKey
	}

	return r.URL.Query().Get("api_key")
}
