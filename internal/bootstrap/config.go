package bootstrap

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ServerAddr string
	GRPCAddr   string

	JWTSecret   []byte
	CORSOrigins []string

	RTCICEServers []ICEServerConfig
	RTCPortMin    int
	RTCPortMax    int

	STTAddress   string
	TTSAddress   string
	SidecarToken string
	SidecarTLS   bool

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

type ICEServerConfig struct {
	URLs       []string
	Username   string
	Credential string
}

func LoadConfig() *Config {
	return &Config{
		ServerAddr: getEnv("SERVER_ADDR", ":8080"),
		GRPCAddr:   getEnv("GRPC_ADDR", ":50051"),

		JWTSecret:   []byte(getEnv("JWT_SECRET", "change-me-in-production")),
		CORSOrigins: parseCORSOrigins(getEnv("CORS_ORIGINS", "")),

		RTCICEServers: parseICEServers(getEnv("RTC_ICE_SERVERS", "stun:stun.l.google.com:19302")),
		RTCPortMin:    getEnvInt("RTC_PORT_MIN", 10000),
		RTCPortMax:    getEnvInt("RTC_PORT_MAX", 20000),

		STTAddress:   getEnv("STT_ADDRESS", "localhost:50052"),
		TTSAddress:   getEnv("TTS_ADDRESS", "localhost:50053"),
		SidecarToken: getEnv("SIDECAR_TOKEN", ""),
		SidecarTLS:   getEnv("SIDECAR_TLS", "false") == "true",

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

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func parseICEServers(envValue string) []ICEServerConfig {
	if envValue == "" {
		return []ICEServerConfig{{URLs: []string{"stun:stun.l.google.com:19302"}}}
	}

	var servers []ICEServerConfig
	for _, url := range strings.Split(envValue, ",") {
		url = strings.TrimSpace(url)
		if url != "" {
			servers = append(servers, ICEServerConfig{URLs: []string{url}})
		}
	}

	if len(servers) == 0 {
		return []ICEServerConfig{{URLs: []string{"stun:stun.l.google.com:19302"}}}
	}

	return servers
}

func parseCORSOrigins(envValue string) []string {
	if envValue == "" {
		return nil
	}

	var origins []string
	for _, origin := range strings.Split(envValue, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}
