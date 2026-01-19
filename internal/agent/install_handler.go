package agent

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/eleven-am/voice-backend/internal/auth"
	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/labstack/echo/v4"
)

type InstallHandler struct {
	store  *Store
	logger *slog.Logger
}

func NewInstallHandler(store *Store, logger *slog.Logger) *InstallHandler {
	return &InstallHandler{
		store:  store,
		logger: logger,
	}
}

func (h *InstallHandler) RegisterRoutes(g *echo.Group) {
	g.GET("", h.List)
	g.POST("/:id/install", h.Install)
	g.DELETE("/:id", h.Uninstall)
	g.PUT("/:id/scopes", h.UpdateScopes)
}

func (h *InstallHandler) List(c echo.Context) error {
	userID, err := auth.RequireAuth(c)
	if err != nil {
		return err
	}

	installs, err := h.store.GetUserInstalls(c.Request().Context(), userID)
	if err != nil {
		h.logger.Error("failed to list user installs", "error", err, "user_id", userID)
		return shared.InternalError("list_failed", "failed to list installed agents")
	}

	response := make([]dto.InstalledAgentResponse, 0, len(installs))
	for _, install := range installs {
		agent, err := h.store.GetByID(c.Request().Context(), install.AgentID)
		if err != nil {
			continue
		}

		response = append(response, dto.InstalledAgentResponse{
			AgentID:       agent.ID,
			Name:          agent.Name,
			Description:   agent.Description,
			LogoURL:       agent.LogoURL,
			GrantedScopes: install.GrantedScopes,
			InstalledAt:   install.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return c.JSON(http.StatusOK, dto.InstalledAgentsResponse{Agents: response})
}

func (h *InstallHandler) Install(c echo.Context) error {
	userID, err := auth.RequireAuth(c)
	if err != nil {
		return err
	}

	agentID := c.Param("id")

	agent, err := h.store.GetByID(c.Request().Context(), agentID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.NotFound("agent_not_found", "agent not found")
		}
		return shared.InternalError("get_failed", "failed to get agent")
	}

	if !agent.IsPublic {
		return shared.NotFound("agent_not_found", "agent not found")
	}

	existing, err := h.store.GetInstall(c.Request().Context(), userID, agentID)
	if err == nil && existing != nil {
		return shared.Conflict("already_installed", "agent is already installed")
	}

	var req dto.InstallRequest
	if err := c.Bind(&req); err != nil {
		return shared.BadRequest("invalid_request", "invalid request body")
	}

	install := &AgentInstall{
		UserID:        userID,
		AgentID:       agentID,
		GrantedScopes: req.Scopes,
	}

	if err := h.store.Install(c.Request().Context(), install); err != nil {
		h.logger.Error("failed to install agent", "error", err, "user_id", userID, "agent_id", agentID)
		return shared.InternalError("install_failed", "failed to install agent")
	}

	return c.JSON(http.StatusCreated, dto.InstalledAgentResponse{
		AgentID:       agent.ID,
		Name:          agent.Name,
		Description:   agent.Description,
		LogoURL:       agent.LogoURL,
		GrantedScopes: install.GrantedScopes,
		InstalledAt:   install.InstalledAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *InstallHandler) Uninstall(c echo.Context) error {
	userID, err := auth.RequireAuth(c)
	if err != nil {
		return err
	}

	agentID := c.Param("id")

	if err := h.store.Uninstall(c.Request().Context(), userID, agentID); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.NotFound("not_installed", "agent is not installed")
		}
		return shared.InternalError("uninstall_failed", "failed to uninstall agent")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *InstallHandler) UpdateScopes(c echo.Context) error {
	userID, err := auth.RequireAuth(c)
	if err != nil {
		return err
	}

	agentID := c.Param("id")

	var req dto.UpdateScopesRequest
	if err := c.Bind(&req); err != nil {
		return shared.BadRequest("invalid_request", "invalid request body")
	}

	if err := h.store.UpdateInstallScopes(c.Request().Context(), userID, agentID, req.Scopes); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.NotFound("not_installed", "agent is not installed")
		}
		return shared.InternalError("update_failed", "failed to update scopes")
	}

	return c.NoContent(http.StatusNoContent)
}
