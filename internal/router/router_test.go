package router

import (
	"context"
	"testing"
	"time"
)

func TestNewSmartRouter(t *testing.T) {
	r := NewSmartRouter()
	if r == nil {
		t.Fatal("expected non-nil router")
	}
	if r.health == nil {
		t.Error("health map should be initialized")
	}
	if r.index == nil {
		t.Error("index map should be initialized")
	}
}

func TestSmartRouter_SetHealth(t *testing.T) {
	r := NewSmartRouter()
	health := map[string]HealthMetrics{
		"agent_1": {LatencyMs: 100, Healthy: true},
		"agent_2": {LatencyMs: 200, Healthy: true},
	}
	r.SetHealth(health)

	if len(r.health) != 2 {
		t.Errorf("expected 2 health entries, got %d", len(r.health))
	}
}

func TestSmartRouter_Index(t *testing.T) {
	r := NewSmartRouter()
	agents := []AgentInfo{
		{
			ID:           "agent_weather",
			Name:         "Weather Agent",
			Keywords:     []string{"weather", "forecast"},
			Capabilities: []string{"get_weather"},
			Description:  "Provides weather information",
		},
		{
			ID:           "agent_calendar",
			Name:         "Calendar Agent",
			Keywords:     []string{"calendar", "schedule"},
			Capabilities: []string{"manage_events"},
			Description:  "Manages calendar events",
		},
	}

	r.Index(agents)

	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.agentIDs) != 2 {
		t.Errorf("expected 2 agent IDs, got %d", len(r.agentIDs))
	}

	if _, ok := r.index["weather"]; !ok {
		t.Error("expected 'weather' to be indexed")
	}

	if _, ok := r.index["calendar"]; !ok {
		t.Error("expected 'calendar' to be indexed")
	}
}

func TestSmartRouter_Route_EmptyAgents(t *testing.T) {
	r := NewSmartRouter()
	result := r.Route(context.Background(), "test query", nil)
	if result != nil {
		t.Errorf("expected nil for empty agents, got %v", result)
	}
}

func TestSmartRouter_Route_SingleAgent(t *testing.T) {
	r := NewSmartRouter()
	agents := []AgentInfo{
		{ID: "only_agent", Name: "Only Agent"},
	}

	result := r.Route(context.Background(), "any query", agents)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0] != "only_agent" {
		t.Errorf("expected 'only_agent', got %s", result[0])
	}
}

func TestSmartRouter_Route_EmptyRequest(t *testing.T) {
	r := NewSmartRouter()
	agents := []AgentInfo{
		{ID: "agent_1", Name: "Agent 1"},
		{ID: "agent_2", Name: "Agent 2"},
	}

	health := map[string]HealthMetrics{
		"agent_1": {LatencyMs: 200, Healthy: true},
		"agent_2": {LatencyMs: 100, Healthy: true},
	}
	r.SetHealth(health)

	result := r.Route(context.Background(), "", agents)
	if len(result) != 1 {
		t.Fatalf("expected 1 result for empty request with health, got %d", len(result))
	}
	if result[0] != "agent_2" {
		t.Errorf("expected 'agent_2' (lower latency), got %s", result[0])
	}
}

func TestSmartRouter_Route_KeywordMatch(t *testing.T) {
	r := NewSmartRouter()
	agents := []AgentInfo{
		{
			ID:       "agent_weather",
			Name:     "Weather Agent",
			Keywords: []string{"weather", "forecast"},
		},
		{
			ID:       "agent_calendar",
			Name:     "Calendar Agent",
			Keywords: []string{"calendar", "schedule"},
		},
	}
	r.Index(agents)

	result := r.Route(context.Background(), "what's the weather today", agents)
	if len(result) == 0 {
		t.Fatal("expected at least one result")
	}
	if result[0] != "agent_weather" {
		t.Errorf("expected weather agent to be first, got %s", result[0])
	}
}

func TestSmartRouter_Route_MultipleMatches(t *testing.T) {
	r := NewSmartRouter()
	agents := []AgentInfo{
		{
			ID:       "agent_1",
			Keywords: []string{"task", "work"},
		},
		{
			ID:       "agent_2",
			Keywords: []string{"task", "project"},
		},
	}
	r.Index(agents)

	result := r.Route(context.Background(), "complete my task", agents)
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
}

