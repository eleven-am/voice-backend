package bootstrap

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/eleven-am/voice-backend/internal/audio"
	"github.com/eleven-am/voice-backend/internal/gateway"
	"github.com/eleven-am/voice-backend/internal/realtime"
	"github.com/eleven-am/voice-backend/internal/router"
	"github.com/eleven-am/voice-backend/internal/synthesis"
	"github.com/eleven-am/voice-backend/internal/transcription"
	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/eleven-am/voice-backend/internal/user"
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
	}
}

func ProvideSTTConfig(cfg *Config) transcription.Config {
	return transcription.Config{
		Address: cfg.STTAddress,
		Token:   cfg.SidecarToken,
	}
}

func ProvideTTSConfig(cfg *Config) synthesis.Config {
	return synthesis.Config{
		Address: cfg.TTSAddress,
		Token:   cfg.SidecarToken,
	}
}

func ProvideRTCManager(cfg realtime.Config) (*realtime.Manager, error) {
	return realtime.NewManager(cfg)
}

func ProvideRouter() *router.SmartRouter {
	return router.NewSmartRouter()
}

func ProvideVoiceSessionManager(bridge *gateway.Bridge, rtr *router.SmartRouter, logger *slog.Logger) *voicesession.Manager {
	return voicesession.NewManager(voicesession.ManagerConfig{
		Bridge: bridge,
		Router: rtr,
		Log:    logger,
	})
}

func ProvideVoiceStarter(sessionMgr *voicesession.Manager, logger *slog.Logger) *gateway.VoiceStarter {
	return gateway.NewVoiceStarter(gateway.VoiceStarterConfig{
		SessionManager: sessionMgr,
		Log:            logger,
	})
}

func ProvideAuthFunc(sessions *user.SessionManager) transport.AuthFunc {
	return func(r *http.Request) (*transport.UserProfile, error) {
		cookie, err := r.Cookie("voice_session")
		if err != nil {
			return nil, err
		}

		payload, err := sessions.VerifyValue(cookie.Value)
		if err != nil {
			return nil, err
		}

		parts := strings.SplitN(payload, "|", 2)
		if len(parts) < 1 || parts[0] == "" {
			return nil, errors.New("invalid session")
		}

		return &transport.UserProfile{
			UserID: parts[0],
		}, nil
	}
}

func ProvideRTCHandler(
	mgr *realtime.Manager,
	starter *gateway.VoiceStarter,
	auth transport.AuthFunc,
	logger *slog.Logger,
) *realtime.Handler {
	return realtime.NewHandler(mgr, starter, auth, nil, logger)
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
	params.Handler.RegisterRoutes(e.Group("/api/v1/voice"))
	params.AudioHandler.RegisterRoutes(e.Group("/api/v1/audio"))
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
