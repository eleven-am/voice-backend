package voicesession

import (
	"io"
	"log/slog"
	"testing"

	"github.com/eleven-am/voice-backend/internal/router"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	if mgr == nil {
		t.Fatal("NewManager should not return nil")
	}
	if mgr.sessions == nil {
		t.Error("sessions map should be initialized")
	}
	if mgr.log == nil {
		t.Error("logger should not be nil")
	}
	if mgr.router == nil {
		t.Error("router should not be nil (default SmartRouter)")
	}
}

func TestNewManager_WithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mgr := NewManager(ManagerConfig{
		Log: logger,
	})
	if mgr.log == nil {
		t.Error("logger should not be nil")
	}
}

func TestNewManager_WithRouter(t *testing.T) {
	customRouter := router.NewSmartRouter()
	mgr := NewManager(ManagerConfig{
		Router: customRouter,
	})
	if mgr.router != customRouter {
		t.Error("manager should use the provided router")
	}
}

func TestNewManager_WithAgents(t *testing.T) {
	agents := []router.AgentInfo{
		{ID: "agent1", Name: "Agent 1"},
		{ID: "agent2", Name: "Agent 2"},
	}
	mgr := NewManager(ManagerConfig{
		Agents: agents,
	})
	if len(mgr.defaultAgents) != 2 {
		t.Errorf("expected 2 default agents, got %d", len(mgr.defaultAgents))
	}
}

func TestManager_GetSession_NotFound(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	session, ok := mgr.GetSession("nonexistent")
	if ok {
		t.Error("should not find nonexistent session")
	}
	if session != nil {
		t.Error("session should be nil for nonexistent ID")
	}
}

func TestManager_RemoveSession_Nonexistent(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	mgr.RemoveSession("nonexistent")
}

func TestManager_SetAgents(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	agents := []router.AgentInfo{
		{ID: "agent1", Name: "Agent 1", Keywords: []string{"help"}},
		{ID: "agent2", Name: "Agent 2", Keywords: []string{"support"}},
	}
	mgr.SetAgents(agents)
	if len(mgr.defaultAgents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(mgr.defaultAgents))
	}
}

func TestManager_UpdateHealth(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	health := map[string]router.HealthMetrics{
		"agent1": {LatencyMs: 100, Healthy: true},
	}
	mgr.UpdateHealth(health)
}

func TestManager_Close(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	err := mgr.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
	if len(mgr.sessions) != 0 {
		t.Error("sessions should be cleared after Close")
	}
}

func TestManager_Close_MultipleTimes(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	mgr.Close()
	err := mgr.Close()
	if err != nil {
		t.Errorf("second Close should not error: %v", err)
	}
}