func TestSmartRouter_Route_HealthTiebreaker(t *testing.T) {
	r := NewSmartRouter()
	agents := []AgentInfo{
		{ID: "agent_1", Keywords: []string{"help"}},
		{ID: "agent_2", Keywords: []string{"help"}},
	}
	r.Index(agents)

	health := map[string]HealthMetrics{
		"agent_1": {LatencyMs: 300, Healthy: true},
		"agent_2": {LatencyMs: 100, Healthy: true},
	}
	r.SetHealth(health)

	result := r.Route(context.Background(), "help me", agents)
	if len(result) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(result))
	}
	if result[0] != "agent_2" {
		t.Errorf("expected agent_2 (lower latency) to be first, got %s", result[0])
	}
}

func TestSmartRouter_Route_CapabilityWeight(t *testing.T) {
	r := NewSmartRouter()
	agents := []AgentInfo{
		{
			ID:           "agent_1",
			Capabilities: []string{"search"},
		},
		{
			ID:          "agent_2",
			Description: "A search agent that finds things",
		},
	}
	r.Index(agents)

	result := r.Route(context.Background(), "search for something", agents)
	if len(result) == 0 {
		t.Fatal("expected at least one result")
	}
	if result[0] != "agent_1" {
		t.Errorf("expected agent_1 (capability weight higher), got %s", result[0])
	}
}

func TestSmartRouter_Route_NoTokensAfterTokenize(t *testing.T) {
	r := NewSmartRouter()
	agents := []AgentInfo{
		{ID: "agent_1", Name: "Agent 1"},
		{ID: "agent_2", Name: "Agent 2"},
	}

	health := map[string]HealthMetrics{
		"agent_1": {LatencyMs: 100, Healthy: true},
	}
	r.SetHealth(health)

	result := r.Route(context.Background(), "a b c", agents)
	if len(result) != 1 {
		t.Fatalf("expected 1 result for short tokens, got %d", len(result))
	}
}

