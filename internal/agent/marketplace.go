package agent

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/eleven-am/voice-backend/internal/auth"
	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/labstack/echo/v4"
)

type MarketplaceHandler struct {
	store      *Store
	embeddings EmbeddingService
	logger     *slog.Logger
}

func NewMarketplaceHandler(store *Store, embeddings EmbeddingService, logger *slog.Logger) *MarketplaceHandler {
	return &MarketplaceHandler{
		store:      store,
		embeddings: embeddings,
		logger:     logger,
	}
}

func (h *MarketplaceHandler) RegisterRoutes(g *echo.Group) {
	g.GET("/agents", h.List)
	g.GET("/agents/:id", h.Get)
	g.GET("/agents/search", h.Search)
	g.GET("/agents/:id/reviews", h.GetReviews)
	g.POST("/agents/:id/reviews", h.CreateReview)
}

func agentToMarketplaceResponse(a *Agent) dto.MarketplaceAgentResponse {
	return dto.MarketplaceAgentResponse{
		ID:            a.ID,
		Name:          a.Name,
		Description:   a.Description,
		LogoURL:       a.LogoURL,
		Keywords:      a.Keywords,
		Category:      string(a.Category),
		IsVerified:    a.IsVerified,
		TotalInstalls: a.TotalInstalls,
		AvgRating:     a.AvgRating,
		TotalReviews:  a.TotalReviews,
	}
}

// @Summary      List public agents
// @Description  Returns paginated list of public agents in the marketplace
// @Tags         marketplace
// @Produce      json
// @Param        limit     query     int     false  "Number of results (default 20, max 100)"
// @Param        offset    query     int     false  "Offset for pagination"
// @Param        category  query     string  false  "Filter by category"
// @Success      200       {object}  dto.MarketplaceListResponse
// @Failure      500       {object}  shared.APIError
// @Router       /store/agents [get]
func (h *MarketplaceHandler) List(c echo.Context) error {
	limitStr := c.QueryParam("limit")
	offsetStr := c.QueryParam("offset")
	categoryStr := c.QueryParam("category")

	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	var category *shared.AgentCategory
	if categoryStr != "" {
		cat := shared.AgentCategory(categoryStr)
		category = &cat
	}

	agents, err := h.store.ListPublic(c.Request().Context(), category, limit, offset)
	if err != nil {
		h.logger.Error("failed to list public agents", "error", err)
		return shared.InternalError("list_failed", "failed to list agents")
	}

	response := make([]dto.MarketplaceAgentResponse, len(agents))
	for i, a := range agents {
		response[i] = agentToMarketplaceResponse(a)
	}

	return c.JSON(http.StatusOK, dto.MarketplaceListResponse{
		Agents: response,
		Limit:  limit,
		Offset: offset,
	})
}

// @Summary      Get a public agent
// @Description  Returns details of a public agent by ID
// @Tags         marketplace
// @Produce      json
// @Param        id   path      string  true  "Agent ID"
// @Success      200  {object}  dto.MarketplaceAgentResponse
// @Failure      404  {object}  shared.APIError
// @Failure      500  {object}  shared.APIError
// @Router       /store/agents/{id} [get]
func (h *MarketplaceHandler) Get(c echo.Context) error {
	agentID := c.Param("id")

	agent, err := h.store.GetByID(c.Request().Context(), agentID)
	if err != nil {
		if err == shared.ErrNotFound {
			return shared.NotFound("agent_not_found", "agent not found")
		}
		return shared.InternalError("get_failed", "failed to get agent")
	}

	if !agent.IsPublic {
		return shared.NotFound("agent_not_found", "agent not found")
	}

	return c.JSON(http.StatusOK, agentToMarketplaceResponse(agent))
}

// @Summary      Search agents
// @Description  Searches public agents using semantic search
// @Tags         marketplace
// @Produce      json
// @Param        q      query     string  true   "Search query"
// @Param        limit  query     int     false  "Number of results (default 10, max 50)"
// @Success      200    {object}  dto.MarketplaceSearchResponse
// @Failure      400    {object}  shared.APIError
// @Failure      500    {object}  shared.APIError
// @Router       /store/agents/search [get]
func (h *MarketplaceHandler) Search(c echo.Context) error {
	query := c.QueryParam("q")
	limitStr := c.QueryParam("limit")

	if query == "" {
		return shared.BadRequest("missing_query", "search query is required")
	}

	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	if h.embeddings == nil {
		return shared.InternalError("search_unavailable", "search is not available")
	}

	embedding, err := h.embeddings.Generate(c.Request().Context(), query)
	if err != nil {
		return shared.InternalError("search_failed", "failed to generate search embedding")
	}

	agents, err := h.store.SearchByEmbedding(c.Request().Context(), embedding, limit)
	if err != nil {
		return shared.InternalError("search_failed", "failed to search agents")
	}

	response := make([]dto.MarketplaceAgentResponse, 0, len(agents))
	for _, a := range agents {
		if a.IsPublic {
			response = append(response, agentToMarketplaceResponse(a))
		}
	}

	return c.JSON(http.StatusOK, dto.MarketplaceSearchResponse{Agents: response})
}

