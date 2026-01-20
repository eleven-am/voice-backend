package voicesession

import (
	"log/slog"
	"sync"

	"github.com/eleven-am/voice-backend/internal/router"
	"github.com/eleven-am/voice-backend/internal/transport"
)

type Manager struct {
	bridge   transport.Bridge
	router   router.Router
	sessions map[string]*VoiceSession
	mu       sync.RWMutex
	log      *slog.Logger

	defaultSTTConfig Config
	defaultTTSConfig Config
	defaultAgents    []router.AgentInfo
}

type ManagerConfig struct {
	Bridge    transport.Bridge
	Router    router.Router
	STTConfig Config
	TTSConfig Config
	Agents    []router.AgentInfo
	Log       *slog.Logger
}

func NewManager(cfg ManagerConfig) *Manager {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}

	var rtr router.Router
	if cfg.Router != nil {
		rtr = cfg.Router
	} else {
		rtr = router.NewSmartRouter()
	}

	return &Manager{
		bridge:           cfg.Bridge,
		router:           rtr,
		sessions:         make(map[string]*VoiceSession),
		log:              cfg.Log.With("component", "voicesession_manager"),
		defaultSTTConfig: cfg.STTConfig,
		defaultTTSConfig: cfg.TTSConfig,
		defaultAgents:    cfg.Agents,
	}
}

func (m *Manager) CreateSession(conn transport.Connection, userCtx *transport.UserContext, cfg Config) (*VoiceSession, error) {
	if cfg.UserID == "" && userCtx != nil {
		cfg.UserID = userCtx.UserID
	}

	if cfg.Router == nil {
		cfg.Router = m.router
	}

	if len(cfg.Agents) == 0 {
		cfg.Agents = m.defaultAgents
	}

	session, err := New(conn, m.bridge, cfg, m.log)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.sessions[session.SessionID()] = session
	m.mu.Unlock()

	session.Start()

	m.log.Info("voice session created", "session_id", session.SessionID(), "user_id", cfg.UserID)
	return session, nil
}

func (m *Manager) GetSession(sessionID string) (*VoiceSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[sessionID]
	return session, ok
}

func (m *Manager) RemoveSession(sessionID string) {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if ok {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()

	if session != nil {
		session.Close()
		m.log.Info("voice session removed", "session_id", sessionID)
	}
}

func (m *Manager) SetAgents(agents []router.AgentInfo) {
	m.mu.Lock()
	m.defaultAgents = agents
	m.mu.Unlock()

	if indexed, ok := m.router.(router.IndexedRouter); ok {
		indexed.Index(agents)
	}
}

func (m *Manager) UpdateHealth(health map[string]router.HealthMetrics) {
	if aware, ok := m.router.(router.HealthAwareRouter); ok {
		aware.SetHealth(health)
	}
}

type SessionInfo struct {
	SessionID   string `json:"session_id"`
	UserID      string `json:"user_id"`
	AgentID     string `json:"agent_id"`
	SpeechState string `json:"speech_state"`
}

func (m *Manager) SessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

func (m *Manager) ListSessions() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, SessionInfo{
			SessionID:   s.SessionID(),
			UserID:      s.UserID(),
			AgentID:     s.AgentID(),
			SpeechState: string(s.speechCtrl.State()),
		})
	}
	return sessions
}

func (m *Manager) Close() error {
	m.mu.Lock()
	sessions := make([]*VoiceSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = make(map[string]*VoiceSession)
	m.mu.Unlock()

	for _, s := range sessions {
		s.Close()
	}
	return nil
}
