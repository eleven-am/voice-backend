package apikey

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/eleven-am/voice-backend/internal/auth"
	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	store     *Store
	userStore *user.Store
	logger    *slog.Logger
}

func NewHandler(store *Store, userStore *user.Store, logger *slog.Logger) *Handler {
	return &Handler{
		store:     store,
		userStore: userStore,
		logger:    logger,
	}
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("", h.List)
	g.POST("", h.Create)
	g.DELETE("/:id", h.Delete)
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

func keyToResponse(k *APIKey) dto.APIKeyResponse {
	resp := dto.APIKeyResponse{
		ID:        k.ID,
		Name:      k.Name,
		Prefix:    k.Prefix,
		CreatedAt: k.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if k.ExpiresAt != nil {
		expiresAt := k.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
		resp.ExpiresAt = &expiresAt
	}

	if k.LastUsedAt != nil {
		lastUsed := k.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.LastUsed = &lastUsed
	}

	return resp
}

func (h *Handler) List(c echo.Context) error {
	userID, err := h.requireDeveloper(c)
	if err != nil {
		return err
	}

	keys, err := h.store.GetByOwner(c.Request().Context(), userID, OwnerTypeUser)
	if err != nil {
		h.logger.Error("failed to list API keys", "error", err, "user_id", userID)
		return shared.InternalError("list_failed", "failed to list API keys")
	}

	response := make([]dto.APIKeyResponse, len(keys))
	for i, k := range keys {
		response[i] = keyToResponse(k)
	}

	return c.JSON(http.StatusOK, dto.APIKeyListResponse{APIKeys: response})
}

func (h *Handler) Create(c echo.Context) error {
	userID, err := h.requireDeveloper(c)
	if err != nil {
		return err
	}

	var req dto.CreateAPIKeyRequest
	if err := c.Bind(&req); err != nil {
		return shared.BadRequest("invalid_request", "invalid request body")
	}

	if req.Name == "" {
		return shared.BadRequest("missing_name", "name is required")
	}

	key := &APIKey{
		OwnerID:   userID,
		OwnerType: OwnerTypeUser,
		Name:      req.Name,
	}

	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		expiresAt := time.Now().AddDate(0, 0, *req.ExpiresIn)
		key.ExpiresAt = &expiresAt
	}

	secret, err := h.store.Create(c.Request().Context(), key)
	if err != nil {
		h.logger.Error("failed to create API key", "error", err, "user_id", userID)
		return shared.InternalError("create_failed", "failed to create API key")
	}

	resp := keyToResponse(key)
	return c.JSON(http.StatusCreated, dto.CreateAPIKeyResponse{
		ID:        resp.ID,
		Name:      resp.Name,
		Prefix:    resp.Prefix,
		CreatedAt: resp.CreatedAt,
		ExpiresAt: resp.ExpiresAt,
		LastUsed:  resp.LastUsed,
		Secret:    secret,
	})
}

func (h *Handler) Delete(c echo.Context) error {
	userID, err := h.requireDeveloper(c)
	if err != nil {
		return err
	}

	keyID := c.Param("id")

	key, err := h.store.GetByID(c.Request().Context(), keyID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.NotFound("key_not_found", "API key not found")
		}
		return shared.InternalError("get_failed", "failed to get API key")
	}

	if key.OwnerID != userID || key.OwnerType != OwnerTypeUser {
		return shared.Forbidden("not_owner", "you don't own this API key")
	}

	if err := h.store.Delete(c.Request().Context(), keyID); err != nil {
		h.logger.Error("failed to delete API key", "error", err, "key_id", keyID)
		return shared.InternalError("delete_failed", "failed to delete API key")
	}

	return c.NoContent(http.StatusNoContent)
}
