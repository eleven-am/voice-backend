package bootstrap

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/eleven-am/voice-backend/internal/audio"
	"github.com/eleven-am/voice-backend/internal/auth"
	"github.com/eleven-am/voice-backend/internal/gateway"
	"github.com/eleven-am/voice-backend/internal/realtime"
	"github.com/eleven-am/voice-backend/internal/router"
	"github.com/eleven-am/voice-backend/internal/synthesis"
	"github.com/eleven-am/voice-backend/internal/transcription"
	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/eleven-am/voice-backend/internal/voicesession"
	"github.com/labstack/echo/v4"
	"go.uber.org/fx"
)

func ProvideRTCConfig(cfg *Config) realtime.Config {
	iceServers := make([]realtime.ICEServerConfig, 0, len(cfg.RTCICEServers))
	for _, s := range cfg.RTCICEServers {
		iceServers = append(iceServers, realtime.ICEServerConfig{
			URLs:       s.URLs,
			Username:   s.Username,
			Credential: s.Credential,
		})
	}

	return realtime.Config{
		ICEServers: iceServers,
		PortRange: realtime.PortRange{
			Min: cfg.RTCPortMin,
			Max: cfg.RTCPortMax,
		},
		TurnServer: cfg.TurnServer,
		TurnSecret: cfg.TurnSecret,
		TurnRealm:  cfg.TurnRealm,
		TurnTTL:    cfg.TurnTTL,
	}
}

func ProvideSTTConfig(cfg *Config) transcription.Config {
	return transcription.Config{
		Address: cfg.SidecarAddress,
		Token:   cfg.SidecarToken,
	}
}

func ProvideTTSConfig(cfg *Config) synthesis.Config {
	return synthesis.Config{
		Address: cfg.SidecarAddress,
		Token:   cfg.SidecarToken,
	}
}

func ProvideRTCManager(cfg realtime.Config) (*realtime.Manager, error) {
	return realtime.NewManager(cfg)
}

func ProvideRouter() *router.SmartRouter {
	return router.NewSmartRouter()
}

func ProvideVoiceSessionManager(bridge *gateway.Bridge, rtr *router.SmartRouter, visionComponents *VisionComponents, logger *slog.Logger) *voicesession.Manager {
	cfg := voicesession.ManagerConfig{
		Bridge: bridge,
		Router: rtr,
		Log:    logger,
	}

	if visionComponents != nil {
		cfg.VisionAnalyzer = visionComponents.Analyzer
		cfg.VisionStore = visionComponents.Store
	}

	return voicesession.NewManager(cfg)
}

func ProvideVoiceStarter(
	sessionMgr *voicesession.Manager,
	agentStore *agent.Store,
	sttConfig transcription.Config,
	ttsConfig synthesis.Config,
	logger *slog.Logger,
) *gateway.VoiceStarter {
	return gateway.NewVoiceStarter(gateway.VoiceStarterConfig{
		SessionManager: sessionMgr,
		AgentStore:     agentStore,
		Log:            logger,
		DefaultConfig: voicesession.Config{
			STTConfig: sttConfig,
			TTSConfig: ttsConfig,
		},
	})
}

func ProvideAuthFunc(validator *auth.JWTValidator) transport.AuthFunc {
	return func(r *http.Request) (*transport.UserProfile, error) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			return nil, errors.New("authorization header required")
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			return nil, errors.New("bearer token required")
		}

		claims, err := validator.Validate(authHeader)
		if err != nil {
			return nil, err
		}

		return &transport.UserProfile{
			UserID: claims.UserID,
		}, nil
	}
}

func ProvideRTCHandler(
	mgr *realtime.Manager,
	starter *gateway.VoiceStarter,
	auth transport.AuthFunc,
	logger *slog.Logger,
) *realtime.Handler {
	return realtime.NewHandler(mgr, starter, auth, logger)
}

func ProvideTTSClient(cfg synthesis.Config) (*synthesis.Client, error) {
	return synthesis.New(cfg)
}

func ProvideAudioHandler(
	ttsClient *synthesis.Client,
	sttConfig transcription.Config,
	apikeyStore *apikey.Store,
	logger *slog.Logger,
) *audio.Handler {
	return audio.NewHandler(ttsClient, sttConfig, apikeyStore, logger)
}

type VoiceRouteParams struct {
	fx.In

	Handler      *realtime.Handler
	AudioHandler *audio.Handler
	Config       *Config
}

func RegisterVoiceRoutes(e *echo.Echo, params VoiceRouteParams) {
	params.Handler.RegisterRoutes(e.Group("/v1/realtime"))
	params.AudioHandler.RegisterRoutes(e.Group("/v1/audio"))
}

var VoiceModule = fx.Options(
	fx.Provide(
		ProvideRTCConfig,
		ProvideSTTConfig,
		ProvideTTSConfig,
		ProvideRTCManager,
		ProvideRouter,
		ProvideVoiceSessionManager,
		ProvideVoiceStarter,
		ProvideAuthFunc,
		ProvideRTCHandler,
		ProvideTTSClient,
		ProvideAudioHandler,
	),
	fx.Invoke(RegisterVoiceRoutes),
)
