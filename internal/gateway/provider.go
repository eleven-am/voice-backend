package gateway

import (
	"log/slog"

	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

type LiveKitConfig struct {
	APIKey    string
	APISecret string
	URL       string
}

func ProvideBridge(redisClient *redis.Client, logger *slog.Logger) *Bridge {
	return NewBridge(redisClient, logger)
}

func ProvideAuthenticator(store *apikey.Store) *Authenticator {
	return NewAuthenticator(store)
}

func ProvideWSServer(auth *Authenticator, bridge *Bridge, logger *slog.Logger) *WSServer {
	return NewWSServer(auth, bridge, logger)
}

func ProvideGRPCServer(bridge *Bridge, agentStore *agent.Store, logger *slog.Logger) *GRPCServer {
	return NewGRPCServer(bridge, agentStore, logger)
}

func ProvideTokenService(cfg LiveKitConfig) *TokenService {
	return NewTokenService(cfg.APIKey, cfg.APISecret, cfg.URL)
}

func ProvideHandler(
	wsServer *WSServer,
	tokenService *TokenService,
	sessions *user.SessionManager,
	agentStore *agent.Store,
	logger *slog.Logger,
) *Handler {
	return NewHandler(wsServer, tokenService, sessions, agentStore, logger.With("handler", "gateway"))
}

var Module = fx.Options(
	fx.Provide(
		ProvideBridge,
		ProvideAuthenticator,
		ProvideWSServer,
		ProvideGRPCServer,
		ProvideTokenService,
		ProvideHandler,
	),
)
