package bootstrap

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/eleven-am/voice-backend/docs"
	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/eleven-am/voice-backend/internal/auth"
	"github.com/eleven-am/voice-backend/internal/gateway"
	"github.com/eleven-am/voice-backend/internal/session"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
	echoSwagger "github.com/swaggo/echo-swagger"
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
	AgentConnHandler   *gateway.AgentHandler
	JWTMiddleware      *auth.Middleware
	Config             *Config
}

func RegisterRoutes(e *echo.Echo, params HandlerParams) {
	api := e.Group("/v1")

	authGroup := api.Group("/auth")
	authGroup.Use(params.JWTMiddleware.Authenticate)
	params.UserHandler.RegisterRoutes(authGroup)

	myAgentsGroup := api.Group("/me/agents")
	myAgentsGroup.Use(params.JWTMiddleware.Authenticate)
	params.InstallHandler.RegisterRoutes(myAgentsGroup)

	agentsGroup := api.Group("/agents")
	agentsGroup.Use(params.JWTMiddleware.Authenticate)
	params.AgentHandler.RegisterRoutes(agentsGroup)

	params.MarketplaceHandler.RegisterRoutes(api.Group("/store"))

	apikeysGroup := api.Group("/apikeys")
	apikeysGroup.Use(params.JWTMiddleware.Authenticate)
	params.APIKeyHandler.RegisterRoutes(apikeysGroup)

	metricsGroup := api.Group("/metrics")
	metricsGroup.Use(params.JWTMiddleware.Authenticate)
	params.SessionHandler.RegisterRoutes(metricsGroup)

	params.AgentConnHandler.RegisterRoutes(api.Group("/agents/connect"))

	e.GET("/swagger/*", echoSwagger.EchoWrapHandlerV3())
	e.GET("/asyncapi.yaml", func(c echo.Context) error {
		return c.Blob(http.StatusOK, "application/yaml", docs.AsyncAPISpec)
	})

	e.Static("/assets", params.Config.StaticDir)
	e.GET("/*", func(c echo.Context) error {
		return c.File(params.Config.IndexHTML)
	})
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func ProvideLogger(cfg *Config) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.LogLevel),
	}))
}

func ProvideJWTValidator(cfg *Config) *auth.JWTValidator {
	return auth.NewJWTValidator(cfg.HMACKey)
}

func ProvideJWTMiddleware(validator *auth.JWTValidator, userStore *user.Store) *auth.Middleware {
	return auth.NewMiddleware(validator, userStore)
}

func ProvideUserHandler(store *user.Store, logger *slog.Logger) *user.Handler {
	return user.NewHandler(store, logger.With("handler", "user"))
}

func ProvideAgentHandler(store *agent.Store, userStore *user.Store, embeddings agent.EmbeddingService, logger *slog.Logger) *agent.Handler {
	return agent.NewHandler(store, userStore, embeddings, logger.With("handler", "agent"))
}

func ProvideMarketplaceHandler(store *agent.Store, embeddings agent.EmbeddingService, logger *slog.Logger) *agent.MarketplaceHandler {
	return agent.NewMarketplaceHandler(store, embeddings, logger.With("handler", "marketplace"))
}

func ProvideInstallHandler(store *agent.Store, logger *slog.Logger) *agent.InstallHandler {
	return agent.NewInstallHandler(store, logger.With("handler", "install"))
}

func ProvideAPIKeyHandler(store *apikey.Store, userStore *user.Store, logger *slog.Logger) *apikey.Handler {
	return apikey.NewHandler(store, userStore, logger.With("handler", "apikey"))
}

func ProvideSessionHandler(store *session.Store, agentStore *agent.Store, userStore *user.Store, logger *slog.Logger) *session.Handler {
	return session.NewHandler(store, agentStore, userStore, logger.With("handler", "session"))
}

var HandlersModule = fx.Options(
	fx.Provide(
		ProvideLogger,
		ProvideJWTValidator,
		ProvideJWTMiddleware,
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
