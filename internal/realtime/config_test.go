package realtime

import (
	"testing"
)

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{}
	if cfg.ICEServers != nil {
		t.Error("ICEServers should be nil by default")
	}
	if cfg.MaxSDPSize != 0 {
		t.Error("MaxSDPSize should be 0 by default")
	}
}

func TestICEServerConfig(t *testing.T) {
	ice := ICEServerConfig{
		URLs:       []string{"stun:stun.l.google.com:19302"},
		Username:   "user",
		Credential: "pass",
	}
	if len(ice.URLs) != 1 {
		t.Errorf("expected 1 URL, got %d", len(ice.URLs))
	}
	if ice.Username != "user" {
		t.Errorf("expected username 'user', got %s", ice.Username)
	}
	if ice.Credential != "pass" {
		t.Errorf("expected credential 'pass', got %s", ice.Credential)
	}
}

func TestPortRange(t *testing.T) {
	pr := PortRange{
		Min: 10000,
		Max: 20000,
	}
	if pr.Min != 10000 {
		t.Errorf("expected Min 10000, got %d", pr.Min)
	}
	if pr.Max != 20000 {
		t.Errorf("expected Max 20000, got %d", pr.Max)
	}
}

func TestBufferSizes(t *testing.T) {
	bs := BufferSizes{
		AudioFrames:   128,
		Events:        64,
		ICECandidates: 32,
	}
	if bs.AudioFrames != 128 {
		t.Errorf("expected AudioFrames 128, got %d", bs.AudioFrames)
	}
	if bs.Events != 64 {
		t.Errorf("expected Events 64, got %d", bs.Events)
	}
	if bs.ICECandidates != 32 {
		t.Errorf("expected ICECandidates 32, got %d", bs.ICECandidates)
	}
}

func TestConfig_WithICEServers(t *testing.T) {
	cfg := Config{
		ICEServers: []ICEServerConfig{
			{URLs: []string{"stun:stun1.example.com"}},
			{URLs: []string{"turn:turn.example.com"}, Username: "u", Credential: "p"},
		},
		PortRange: PortRange{Min: 5000, Max: 6000},
		BufferSizes: BufferSizes{
			AudioFrames:   256,
			Events:        128,
			ICECandidates: 64,
		},
		MaxSDPSize: 65536,
	}
	if len(cfg.ICEServers) != 2 {
		t.Errorf("expected 2 ICE servers, got %d", len(cfg.ICEServers))
	}
	if cfg.PortRange.Min != 5000 {
		t.Errorf("expected port min 5000, got %d", cfg.PortRange.Min)
	}
	if cfg.MaxSDPSize != 65536 {
		t.Errorf("expected MaxSDPSize 65536, got %d", cfg.MaxSDPSize)
	}
}
