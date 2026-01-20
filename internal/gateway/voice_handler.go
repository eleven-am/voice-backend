package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/router"
	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/eleven-am/voice-backend/internal/voicesession"
)

type VoiceStarter struct {
	sessionMgr    *voicesession.Manager
	agentStore    *agent.Store
	log           *slog.Logger
	defaultConfig voicesession.Config
}

type VoiceStarterConfig struct {
	SessionManager *voicesession.Manager
	AgentStore     *agent.Store
	DefaultConfig  voicesession.Config
	Log            *slog.Logger
}

func NewVoiceStarter(cfg VoiceStarterConfig) *VoiceStarter {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &VoiceStarter{
		sessionMgr:    cfg.SessionManager,
		agentStore:    cfg.AgentStore,
		defaultConfig: cfg.DefaultConfig,
		log:           cfg.Log.With("component", "voice_starter"),
	}
}

func (v *VoiceStarter) Start(req transport.StartRequest) error {
	cfg := v.defaultConfig

	if req.UserContext != nil {
		cfg.UserID = req.UserContext.UserID
	}

	v.applySessionConfig(&cfg, req.Config)

	if v.agentStore != nil && cfg.UserID != "" {
		agents, err := v.agentStore.GetInstalledAgents(context.Background(), cfg.UserID)
		if err != nil {
			v.log.Warn("failed to load user agents, continuing without",
				"error", err,
				"user_id", cfg.UserID,
			)
		} else {
			cfg.Agents = toRouterAgents(agents)
		}
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
		"agent_count", len(cfg.Agents),
		"timestamp", time.Now(),
	)

	return nil
}

func (v *VoiceStarter) applySessionConfig(cfg *voicesession.Config, sessionCfg *transport.SessionConfig) {
	if sessionCfg == nil {
		return
	}

	if sessionCfg.Voice != "" {
		cfg.VoiceID = sessionCfg.Voice
	}
	if sessionCfg.Speed > 0 {
		cfg.TTSSpeed = sessionCfg.Speed
	}

	if sessionCfg.InputAudioTranscription != nil {
		if sessionCfg.InputAudioTranscription.Model != "" {
			cfg.STTOptions.ModelID = sessionCfg.InputAudioTranscription.Model
		}
		if sessionCfg.InputAudioTranscription.Language != "" {
			cfg.STTOptions.Language = sessionCfg.InputAudioTranscription.Language
		}
		if sessionCfg.InputAudioTranscription.Prompt != "" {
			cfg.STTOptions.InitialPrompt = sessionCfg.InputAudioTranscription.Prompt
		}
		if sessionCfg.InputAudioTranscription.Temperature > 0 {
			cfg.STTOptions.Temperature = sessionCfg.InputAudioTranscription.Temperature
		}
	}

	if sessionCfg.TurnDetection != nil {
		cfg.BargeInPolicy.AllowWhileSpeaking = true
		if sessionCfg.TurnDetection.SilenceDurationMs > 0 {
			cfg.BargeInPolicy.MinSilenceForEnd = time.Duration(sessionCfg.TurnDetection.SilenceDurationMs) * time.Millisecond
		}
		if sessionCfg.TurnDetection.PrefixPaddingMs > 0 {
			cfg.BargeInPolicy.DebounceMin = time.Duration(sessionCfg.TurnDetection.PrefixPaddingMs) * time.Millisecond
		}
	}
}

func toRouterAgents(agents []*agent.Agent) []router.AgentInfo {
	result := make([]router.AgentInfo, len(agents))
	for i, a := range agents {
		result[i] = router.AgentInfo{
			ID:           a.ID,
			Name:         a.Name,
			Description:  a.Description,
			Keywords:     a.Keywords,
			Capabilities: a.Capabilities,
		}
	}
	return result
}
