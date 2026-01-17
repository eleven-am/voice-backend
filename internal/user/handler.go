package user

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	store    *Store
	google   *GoogleProvider
	github   *GitHubProvider
	sessions *SessionManager
	schemes  map[string]struct{}
	logger   *slog.Logger
}

func NewHandler(store *Store, google *GoogleProvider, github *GitHubProvider, sessions *SessionManager, allowedSchemes []string, logger *slog.Logger) *Handler {
	schemeSet := make(map[string]struct{})
	for _, s := range allowedSchemes {
		if trimmed := strings.ToLower(strings.TrimSpace(s)); trimmed != "" {
			schemeSet[trimmed] = struct{}{}
		}
	}

	return &Handler{
		store:    store,
		google:   google,
		github:   github,
		sessions: sessions,
		schemes:  schemeSet,
		logger:   logger,
	}
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/google", h.GoogleLogin)
	g.GET("/google/callback", h.GoogleCallback)
	g.GET("/github", h.GitHubLogin)
	g.GET("/github/callback", h.GitHubCallback)
	g.GET("/me", h.Me)
	g.POST("/me/developer", h.BecomeDeveloper)
	g.POST("/logout", h.Logout)
}

// GoogleLogin godoc
// @Summary      Initiate Google OAuth login
// @Description  Redirects the user to Google OAuth consent page
// @Tags         auth
// @Param        redirect_uri  query  string  false  "URL to redirect after successful login"
// @Success      307  "Redirect to Google OAuth"
// @Failure      500  {object}  shared.APIError
// @Router       /auth/google [get]
func (h *Handler) GoogleLogin(c echo.Context) error {
	return h.handleLogin(c, h.google)
}

// GoogleCallback godoc
// @Summary      Google OAuth callback
// @Description  Handles the callback from Google OAuth and creates user session
// @Tags         auth
// @Param        state  query  string  true   "OAuth state parameter"
// @Param        code   query  string  true   "Authorization code"
// @Success      307  "Redirect to redirect_uri or /"
// @Failure      400  {object}  shared.APIError
// @Failure      500  {object}  shared.APIError
// @Router       /auth/google/callback [get]
func (h *Handler) GoogleCallback(c echo.Context) error {
	return h.handleCallback(c, h.google)
}

// GitHubLogin godoc
// @Summary      Initiate GitHub OAuth login
// @Description  Redirects the user to GitHub OAuth consent page
// @Tags         auth
// @Param        redirect_uri  query  string  false  "URL to redirect after successful login"
// @Success      307  "Redirect to GitHub OAuth"
// @Failure      500  {object}  shared.APIError
// @Router       /auth/github [get]
func (h *Handler) GitHubLogin(c echo.Context) error {
	return h.handleLogin(c, h.github)
}

// GitHubCallback godoc
// @Summary      GitHub OAuth callback
// @Description  Handles the callback from GitHub OAuth and creates user session
// @Tags         auth
// @Param        state  query  string  true   "OAuth state parameter"
// @Param        code   query  string  true   "Authorization code"
// @Success      307  "Redirect to redirect_uri or /"
// @Failure      400  {object}  shared.APIError
// @Failure      500  {object}  shared.APIError
// @Router       /auth/github/callback [get]
func (h *Handler) GitHubCallback(c echo.Context) error {
	return h.handleCallback(c, h.github)
}

