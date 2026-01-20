package realtime

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	cfg := Config{}
	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager should not error: %v", err)
	}
	if mgr == nil {
		t.Fatal("NewManager should not return nil")
	}
	if mgr.sessions == nil {
		t.Error("sessions map should be initialized")
	}
}

func TestNewManager_WithICEServers(t *testing.T) {
	cfg := Config{
		ICEServers: []ICEServerConfig{
			{URLs: []string{"stun:stun.example.com"}},
		},
	}
	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager should not error: %v", err)
	}
	if len(mgr.cfg.ICEServers) != 1 {
		t.Errorf("expected 1 ICE server, got %d", len(mgr.cfg.ICEServers))
	}
}

func TestNewManager_WithPortRange(t *testing.T) {
	cfg := Config{
		PortRange: PortRange{
			Min: 10000,
			Max: 20000,
		},
	}
	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager should not error: %v", err)
	}
	if mgr.cfg.PortRange.Min != 10000 {
		t.Errorf("expected port min 10000, got %d", mgr.cfg.PortRange.Min)
	}
	if mgr.cfg.PortRange.Max != 20000 {
		t.Errorf("expected port max 20000, got %d", mgr.cfg.PortRange.Max)
	}
}

func TestNewManager_InvalidPortRange(t *testing.T) {
	cfg := Config{
		PortRange: PortRange{
			Min: 20000,
			Max: 10000,
		},
	}
	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager should not error with invalid port range: %v", err)
	}
	if mgr == nil {
		t.Fatal("should still create manager")
	}
}

func TestManager_Config(t *testing.T) {
	cfg := Config{
		ICEServers: []ICEServerConfig{
			{URLs: []string{"stun:stun.example.com"}},
		},
		PortRange: PortRange{Min: 5000, Max: 6000},
	}
	mgr, _ := NewManager(cfg)

	returnedCfg := mgr.Config()
	if len(returnedCfg.ICEServers) != 1 {
		t.Errorf("expected 1 ICE server, got %d", len(returnedCfg.ICEServers))
	}
	if returnedCfg.PortRange.Min != 5000 {
		t.Errorf("expected port min 5000, got %d", returnedCfg.PortRange.Min)
	}
}

func TestManager_ICEServers(t *testing.T) {
	cfg := Config{
		ICEServers: []ICEServerConfig{
			{URLs: []string{"stun:stun1.example.com"}},
			{URLs: []string{"turn:turn.example.com"}, Username: "user", Credential: "pass"},
		},
	}
	mgr, _ := NewManager(cfg)

	servers := mgr.ICEServers()
	if len(servers) != 2 {
		t.Errorf("expected 2 ICE servers, got %d", len(servers))
	}
	if servers[0].URLs[0] != "stun:stun1.example.com" {
		t.Errorf("expected first server URL 'stun:stun1.example.com', got %s", servers[0].URLs[0])
	}
	if servers[1].Username != "user" {
		t.Errorf("expected second server username 'user', got %s", servers[1].Username)
	}
}

func TestManager_GetSession_NotFound(t *testing.T) {
	mgr, _ := NewManager(Config{})

	session, ok := mgr.GetSession("nonexistent")
	if ok {
		t.Error("should not find nonexistent session")
	}
	if session != nil {
		t.Error("session should be nil for nonexistent ID")
	}
}

func TestManager_RemoveSession_Nonexistent(t *testing.T) {
	mgr, _ := NewManager(Config{})
	mgr.RemoveSession("nonexistent")
}

func TestManager_iceServers_Default(t *testing.T) {
	cfg := Config{}
	mgr, _ := NewManager(cfg)

	servers := mgr.iceServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 default ICE server, got %d", len(servers))
	}
	if servers[0].URLs[0] != "stun:stun.l.google.com:19302" {
		t.Errorf("expected default STUN server, got %s", servers[0].URLs[0])
	}
}

func TestManager_iceServers_WithCredentials(t *testing.T) {
	cfg := Config{
		ICEServers: []ICEServerConfig{
			{
				URLs:       []string{"turn:turn.example.com"},
				Username:   "testuser",
				Credential: "testpass",
			},
		},
	}
	mgr, _ := NewManager(cfg)

	servers := mgr.iceServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 ICE server, got %d", len(servers))
	}
	if servers[0].Username != "testuser" {
		t.Errorf("expected username 'testuser', got %s", servers[0].Username)
	}
}

func TestManager_iceServers_Mixed(t *testing.T) {
	cfg := Config{
		ICEServers: []ICEServerConfig{
			{URLs: []string{"stun:stun.example.com"}},
			{
				URLs:       []string{"turn:turn.example.com"},
				Username:   "user",
				Credential: "pass",
			},
		},
	}
	mgr, _ := NewManager(cfg)

	servers := mgr.iceServers()
	if len(servers) != 2 {
		t.Fatalf("expected 2 ICE servers, got %d", len(servers))
	}
	if servers[0].Username != "" {
		t.Error("first server should not have credentials")
	}
	if servers[1].Username != "user" {
		t.Error("second server should have credentials")
	}
}

func TestConfig_BufferSizes(t *testing.T) {
	cfg := Config{
		BufferSizes: BufferSizes{
			AudioFrames:   256,
			Events:        64,
			ICECandidates: 32,
		},
	}
	mgr, _ := NewManager(cfg)
	if mgr.cfg.BufferSizes.AudioFrames != 256 {
		t.Errorf("expected AudioFrames 256, got %d", mgr.cfg.BufferSizes.AudioFrames)
	}
	if mgr.cfg.BufferSizes.Events != 64 {
		t.Errorf("expected Events 64, got %d", mgr.cfg.BufferSizes.Events)
	}
	if mgr.cfg.BufferSizes.ICECandidates != 32 {
		t.Errorf("expected ICECandidates 32, got %d", mgr.cfg.BufferSizes.ICECandidates)
	}
}