func reviewToResponse(r *AgentReview) dto.ReviewResponse {
	resp := dto.ReviewResponse{
		ID:        r.ID,
		UserID:    r.UserID,
		Rating:    r.Rating,
		Body:      r.Body,
		CreatedAt: r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if r.DeveloperReply != nil {
		resp.DeveloperReply = r.DeveloperReply
	}
	if r.RepliedAt != nil {
		repliedAt := r.RepliedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.RepliedAt = &repliedAt
	}

	return resp
}

// @Summary      Get agent reviews
// @Description  Returns paginated list of reviews for a public agent
// @Tags         marketplace
// @Produce      json
// @Param        id      path      string  true   "Agent ID"
// @Param        limit   query     int     false  "Number of results (default 20, max 100)"
// @Param        offset  query     int     false  "Offset for pagination"
// @Success      200     {object}  dto.ReviewListResponse
// @Failure      404     {object}  shared.APIError
// @Failure      500     {object}  shared.APIError
// @Router       /store/agents/{id}/reviews [get]
func (h *MarketplaceHandler) GetReviews(c echo.Context) error {
	agentID := c.Param("id")
	limitStr := c.QueryParam("limit")
	offsetStr := c.QueryParam("offset")

	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	agent, err := h.store.GetByID(c.Request().Context(), agentID)
	if err != nil {
		if err == shared.ErrNotFound {
			return shared.NotFound("agent_not_found", "agent not found")
		}
		return shared.InternalError("get_failed", "failed to get agent")
	}

	if !agent.IsPublic {
		return shared.NotFound("agent_not_found", "agent not found")
	}

	reviews, err := h.store.GetReviews(c.Request().Context(), agentID, limit, offset)
	if err != nil {
		return shared.InternalError("get_reviews_failed", "failed to get reviews")
	}

	response := make([]dto.ReviewResponse, len(reviews))
	for i, r := range reviews {
		response[i] = reviewToResponse(r)
	}

	return c.JSON(http.StatusOK, dto.ReviewListResponse{
		Reviews: response,
		Limit:   limit,
		Offset:  offset,
	})
}

// @Summary      Create a review
// @Description  Creates a review for an installed agent
// @Tags         marketplace
// @Accept       json
// @Produce      json
// @Param        id       path      string                  true  "Agent ID"
// @Param        request  body      dto.CreateReviewRequest true  "Review content"
// @Success      201      {object}  dto.ReviewResponse
// @Failure      400      {object}  shared.APIError
// @Failure      401      {object}  shared.APIError
// @Failure      403      {object}  shared.APIError
// @Failure      404      {object}  shared.APIError
// @Failure      409      {object}  shared.APIError
// @Failure      500      {object}  shared.APIError
// @Security     BearerAuth
// @Router       /store/agents/{id}/reviews [post]
func (h *MarketplaceHandler) CreateReview(c echo.Context) error {
	userID, err := auth.RequireAuth(c)
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

	if !agent.IsPublic {
		return shared.NotFound("agent_not_found", "agent not found")
	}

	install, err := h.store.GetInstall(c.Request().Context(), userID, agentID)
	if err != nil {
		return shared.Forbidden("not_installed", "you must install the agent before reviewing")
	}
	_ = install

	existing, err := h.store.GetUserReview(c.Request().Context(), userID, agentID)
	if err == nil && existing != nil {
		return shared.Conflict("already_reviewed", "you have already reviewed this agent")
	}

	var req dto.CreateReviewRequest
	if err := c.Bind(&req); err != nil {
		return shared.BadRequest("invalid_request", "invalid request body")
	}

	if req.Rating < 1 || req.Rating > 5 {
		return shared.BadRequest("invalid_rating", "rating must be between 1 and 5")
	}

	review := &AgentReview{
		AgentID: agentID,
		UserID:  userID,
		Rating:  req.Rating,
		Body:    req.Body,
	}

	if err := h.store.CreateReview(c.Request().Context(), review); err != nil {
		return shared.InternalError("create_review_failed", "failed to create review")
	}

	return c.JSON(http.StatusCreated, reviewToResponse(review))
}
