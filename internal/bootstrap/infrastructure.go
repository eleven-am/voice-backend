package bootstrap

import (
	"github.com/qdrant/go-client/qdrant"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ProvideRedisClient(cfg *Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
}

func ProvideDatabase(cfg *Config) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(cfg.DatabaseDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

func ProvideQdrantClient(cfg *Config) (*qdrant.Client, error) {
	return qdrant.NewClient(&qdrant.Config{
		Host:   cfg.QdrantHost,
		Port:   cfg.QdrantPort,
		APIKey: cfg.QdrantAPIKey,
	})
}

var InfrastructureModule = fx.Options(
	fx.Provide(
		ProvideRedisClient,
		ProvideDatabase,
		ProvideQdrantClient,
	),
)
