package bootstrap

import (
	"fmt"
	"time"

	"github.com/eleven-am/voice-backend/internal/vision"
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

type VisionComponents struct {
	Client   *vision.Client
	Store    *vision.Store
	Analyzer *vision.Analyzer
}

func ProvideVisionComponents(cfg *Config, redisClient *redis.Client) *VisionComponents {
	fmt.Printf("VISION DEBUG: Enabled=%v, OllamaURL=%q, Model=%q\n", cfg.VisionEnabled, cfg.VisionOllamaURL, cfg.VisionModel)
	if !cfg.VisionEnabled || cfg.VisionOllamaURL == "" {
		fmt.Println("VISION DEBUG: returning nil - vision disabled or no URL")
		return nil
	}
	fmt.Println("VISION DEBUG: creating vision components")

	visionCfg := vision.Config{
		OllamaURL: cfg.VisionOllamaURL,
		Model:     cfg.VisionModel,
		Timeout:   30 * time.Second,
		FrameTTL:  60 * time.Second,
	}

	client := vision.NewClient(visionCfg)
	store := vision.NewStore(redisClient, 60*time.Second)
	analyzer := vision.NewAnalyzer(client, store, nil)

	return &VisionComponents{
		Client:   client,
		Store:    store,
		Analyzer: analyzer,
	}
}

var InfrastructureModule = fx.Options(
	fx.Provide(
		ProvideRedisClient,
		ProvideDatabase,
		ProvideQdrantClient,
		ProvideVisionComponents,
	),
)
