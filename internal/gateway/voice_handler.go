package gateway

import (
	"log/slog"
	"time"

	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/eleven-am/voice-backend/internal/voicesession"
)

type VoiceStarter struct {
	sessionMgr *voicesession.Manager
	log        *slog.Logger

	defaultAgentID string
	defaultConfig  voicesession.Config
}

type VoiceStarterConfig struct {
	SessionManager *voicesession.Manager
	DefaultAgentID string
	DefaultConfig  voicesession.Config
	Log            *slog.Logger
}

func NewVoiceStarter(cfg VoiceStarterConfig) *VoiceStarter {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &VoiceStarter{
		sessionMgr:     cfg.SessionManager,
		defaultAgentID: cfg.DefaultAgentID,
		defaultConfig:  cfg.DefaultConfig,
		log:            cfg.Log.With("component", "voice_starter"),
	}
}

func (v *VoiceStarter) Start(req transport.StartRequest) error {
	cfg := v.defaultConfig
	cfg.AgentID = v.defaultAgentID

	if req.UserContext != nil {
		cfg.UserID = req.UserContext.UserID
	}

	session, err := v.sessionMgr.CreateSession(req.Conn, req.UserContext, cfg)
	if err != nil {
		v.log.Error("failed to create voice session",
			"error", err,
			"user_id", cfg.UserID,
		)
		return err
	}

	v.log.Info("voice session started",
		"session_id", session.SessionID(),
		"user_id", cfg.UserID,
		"timestamp", time.Now(),
	)

	return nil
}
