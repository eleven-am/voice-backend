package realtime

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strconv"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

type Manager struct {
	cfg Config
	api *webrtc.API

	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewManager(cfg Config) (*Manager, error) {
	me := &webrtc.MediaEngine{}

	if err := me.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}

	se := &webrtc.SettingEngine{}

	if cfg.PortRange.Min > 0 && cfg.PortRange.Max > cfg.PortRange.Min {
		if err := se.SetEphemeralUDPPortRange(uint16(cfg.PortRange.Min), uint16(cfg.PortRange.Max)); err != nil {
			return nil, err
		}
	}

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(me),
		webrtc.WithSettingEngine(*se),
	)

	return &Manager{
		cfg:      cfg,
		api:      api,
		sessions: make(map[string]*Session),
	}, nil
}

func (m *Manager) NewPeer() (*Peer, error) {
	pcConfig := webrtc.Configuration{
		ICEServers: m.iceServers(),
	}

	pc, err := m.api.NewPeerConnection(pcConfig)
	if err != nil {
		return nil, err
	}

	return NewPeer(pc)
}

func (m *Manager) iceServers() []webrtc.ICEServer {
	servers := make([]webrtc.ICEServer, 0, len(m.cfg.ICEServers)+1)
	for _, s := range m.cfg.ICEServers {
		server := webrtc.ICEServer{
			URLs: s.URLs,
		}
		if s.Username != "" {
			server.Username = s.Username
			server.Credential = s.Credential
			server.CredentialType = webrtc.ICECredentialTypePassword
		}
		servers = append(servers, server)
	}

	if m.cfg.TurnServer != "" && m.cfg.TurnSecret != "" {
		username, credential := m.generateTURNCredentials(m.cfg.TurnTTL)
		servers = append(servers, webrtc.ICEServer{
			URLs:           []string{"turn:" + m.cfg.TurnServer},
			Username:       username,
			Credential:     credential,
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}

	if len(servers) == 0 {
		servers = append(servers, webrtc.ICEServer{
			URLs: []string{"stun:stun.l.google.com:19302"},
		})
	}

	return servers
}

func (m *Manager) CreateSession(conn *Conn, userID string) *Session {
	iceBufSize := m.cfg.BufferSizes.ICECandidates
	if iceBufSize <= 0 {
		iceBufSize = 128
	}

	session := NewSession(conn, userID, iceBufSize, nil)

	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()

	return session
}

func (m *Manager) GetSession(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *Manager) RemoveSession(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.Close()
		delete(m.sessions, id)
	}
}

func (m *Manager) ICEServers() []ICEServerConfig {
	servers := make([]ICEServerConfig, 0, len(m.cfg.ICEServers)+1)
	servers = append(servers, m.cfg.ICEServers...)

	if m.cfg.TurnServer != "" && m.cfg.TurnSecret != "" {
		ttl := m.cfg.TurnTTL
		if ttl <= 0 {
			ttl = 86400
		}

		username, credential := m.generateTURNCredentials(ttl)
		servers = append(servers, ICEServerConfig{
			URLs:       []string{"turn:" + m.cfg.TurnServer},
			Username:   username,
			Credential: credential,
		})
	}

	return servers
}

func (m *Manager) generateTURNCredentials(ttlSeconds int) (username, credential string) {
	expiry := time.Now().Unix() + int64(ttlSeconds)
	username = strconv.FormatInt(expiry, 10)

	mac := hmac.New(sha1.New, []byte(m.cfg.TurnSecret))
	mac.Write([]byte(username))
	credential = base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return username, credential
}

func (m *Manager) Config() Config {
	return m.cfg
}
