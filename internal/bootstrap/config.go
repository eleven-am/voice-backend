package bootstrap

import (
	"os"
)

type Config struct {
	ServerAddr string
	GRPCAddr   string

	HMACKey        []byte
	CookieSecure   bool
	CookieDomain   string
	AllowedSchemes []string

	LiveKitAPIKey    string
	LiveKitAPISecret string
	LiveKitURL       string

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	GitHubClientID     string
	GitHubClientSecret string
	GitHubRedirectURL  string

	DatabaseDSN string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	QdrantHost   string
	QdrantPort   int
	QdrantAPIKey string

	StaticDir string
	IndexHTML string
}

func LoadConfig() *Config {
	return &Config{
		ServerAddr: getEnv("SERVER_ADDR", ":8080"),
		GRPCAddr:   getEnv("GRPC_ADDR", ":50051"),

		HMACKey:        []byte(getEnv("HMAC_KEY", "change-me-in-production")),
		CookieSecure:   getEnv("COOKIE_SECURE", "false") == "true",
		CookieDomain:   getEnv("COOKIE_DOMAIN", ""),
		AllowedSchemes: []string{},

		LiveKitAPIKey:    getEnv("LIVEKIT_API_KEY", ""),
		LiveKitAPISecret: getEnv("LIVEKIT_API_SECRET", ""),
		LiveKitURL:       getEnv("LIVEKIT_URL", "ws://localhost:7880"),

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", ""),

		GitHubClientID:     getEnv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
		GitHubRedirectURL:  getEnv("GITHUB_REDIRECT_URL", ""),

		DatabaseDSN: getEnv("DATABASE_DSN", ""),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       0,

		QdrantHost:   getEnv("QDRANT_HOST", "localhost"),
		QdrantPort:   6334,
		QdrantAPIKey: getEnv("QDRANT_API_KEY", ""),

		StaticDir: getEnv("STATIC_DIR", "./static"),
		IndexHTML: getEnv("INDEX_HTML", "./static/index.html"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
