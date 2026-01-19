package gateway

import (
	"log/slog"

	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
)

func ProvideBridge(redisClient *redis.Client, logger *slog.Logger) *Bridge {
	return NewBridge(redisClient, logger)
}

func ProvideAuthenticator(store *apikey.Store) *Authenticator {
	return NewAuthenticator(store)
}

func ProvideAgentHandler(auth *Authenticator, bridge *Bridge, logger *slog.Logger) *AgentHandler {
	return NewAgentHandler(auth, bridge, logger)
}

var Module = fx.Options(
	fx.Provide(
		ProvideBridge,
		ProvideAuthenticator,
		ProvideAgentHandler,
	),
)
