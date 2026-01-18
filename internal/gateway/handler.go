package gateway

import (
	"log/slog"
	"net/http"

	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	wsServer     *WSServer
	tokenService *TokenService
	sessions     *user.SessionManager
	agentStore   *agent.Store
	logger       *slog.Logger
}

func NewHandler(
	wsServer *WSServer,
	tokenService *TokenService,
	sessions *user.SessionManager,
	agentStore *agent.Store,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		wsServer:     wsServer,
		tokenService: tokenService,
		sessions:     sessions,
		agentStore:   agentStore,
		logger:       logger,
	}
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/ws", h.wsServer.HandleConnection)
	g.POST("/token", h.CreateToken)
}

// CreateToken godoc
// @Summary      Create a LiveKit token
// @Description  Generates a LiveKit access token for the authenticated user to join a voice room
// @Tags         gateway
// @Produce      json
// @Success      200  {object}  dto.LiveKitTokenResponse
// @Failure      401  {object}  shared.APIError
// @Failure      403  {object}  shared.APIError  "No installed agents"
// @Failure      500  {object}  shared.APIError
// @Security     SessionAuth
// @Router       /gateway/token [post]
func (h *Handler) CreateToken(c echo.Context) error {
	userID, csrf, err := h.sessions.Get(c)
	if err != nil {
		return shared.Unauthorized("auth_required", "authentication required")
	}

	if err := h.sessions.RequireCSRF(c, csrf); err != nil {
		return err
	}

	installs, err := h.agentStore.GetUserInstalls(c.Request().Context(), userID)
	if err != nil {
		h.logger.Error("failed to get user installs", "error", err, "user_id", userID)
		return shared.InternalError("get_installs_failed", "failed to check installed agents")
	}

	if len(installs) == 0 {
		return shared.Forbidden("no_agents", "you must have at least one agent installed to use voice features")
	}

	room := h.tokenService.GenerateRoomName()
	token, err := h.tokenService.GenerateToken(userID, room)
	if err != nil {
		h.logger.Error("failed to generate token", "error", err, "user_id", userID)
		return shared.InternalError("token_failed", "failed to generate LiveKit token")
	}

	return c.JSON(http.StatusOK, dto.LiveKitTokenResponse{
		Token:    token,
		URL:      h.tokenService.URL(),
		Room:     room,
		Identity: userID,
	})
}
