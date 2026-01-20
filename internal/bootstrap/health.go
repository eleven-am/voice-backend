package bootstrap

import (
	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/gateway"
	"github.com/eleven-am/voice-backend/internal/health"
	"github.com/eleven-am/voice-backend/internal/synthesis"
	"github.com/eleven-am/voice-backend/internal/transcription"
	"github.com/eleven-am/voice-backend/internal/voicesession"
	"github.com/labstack/echo/v4"
	"github.com/qdrant/go-client/qdrant"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

const version = "1.0.0"

func ProvideHealthHandler(
	db *gorm.DB,
	redis *redis.Client,
	qdrant *qdrant.Client,
	ttsClient *synthesis.Client,
	sttConfig transcription.Config,
	bridge *gateway.Bridge,
	voiceSessionMgr *voicesession.Manager,
	agentStore *agent.Store,
) *health.Handler {
	return health.NewHandler(
		db,
		redis,
		qdrant,
		ttsClient,
		sttConfig,
		bridge,
		voiceSessionMgr,
		agentStore,
		version,
	)
}

func metricsMiddleware(h *health.Handler) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h.IncrementRequests()
			h.IncrementConnections()
			defer h.DecrementConnections()
			return next(c)
		}
	}
}

func RegisterHealthRoutes(e *echo.Echo, h *health.Handler) {
	e.Use(metricsMiddleware(h))
	h.RegisterRoutes(e)
}

var HealthModule = fx.Options(
	fx.Provide(ProvideHealthHandler),
	fx.Invoke(RegisterHealthRoutes),
)
