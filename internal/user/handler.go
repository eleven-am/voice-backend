package user

import (
	"log/slog"
	"net/http"

	"github.com/eleven-am/voice-backend/internal/auth"
	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	store  *Store
	logger *slog.Logger
}

func NewHandler(store *Store, logger *slog.Logger) *Handler {
	return &Handler{
		store:  store,
		logger: logger,
	}
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/me", h.Me)
	g.POST("/me/developer", h.BecomeDeveloper)
}

func (h *Handler) Me(c echo.Context) error {
	claims := auth.GetClaims(c)
	if claims == nil {
		return shared.Unauthorized("auth_required", "authentication required")
	}

	user, err := h.store.GetByID(c.Request().Context(), claims.UserID)
	if err != nil {
		h.logger.Error("failed to get user", "error", err, "user_id", claims.UserID)
		return shared.NotFound("user_not_found", "user not found")
	}

	return c.JSON(http.StatusOK, dto.MeResponse{
		ID:          user.ID,
		Email:       user.Email,
		Name:        user.Name,
		AvatarURL:   user.AvatarURL,
		IsDeveloper: user.IsDeveloper,
	})
}

func (h *Handler) BecomeDeveloper(c echo.Context) error {
	claims := auth.GetClaims(c)
	if claims == nil {
		return shared.Unauthorized("auth_required", "authentication required")
	}

	if err := h.store.SetDeveloper(c.Request().Context(), claims.UserID, true); err != nil {
		if err == shared.ErrNotFound {
			return shared.NotFound("user_not_found", "user not found")
		}
		h.logger.Error("failed to set developer status", "error", err, "user_id", claims.UserID)
		return shared.InternalError("update_failed", "failed to update user")
	}

	return c.NoContent(http.StatusNoContent)
}
