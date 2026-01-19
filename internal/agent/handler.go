package agent

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/eleven-am/voice-backend/internal/auth"
	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
)

type EmbeddingService interface {
	Generate(ctx context.Context, text string) ([]float32, error)
}

type Handler struct {
	store      *Store
	userStore  *user.Store
	embeddings EmbeddingService
	logger     *slog.Logger
}

func NewHandler(store *Store, userStore *user.Store, embeddings EmbeddingService, logger *slog.Logger) *Handler {
	return &Handler{
		store:      store,
		userStore:  userStore,
		embeddings: embeddings,
		logger:     logger,
	}
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("", h.List)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	g.POST("/:id/publish", h.Publish)
	g.POST("/:id/reviews/:review_id/reply", h.ReplyToReview)
}

func (h *Handler) requireDeveloper(c echo.Context) (string, error) {
	userID, err := auth.RequireAuth(c)
	if err != nil {
		return "", err
	}

	u, err := h.userStore.GetByID(c.Request().Context(), userID)
	if err != nil {
		return "", shared.NotFound("user_not_found", "user not found")
	}

	if !u.IsDeveloper {
		return "", shared.Forbidden("not_developer", "developer access required")
	}

	return userID, nil
}

func agentToResponse(a *Agent) dto.AgentResponse {
	return dto.AgentResponse{
		ID:             a.ID,
		DeveloperID:    a.DeveloperID,
		Name:           a.Name,
		Description:    a.Description,
		LogoURL:        a.LogoURL,
		Keywords:       a.Keywords,
		Capabilities:   a.Capabilities,
		Category:       string(a.Category),
		IsPublic:       a.IsPublic,
		IsVerified:     a.IsVerified,
		TotalInstalls:  a.TotalInstalls,
		ActiveInstalls: a.ActiveInstalls,
		AvgRating:      a.AvgRating,
		TotalReviews:   a.TotalReviews,
		CreatedAt:      a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      a.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *Handler) List(c echo.Context) error {
	developerID, err := h.requireDeveloper(c)
	if err != nil {
		return err
	}

	agents, err := h.store.GetByDeveloper(c.Request().Context(), developerID)
	if err != nil {
		h.logger.Error("failed to list agents", "error", err, "developer_id", developerID)
		return shared.InternalError("list_failed", "failed to list agents")
	}

	response := make([]dto.AgentResponse, len(agents))
	for i, a := range agents {
		response[i] = agentToResponse(a)
	}

	return c.JSON(http.StatusOK, dto.AgentListResponse{Agents: response})
}

func (h *Handler) Create(c echo.Context) error {
	developerID, err := h.requireDeveloper(c)
	if err != nil {
		return err
	}

	var req dto.CreateAgentRequest
	if err := c.Bind(&req); err != nil {
		return shared.BadRequest("invalid_request", "invalid request body")
	}

	if req.Name == "" {
		return shared.BadRequest("missing_name", "name is required")
	}

	category := shared.AgentCategory(req.Category)
	if category == "" {
		category = shared.AgentCategoryAssistant
	}

	agent := &Agent{
		DeveloperID:  developerID,
		Name:         req.Name,
		Description:  req.Description,
		LogoURL:      req.LogoURL,
		Keywords:     req.Keywords,
		Capabilities: req.Capabilities,
		Category:     category,
	}

	if err := h.store.Create(c.Request().Context(), agent); err != nil {
		h.logger.Error("failed to create agent", "error", err, "developer_id", developerID)
		return shared.InternalError("create_failed", "failed to create agent")
	}

	if h.embeddings != nil {
		go h.updateEmbedding(agent)
	}

	return c.JSON(http.StatusCreated, agentToResponse(agent))
}

func (h *Handler) Get(c echo.Context) error {
	developerID, err := h.requireDeveloper(c)
	if err != nil {
		return err
	}

	agentID := c.Param("id")
	agent, err := h.store.GetByID(c.Request().Context(), agentID)
	if err != nil {
		if err == shared.ErrNotFound {
			return shared.NotFound("agent_not_found", "agent not found")
		}
		return shared.InternalError("get_failed", "failed to get agent")
	}

	if agent.DeveloperID != developerID {
		return shared.Forbidden("not_owner", "you don't own this agent")
	}

	return c.JSON(http.StatusOK, agentToResponse(agent))
}

func (h *Handler) Update(c echo.Context) error {
	developerID, err := h.requireDeveloper(c)
	if err != nil {
		return err
	}

	agentID := c.Param("id")
	agent, err := h.store.GetByID(c.Request().Context(), agentID)
	if err != nil {
		if err == shared.ErrNotFound {
			return shared.NotFound("agent_not_found", "agent not found")
		}
		return shared.InternalError("get_failed", "failed to get agent")
	}

	if agent.DeveloperID != developerID {
		return shared.Forbidden("not_owner", "you don't own this agent")
	}

	var req dto.UpdateAgentRequest
	if err := c.Bind(&req); err != nil {
		return shared.BadRequest("invalid_request", "invalid request body")
	}

	if req.Name != nil {
		agent.Name = *req.Name
	}
	if req.Description != nil {
		agent.Description = *req.Description
	}
	if req.LogoURL != nil {
		agent.LogoURL = *req.LogoURL
	}
	if req.Keywords != nil {
		agent.Keywords = req.Keywords
	}
	if req.Capabilities != nil {
		agent.Capabilities = req.Capabilities
	}
	if req.Category != nil {
		agent.Category = shared.AgentCategory(*req.Category)
	}

	if err := h.store.Update(c.Request().Context(), agent); err != nil {
		h.logger.Error("failed to update agent", "error", err, "agent_id", agent.ID)
		return shared.InternalError("update_failed", "failed to update agent")
	}

	if h.embeddings != nil {
		go h.updateEmbedding(agent)
	}

	return c.JSON(http.StatusOK, agentToResponse(agent))
}

func (h *Handler) Delete(c echo.Context) error {
	developerID, err := h.requireDeveloper(c)
	if err != nil {
		return err
	}

	agentID := c.Param("id")
	agent, err := h.store.GetByID(c.Request().Context(), agentID)
	if err != nil {
		if err == shared.ErrNotFound {
			return shared.NotFound("agent_not_found", "agent not found")
		}
		return shared.InternalError("get_failed", "failed to get agent")
	}

	if agent.DeveloperID != developerID {
		return shared.Forbidden("not_owner", "you don't own this agent")
	}

	if err := h.store.Delete(c.Request().Context(), agentID); err != nil {
		h.logger.Error("failed to delete agent", "error", err, "agent_id", agentID)
		return shared.InternalError("delete_failed", "failed to delete agent")
	}

	if h.embeddings != nil {
		go h.store.DeleteEmbedding(context.Background(), agentID)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) Publish(c echo.Context) error {
	developerID, err := h.requireDeveloper(c)
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

	if agent.DeveloperID != developerID {
		return shared.Forbidden("not_owner", "you don't own this agent")
	}

	agent.IsPublic = true
	if err := h.store.Update(c.Request().Context(), agent); err != nil {
		return shared.InternalError("publish_failed", "failed to publish agent")
	}

	return c.JSON(http.StatusOK, agentToResponse(agent))
}

func (h *Handler) ReplyToReview(c echo.Context) error {
	developerID, err := h.requireDeveloper(c)
	if err != nil {
		return err
	}

	agentID := c.Param("id")
	reviewID := c.Param("review_id")

	agent, err := h.store.GetByID(c.Request().Context(), agentID)
	if err != nil {
		if err == shared.ErrNotFound {
			return shared.NotFound("agent_not_found", "agent not found")
		}
		return shared.InternalError("get_failed", "failed to get agent")
	}

	if agent.DeveloperID != developerID {
		return shared.Forbidden("not_owner", "you don't own this agent")
	}

	var req dto.ReplyToReviewRequest
	if err := c.Bind(&req); err != nil {
		return shared.BadRequest("invalid_request", "invalid request body")
	}

	if req.Reply == "" {
		return shared.BadRequest("missing_reply", "reply is required")
	}

	if err := h.store.AddDeveloperReply(c.Request().Context(), agentID, reviewID, req.Reply); err != nil {
		if err == shared.ErrNotFound {
			return shared.NotFound("review_not_found", "review not found or does not belong to this agent")
		}
		return shared.InternalError("reply_failed", "failed to add reply")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) updateEmbedding(agent *Agent) {
	ctx := context.Background()
	text := agent.Name + " " + agent.Description + " " + strings.Join(agent.Keywords, " ")
	embedding, err := h.embeddings.Generate(ctx, text)
	if err != nil {
		return
	}
	h.store.UpsertEmbedding(ctx, agent.ID, embedding)
}
