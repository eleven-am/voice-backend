package gateway

import (
	"log/slog"

	"github.com/eleven-am/voice-backend/internal/agent"
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

func ProvideWSServer(auth *Authenticator, bridge *Bridge, logger *slog.Logger) *WSServer {
	return NewWSServer(auth, bridge, logger)
}

func ProvideGRPCServer(bridge *Bridge, agentStore *agent.Store, logger *slog.Logger) *GRPCServer {
	return NewGRPCServer(bridge, agentStore, logger)
}

func ProvideHandler(wsServer *WSServer) *Handler {
	return NewHandler(wsServer)
}

var Module = fx.Options(
	fx.Provide(
		ProvideBridge,
		ProvideAuthenticator,
		ProvideWSServer,
		ProvideGRPCServer,
		ProvideHandler,
	),
)
