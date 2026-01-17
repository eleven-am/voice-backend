package bootstrap

import (
	"context"
	"log/slog"
	"os"

	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/eleven-am/voice-backend/internal/gateway"
	"github.com/eleven-am/voice-backend/internal/session"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
	"go.uber.org/fx"
)

type noopEmbeddingService struct{}

func (n *noopEmbeddingService) Generate(ctx context.Context, text string) ([]float32, error) {
	return make([]float32, 384), nil
}

func ProvideEmbeddingService() agent.EmbeddingService {
	return &noopEmbeddingService{}
}

type HandlerParams struct {
	fx.In

	UserHandler        *user.Handler
	AgentHandler       *agent.Handler
	MarketplaceHandler *agent.MarketplaceHandler
	InstallHandler     *agent.InstallHandler
	APIKeyHandler      *apikey.Handler
	SessionHandler     *session.Handler
	GatewayHandler     *gateway.Handler
	Config             *Config
}

func RegisterRoutes(e *echo.Echo, params HandlerParams) {
	api := e.Group("/api/v1")

	params.UserHandler.RegisterRoutes(api.Group("/auth"))

	params.InstallHandler.RegisterRoutes(api.Group("/me/agents"))

	params.AgentHandler.RegisterRoutes(api.Group("/agents"))

	params.MarketplaceHandler.RegisterRoutes(api.Group("/store"))

	params.APIKeyHandler.RegisterRoutes(api.Group("/apikeys"))

	params.SessionHandler.RegisterRoutes(api.Group("/metrics"))

	params.GatewayHandler.RegisterRoutes(api.Group("/gateway"))

	e.Static("/assets", params.Config.StaticDir)
	e.GET("/*", func(c echo.Context) error {
		return c.File(params.Config.IndexHTML)
	})
}

func ProvideLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

func ProvideSessionManager(cfg *Config) *user.SessionManager {
	return user.NewSessionManager(cfg.HMACKey, cfg.CookieSecure, cfg.CookieDomain)
}

func ProvideGoogleProvider(cfg *Config) *user.GoogleProvider {
	return user.NewGoogleProvider(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL)
}

func ProvideGitHubProvider(cfg *Config) *user.GitHubProvider {
	return user.NewGitHubProvider(cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.GitHubRedirectURL)
}

func ProvideUserHandler(store *user.Store, google *user.GoogleProvider, github *user.GitHubProvider, sessions *user.SessionManager, cfg *Config, logger *slog.Logger) *user.Handler {
	return user.NewHandler(store, google, github, sessions, cfg.AllowedSchemes, logger.With("handler", "user"))
}

func ProvideAgentHandler(store *agent.Store, userStore *user.Store, sessions *user.SessionManager, embeddings agent.EmbeddingService, logger *slog.Logger) *agent.Handler {
	return agent.NewHandler(store, userStore, sessions, embeddings, logger.With("handler", "agent"))
}

func ProvideMarketplaceHandler(store *agent.Store, sessions *user.SessionManager, embeddings agent.EmbeddingService, logger *slog.Logger) *agent.MarketplaceHandler {
	return agent.NewMarketplaceHandler(store, sessions, embeddings, logger.With("handler", "marketplace"))
}

func ProvideInstallHandler(store *agent.Store, sessions *user.SessionManager, logger *slog.Logger) *agent.InstallHandler {
	return agent.NewInstallHandler(store, sessions, logger.With("handler", "install"))
}

func ProvideAPIKeyHandler(store *apikey.Store, userStore *user.Store, sessions *user.SessionManager, logger *slog.Logger) *apikey.Handler {
	return apikey.NewHandler(store, userStore, sessions, logger.With("handler", "apikey"))
}

func ProvideSessionHandler(store *session.Store, agentStore *agent.Store, userStore *user.Store, sessions *user.SessionManager, logger *slog.Logger) *session.Handler {
	return session.NewHandler(store, agentStore, userStore, sessions, logger.With("handler", "session"))
}

var HandlersModule = fx.Options(
	fx.Provide(
		ProvideLogger,
		ProvideSessionManager,
		ProvideGoogleProvider,
		ProvideGitHubProvider,
		ProvideEmbeddingService,
		ProvideUserHandler,
		ProvideAgentHandler,
		ProvideMarketplaceHandler,
		ProvideInstallHandler,
		ProvideAPIKeyHandler,
		ProvideSessionHandler,
	),
	fx.Invoke(RegisterRoutes),
)