// Me godoc
// @Summary      Get current user
// @Description  Returns the currently authenticated user's profile
// @Tags         auth
// @Produce      json
// @Success      200  {object}  dto.MeResponse
// @Failure      401  {object}  shared.APIError
// @Failure      404  {object}  shared.APIError
// @Router       /auth/me [get]
func (h *Handler) Me(c echo.Context) error {
	userID, csrf, err := h.sessions.Get(c)
	if err != nil {
		return shared.Unauthorized("auth_required", "authentication required")
	}

	if err := h.sessions.RequireCSRF(c, csrf); err != nil {
		return err
	}

	user, err := h.store.GetByID(c.Request().Context(), userID)
	if err != nil {
		h.logger.Error("failed to get user", "error", err, "user_id", userID)
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

// BecomeDeveloper godoc
// @Summary      Become a developer
// @Description  Upgrades the current user to developer status
// @Tags         auth
// @Success      204  "No Content"
// @Failure      401  {object}  shared.APIError
// @Failure      404  {object}  shared.APIError
// @Failure      500  {object}  shared.APIError
// @Router       /auth/me/developer [post]
func (h *Handler) BecomeDeveloper(c echo.Context) error {
	userID, csrf, err := h.sessions.Get(c)
	if err != nil {
		return shared.Unauthorized("auth_required", "authentication required")
	}

	if err := h.sessions.RequireCSRF(c, csrf); err != nil {
		return err
	}

	if err := h.store.SetDeveloper(c.Request().Context(), userID, true); err != nil {
		if err == shared.ErrNotFound {
			return shared.NotFound("user_not_found", "user not found")
		}
		h.logger.Error("failed to set developer status", "error", err, "user_id", userID)
		return shared.InternalError("update_failed", "failed to update user")
	}

	return c.NoContent(http.StatusNoContent)
}

// Logout godoc
// @Summary      Logout
// @Description  Clears the user session and logs out
// @Tags         auth
// @Success      204  "No Content"
// @Failure      401  {object}  shared.APIError
// @Router       /auth/logout [post]
func (h *Handler) Logout(c echo.Context) error {
	_, csrf, err := h.sessions.Get(c)
	if err != nil {
		return shared.Unauthorized("auth_required", "authentication required")
	}

	if err := h.sessions.RequireCSRF(c, csrf); err != nil {
		return err
	}

	h.sessions.Clear(c)
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) handleLogin(c echo.Context, provider Provider) error {
	if provider == nil {
		return shared.InternalError("provider_not_configured", "provider not configured")
	}

	redirectURI := h.sanitizeRedirectURI(c.QueryParam("redirect_uri"))
	state := h.sessions.GenerateOAuthState(redirectURI)

	cookie := &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.sessions.secure,
		SameSite: http.SameSiteLaxMode,
	}
	c.SetCookie(cookie)

	return c.Redirect(http.StatusTemporaryRedirect, provider.AuthURL(state))
}

func (h *Handler) handleCallback(c echo.Context, provider Provider) error {
	if provider == nil {
		return shared.InternalError("provider_not_configured", "provider not configured")
	}

	stateCookie, err := c.Cookie("oauth_state")
	if err != nil {
		return shared.BadRequest("missing_state", "missing state cookie")
	}

	state := c.QueryParam("state")
	if state != stateCookie.Value {
		return shared.BadRequest("invalid_state", "state mismatch")
	}

	if _, err := h.sessions.VerifyValue(state); err != nil {
		return shared.BadRequest("invalid_state", "invalid state signature")
	}

	code := c.QueryParam("code")
	if code == "" {
		errMsg := c.QueryParam("error")
		if errMsg == "" {
			errMsg = "missing authorization code"
		}
		return shared.BadRequest("oauth_error", errMsg)
	}

	providerUser, err := provider.Exchange(c.Request().Context(), code)
	if err != nil {
		return shared.InternalError("exchange_failed", "failed to authenticate")
	}

	user, err := h.store.FindOrCreate(
		c.Request().Context(),
		provider.Name(),
		providerUser.Sub,
		providerUser.Email,
		providerUser.Name,
		providerUser.AvatarURL,
	)
	if err != nil {
		h.logger.Error("failed to find or create user", "error", err, "provider", provider.Name())
		return shared.InternalError("user_creation_failed", "failed to create user")
	}

	h.sessions.Create(c, user.ID)

	clearCookie := &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	}
	c.SetCookie(clearCookie)

	redirectURI := h.sessions.ExtractRedirectURI(stateCookie.Value)
	if redirectURI == "" {
		redirectURI = "/"
	}

	return c.Redirect(http.StatusTemporaryRedirect, redirectURI)
}

func (h *Handler) sanitizeRedirectURI(raw string) string {
	if raw == "" {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	if u.Scheme == "" && u.Host == "" && strings.HasPrefix(u.Path, "/") {
		return u.Path
	}

	if u.Scheme == "https" && u.Host != "" {
		return u.String()
	}

	if u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1") {
		return u.String()
	}

	if _, ok := h.schemes[strings.ToLower(u.Scheme)]; ok {
		return raw
	}

	return ""
}