func TestSmartRouter_Route_AutoIndex(t *testing.T) {
	r := NewSmartRouter()
	agents := []AgentInfo{
		{ID: "agent_1", Keywords: []string{"testing"}},
		{ID: "agent_2", Keywords: []string{"other"}},
	}

	r.mu.RLock()
	indexed := len(r.index)
	r.mu.RUnlock()

	if indexed != 0 {
		t.Error("expected empty index before routing")
	}

	result := r.Route(context.Background(), "testing query", agents)
	if len(result) == 0 {
		t.Error("expected result after auto-indexing")
	}

	r.mu.RLock()
	_, hasToken := r.index["testing"]
	r.mu.RUnlock()

	if !hasToken {
		t.Error("expected 'testing' token to be indexed after routing")
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"Hello World", []string{"hello", "world"}},
		{"a b c", nil},
		{"test123 example", []string{"test123", "example"}},
		{"punctuation, testing!", []string{"punctuation", "testing"}},
		{"", nil},
		{"word-with-dash", []string{"word", "with", "dash"}},
		{"CamelCase", []string{"camelcase"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("tokenize(%q) = %v, want %v", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSelectBestByHealth(t *testing.T) {
	agents := []AgentInfo{
		{ID: "agent_1"},
		{ID: "agent_2"},
		{ID: "agent_3"},
	}

	tests := []struct {
		name   string
		health map[string]HealthMetrics
		want   string
	}{
		{
			name:   "empty health",
			health: nil,
			want:   "",
		},
		{
			name: "single healthy agent",
			health: map[string]HealthMetrics{
				"agent_1": {LatencyMs: 100, Healthy: true},
			},
			want: "agent_1",
		},
		{
			name: "select lowest latency",
			health: map[string]HealthMetrics{
				"agent_1": {LatencyMs: 300, Healthy: true},
				"agent_2": {LatencyMs: 100, Healthy: true},
				"agent_3": {LatencyMs: 200, Healthy: true},
			},
			want: "agent_2",
		},
		{
			name: "skip unhealthy",
			health: map[string]HealthMetrics{
				"agent_1": {LatencyMs: 50, Healthy: false},
				"agent_2": {LatencyMs: 100, Healthy: true},
			},
			want: "agent_2",
		},
		{
			name: "all unhealthy",
			health: map[string]HealthMetrics{
				"agent_1": {LatencyMs: 50, Healthy: false},
				"agent_2": {LatencyMs: 100, Healthy: false},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectBestByHealth(agents, tt.health)
			if got != tt.want {
				t.Errorf("selectBestByHealth() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSmartRouter_CompareHealthLocked(t *testing.T) {
	r := NewSmartRouter()
	r.health = map[string]HealthMetrics{
		"healthy_fast":     {LatencyMs: 100, Healthy: true},
		"healthy_slow":     {LatencyMs: 300, Healthy: true},
		"unhealthy":        {LatencyMs: 50, Healthy: false},
	}

	tests := []struct {
		a, b string
		want bool
	}{
		{"healthy_fast", "healthy_slow", true},
		{"healthy_slow", "healthy_fast", false},
		{"healthy_fast", "unhealthy", true},
		{"unhealthy", "healthy_fast", false},
		{"unknown_a", "unknown_b", false},
		{"healthy_fast", "unknown_b", true},
		{"unknown_a", "healthy_fast", false},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := r.compareHealthLocked(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareHealthLocked(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSmartRouter_ScoreFromIndex(t *testing.T) {
	r := NewSmartRouter()
	agents := []AgentInfo{
		{
			ID:           "agent_1",
			Keywords:     []string{"weather"},
			Capabilities: []string{"forecast"},
		},
		{
			ID:       "agent_2",
			Keywords: []string{"calendar"},
		},
	}
	r.Index(agents)

	matches := r.scoreFromIndex([]string{"weather", "forecast"}, agents)

	found := false
	for _, m := range matches {
		if m.id == "agent_1" {
			found = true
			if m.score < 3 {
				t.Errorf("expected agent_1 score >= 3 (keyword), got %d", m.score)
			}
		}
	}
	if !found {
		t.Error("expected agent_1 to be in matches")
	}
}

func TestSmartRouter_SelectByHealth(t *testing.T) {
	r := NewSmartRouter()
	agents := []AgentInfo{
		{ID: "agent_1"},
		{ID: "agent_2"},
	}

	result := r.selectByHealth(agents)
	if result != nil {
		t.Errorf("expected nil when no health data, got %v", result)
	}

	r.SetHealth(map[string]HealthMetrics{
		"agent_1": {LatencyMs: 100, Healthy: true},
	})

	result = r.selectByHealth(agents)
	if len(result) != 1 || result[0] != "agent_1" {
		t.Errorf("expected [agent_1], got %v", result)
	}
}

func TestAgentInfo_Fields(t *testing.T) {
	agent := AgentInfo{
		ID:           "test_id",
		Model:        "gpt-4",
		Name:         "Test Agent",
		Description:  "A test agent",
		Keywords:     []string{"test"},
		Capabilities: []string{"testing"},
		Examples:     []string{"example query"},
	}

	if agent.ID != "test_id" {
		t.Error("ID not set")
	}
	if agent.Model != "gpt-4" {
		t.Error("Model not set")
	}
	if len(agent.Keywords) != 1 {
		t.Error("Keywords not set")
	}
}

func TestHealthMetrics_Fields(t *testing.T) {
	now := time.Now()
	metrics := HealthMetrics{
		LatencyMs: 150,
		Load:      0.75,
		Healthy:   true,
		UpdatedAt: now,
	}

	if metrics.LatencyMs != 150 {
		t.Error("LatencyMs not set")
	}
	if metrics.Load != 0.75 {
		t.Error("Load not set")
	}
	if !metrics.Healthy {
		t.Error("Healthy not set")
	}
	if metrics.UpdatedAt != now {
		t.Error("UpdatedAt not set")
	}
}

func TestRouteContext_Fields(t *testing.T) {
	ctx := RouteContext{
		RequestText: "hello",
		Agents:      []AgentInfo{{ID: "a1"}},
		Health:      map[string]HealthMetrics{"a1": {Healthy: true}},
	}

	if ctx.RequestText != "hello" {
		t.Error("RequestText not set")
	}
	if len(ctx.Agents) != 1 {
		t.Error("Agents not set")
	}
	if len(ctx.Health) != 1 {
		t.Error("Health not set")
	}
}
