package session

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
)

func newTestSessionHandler() (*Handler, *user.SessionManager) {
	sm := user.NewSessionManager([]byte("test-key"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(nil, nil, nil, sm, logger)
	return h, sm
}

func TestNewSessionHandler(t *testing.T) {
	sm := user.NewSessionManager([]byte("key"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(nil, nil, nil, sm, logger)

	if h == nil {
		t.Fatal("handler should not be nil")
	}
	if h.sessions != sm {
		t.Error("session manager should be set")
	}
}

func TestSessionHandler_RegisterRoutes(t *testing.T) {
	h, _ := newTestSessionHandler()
	e := echo.New()
	g := e.Group("/metrics")

	h.RegisterRoutes(g)

	routes := e.Routes()
	expectedPaths := []string{
		"/metrics/agents/:id",
		"/metrics/agents/:id/summary",
	}

	routePaths := make(map[string]bool)
	for _, r := range routes {
		routePaths[r.Path] = true
	}

	for _, path := range expectedPaths {
		if !routePaths[path] {
			t.Errorf("expected route %s to be registered", path)
		}
	}
}

func TestSessionHandler_GetMetrics_Unauthorized(t *testing.T) {
	h, _ := newTestSessionHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/agent_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	err := h.GetMetrics(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestSessionHandler_GetSummary_Unauthorized(t *testing.T) {
	h, _ := newTestSessionHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/agent_123/summary", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	err := h.GetSummary(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestMetricsResponse_JSON(t *testing.T) {
	resp := dto.MetricsResponse{
		AgentID:      "agent_123",
		Date:         "2024-01-15",
		Hour:         14,
		Sessions:     100,
		Utterances:   500,
		Responses:    450,
		UniqueUsers:  25,
		AvgLatencyMs: 150,
		ErrorCount:   5,
		NewInstalls:  10,
		Uninstalls:   2,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"agent_id":"agent_123"`) {
		t.Error("expected JSON to contain agent_id")
	}
	if !strings.Contains(jsonStr, `"date":"2024-01-15"`) {
		t.Error("expected JSON to contain date")
	}
	if !strings.Contains(jsonStr, `"hour":14`) {
		t.Error("expected JSON to contain hour")
	}
	if !strings.Contains(jsonStr, `"sessions":100`) {
		t.Error("expected JSON to contain sessions")
	}
	if !strings.Contains(jsonStr, `"avg_latency_ms":150`) {
		t.Error("expected JSON to contain avg_latency_ms")
	}
}

func TestSummaryResponse_JSON(t *testing.T) {
	resp := dto.SummaryResponse{
		AgentID:         "agent_123",
		Period:          "7d",
		TotalSessions:   1000,
		TotalUtterances: 5000,
		TotalResponses:  4500,
		UniqueUsers:     200,
		AvgLatencyMs:    145,
		ErrorRate:       1.5,
		NetInstalls:     50,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"agent_id":"agent_123"`) {
		t.Error("expected JSON to contain agent_id")
	}
	if !strings.Contains(jsonStr, `"period":"7d"`) {
		t.Error("expected JSON to contain period")
	}
	if !strings.Contains(jsonStr, `"total_sessions":1000`) {
		t.Error("expected JSON to contain total_sessions")
	}
	if !strings.Contains(jsonStr, `"error_rate":1.5`) {
		t.Error("expected JSON to contain error_rate")
	}
	if !strings.Contains(jsonStr, `"net_installs":50`) {
		t.Error("expected JSON to contain net_installs")
	}
}

func TestMetricsToResponse(t *testing.T) {
	m := &Metrics{
		AgentID:      "agent_123",
		Date:         "2024-01-15",
		Hour:         10,
		Sessions:     50,
		Utterances:   200,
		Responses:    180,
		UniqueUsers:  15,
		AvgLatencyMs: 120,
		ErrorCount:   3,
		NewInstalls:  5,
		Uninstalls:   1,
	}

	resp := metricsToResponse(m)

	if resp.AgentID != m.AgentID {
		t.Errorf("expected AgentID %s, got %s", m.AgentID, resp.AgentID)
	}
	if resp.Date != m.Date {
		t.Errorf("expected Date %s, got %s", m.Date, resp.Date)
	}
	if resp.Hour != m.Hour {
		t.Errorf("expected Hour %d, got %d", m.Hour, resp.Hour)
	}
	if resp.Sessions != m.Sessions {
		t.Errorf("expected Sessions %d, got %d", m.Sessions, resp.Sessions)
	}
	if resp.Utterances != m.Utterances {
		t.Errorf("expected Utterances %d, got %d", m.Utterances, resp.Utterances)
	}
	if resp.Responses != m.Responses {
		t.Errorf("expected Responses %d, got %d", m.Responses, resp.Responses)
	}
	if resp.UniqueUsers != m.UniqueUsers {
		t.Errorf("expected UniqueUsers %d, got %d", m.UniqueUsers, resp.UniqueUsers)
	}
	if resp.AvgLatencyMs != m.AvgLatencyMs {
		t.Errorf("expected AvgLatencyMs %d, got %d", m.AvgLatencyMs, resp.AvgLatencyMs)
	}
	if resp.ErrorCount != m.ErrorCount {
		t.Errorf("expected ErrorCount %d, got %d", m.ErrorCount, resp.ErrorCount)
	}
	if resp.NewInstalls != m.NewInstalls {
		t.Errorf("expected NewInstalls %d, got %d", m.NewInstalls, resp.NewInstalls)
	}
	if resp.Uninstalls != m.Uninstalls {
		t.Errorf("expected Uninstalls %d, got %d", m.Uninstalls, resp.Uninstalls)
	}
}

func TestMetrics_Fields(t *testing.T) {
	m := &Metrics{
		AgentID:      "agent_123",
		Date:         "2024-01-15",
		Hour:         12,
		Sessions:     100,
		Utterances:   500,
		Responses:    480,
		UniqueUsers:  30,
		AvgLatencyMs: 200,
		ErrorCount:   10,
		NewInstalls:  15,
		Uninstalls:   3,
	}

	if m.AgentID != "agent_123" {
		t.Errorf("expected AgentID 'agent_123', got '%s'", m.AgentID)
	}
	if m.Hour != 12 {
		t.Errorf("expected Hour 12, got %d", m.Hour)
	}
}

func TestMetricsRedisKey(t *testing.T) {
	key := MetricsRedisKey("agent_123", "2024-01-15", 14)
	expected := "agent:agent_123:metrics:2024-01-15:14"
	if key != expected {
		t.Errorf("expected '%s', got '%s'", expected, key)
	}
}

func TestStatus(t *testing.T) {
	if StatusActive != "active" {
		t.Errorf("expected StatusActive to be 'active', got '%s'", StatusActive)
	}
	if StatusEnded != "ended" {
		t.Errorf("expected StatusEnded to be 'ended', got '%s'", StatusEnded)
	}
	if StatusError != "error" {
		t.Errorf("expected StatusError to be 'error', got '%s'", StatusError)
	}
}

func TestSession_RedisKey(t *testing.T) {
	s := &Session{ID: "sess_abc123"}
	key := s.RedisKey()
	expected := "session:sess_abc123"
	if key != expected {
		t.Errorf("expected '%s', got '%s'", expected, key)
	}
}
