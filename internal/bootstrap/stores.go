package bootstrap

import (
	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/eleven-am/voice-backend/internal/session"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/qdrant/go-client/qdrant"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

func ProvideUserStore(db *gorm.DB) *user.Store {
	return user.NewStore(db)
}

func ProvideAgentStore(db *gorm.DB, qdrantClient *qdrant.Client) *agent.Store {
	return agent.NewStore(db, qdrantClient)
}

func ProvideAPIKeyStore(db *gorm.DB) *apikey.Store {
	return apikey.NewStore(db)
}

func ProvideSessionStore(redisClient *redis.Client) *session.Store {
	return session.NewStore(redisClient)
}

func RunMigrations(userStore *user.Store, agentStore *agent.Store, apiKeyStore *apikey.Store) error {
	if err := userStore.Migrate(); err != nil {
		return err
	}
	if err := agentStore.Migrate(); err != nil {
		return err
	}
	return apiKeyStore.Migrate()
}

var StoresModule = fx.Options(
	fx.Provide(
		ProvideUserStore,
		ProvideAgentStore,
		ProvideAPIKeyStore,
		ProvideSessionStore,
	),
	fx.Invoke(RunMigrations),
)
