package session

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/auth"
	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func newTestSessionHandler() *Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(nil, nil, nil, logger)
	return h
}

func setSessionAuthClaims(c echo.Context, userID string) {
	claims := &auth.Claims{
		UserID: userID,
		Email:  userID + "@test.com",
		Name:   "Test User",
	}
	auth.SetClaimsForTest(c, claims)
}

func TestNewSessionHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(nil, nil, nil, logger)

	if h == nil {
		t.Fatal("handler should not be nil")
	}
}

func TestSessionHandler_RegisterRoutes(t *testing.T) {
	h := newTestSessionHandler()
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
	h := newTestSessionHandler()
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
	h := newTestSessionHandler()
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

func newTestSessionHandlerWithDB(t *testing.T) (*Handler, *Store, *user.Store, *agent.Store, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	userStore := user.NewStore(db)
	userStore.Migrate()
	agentStore := agent.NewStore(db, nil)
	agentStore.Migrate()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sessionStore := NewStore(redisClient)

	h := NewHandler(sessionStore, agentStore, userStore, logger)
	return h, sessionStore, userStore, agentStore, mr
}

func TestSessionHandler_GetMetrics_UserNotFound(t *testing.T) {
	h, _, _, _, mr := newTestSessionHandlerWithDB(t)
	defer mr.Close()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/agent_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	setSessionAuthClaims(c, "user_nonexistent")

	err := h.GetMetrics(c)
	if err == nil {
		t.Fatal("expected error when user not found")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestSessionHandler_GetMetrics_NotDeveloper(t *testing.T) {
	h, _, userStore, _, mr := newTestSessionHandlerWithDB(t)
	defer mr.Close()

	userID := "user_regular123"
	ctx := context.Background()
	userStore.Create(ctx, &user.User{
		ID:          userID,
		Email:       "regular@test.com",
		IsDeveloper: false,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/agent_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	setSessionAuthClaims(c, userID)

	err := h.GetMetrics(c)
	if err == nil {
		t.Fatal("expected error when user is not developer")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, httpErr.Code)
	}
}

func TestSessionHandler_GetMetrics_AgentNotFound(t *testing.T) {
	h, _, userStore, _, mr := newTestSessionHandlerWithDB(t)
	defer mr.Close()

	userID := "user_dev123"
	ctx := context.Background()
	userStore.Create(ctx, &user.User{
		ID:          userID,
		Email:       "dev@test.com",
		IsDeveloper: true,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/agent_nonexistent", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_nonexistent")

	setSessionAuthClaims(c, userID)

	err := h.GetMetrics(c)
	if err == nil {
		t.Fatal("expected error when agent not found")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestSessionHandler_GetMetrics_NotOwner(t *testing.T) {
	h, _, userStore, agentStore, mr := newTestSessionHandlerWithDB(t)
	defer mr.Close()

	userID := "user_dev123"
	otherUserID := "user_other456"
	ctx := context.Background()
	userStore.Create(ctx, &user.User{
		ID:          userID,
		Email:       "dev@test.com",
		IsDeveloper: true,
	})
	userStore.Create(ctx, &user.User{
		ID:          otherUserID,
		Email:       "other@test.com",
		IsDeveloper: true,
	})

	agentStore.Create(ctx, &agent.Agent{
		ID:          "agent_123",
		DeveloperID: otherUserID,
		Name:        "Other Agent",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/agent_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	setSessionAuthClaims(c, userID)

	err := h.GetMetrics(c)
	if err == nil {
		t.Fatal("expected error when user doesn't own agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, httpErr.Code)
	}
}

func TestSessionHandler_GetMetrics_Success(t *testing.T) {
	h, sessionStore, userStore, agentStore, mr := newTestSessionHandlerWithDB(t)
	defer mr.Close()

	userID := "user_dev123"
	agentID := "agent_123"
	ctx := context.Background()
	userStore.Create(ctx, &user.User{
		ID:          userID,
		Email:       "dev@test.com",
		IsDeveloper: true,
	})

	agentStore.Create(ctx, &agent.Agent{
		ID:          agentID,
		DeveloperID: userID,
		Name:        "My Agent",
	})

	sessionStore.IncrementSessions(ctx, agentID)
	sessionStore.IncrementUtterances(ctx, agentID)
	sessionStore.IncrementResponses(ctx, agentID)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/"+agentID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(agentID)

	setSessionAuthClaims(c, userID)

	err := h.GetMetrics(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response dto.MetricsListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.AgentID != agentID {
		t.Errorf("expected AgentID %s, got %s", agentID, response.AgentID)
	}
	if response.Hours != 24 {
		t.Errorf("expected Hours 24, got %d", response.Hours)
	}
}

func TestSessionHandler_GetMetrics_CustomHours(t *testing.T) {
	h, _, userStore, agentStore, mr := newTestSessionHandlerWithDB(t)
	defer mr.Close()

	userID := "user_dev123"
	agentID := "agent_123"
	ctx := context.Background()
	userStore.Create(ctx, &user.User{
		ID:          userID,
		Email:       "dev@test.com",
		IsDeveloper: true,
	})

	agentStore.Create(ctx, &agent.Agent{
		ID:          agentID,
		DeveloperID: userID,
		Name:        "My Agent",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/"+agentID+"?hours=48", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(agentID)

	setSessionAuthClaims(c, userID)

	err := h.GetMetrics(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var response dto.MetricsListResponse
	json.Unmarshal(rec.Body.Bytes(), &response)

	if response.Hours != 48 {
		t.Errorf("expected Hours 48, got %d", response.Hours)
	}
}

func TestSessionHandler_GetMetrics_InvalidHours(t *testing.T) {
	h, _, userStore, agentStore, mr := newTestSessionHandlerWithDB(t)
	defer mr.Close()

	userID := "user_dev123"
	agentID := "agent_123"
	ctx := context.Background()
	userStore.Create(ctx, &user.User{
		ID:          userID,
		Email:       "dev@test.com",
		IsDeveloper: true,
	})

	agentStore.Create(ctx, &agent.Agent{
		ID:          agentID,
		DeveloperID: userID,
		Name:        "My Agent",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/"+agentID+"?hours=invalid", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(agentID)

	setSessionAuthClaims(c, userID)

	err := h.GetMetrics(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var response dto.MetricsListResponse
	json.Unmarshal(rec.Body.Bytes(), &response)

	if response.Hours != 24 {
		t.Errorf("expected default Hours 24 for invalid input, got %d", response.Hours)
	}
}

func TestSessionHandler_GetMetrics_HoursExceedsMax(t *testing.T) {
	h, _, userStore, agentStore, mr := newTestSessionHandlerWithDB(t)
	defer mr.Close()

	userID := "user_dev123"
	agentID := "agent_123"
	ctx := context.Background()
	userStore.Create(ctx, &user.User{
		ID:          userID,
		Email:       "dev@test.com",
		IsDeveloper: true,
	})

	agentStore.Create(ctx, &agent.Agent{
		ID:          agentID,
		DeveloperID: userID,
		Name:        "My Agent",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/"+agentID+"?hours=500", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(agentID)

	setSessionAuthClaims(c, userID)

	err := h.GetMetrics(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var response dto.MetricsListResponse
	json.Unmarshal(rec.Body.Bytes(), &response)

	if response.Hours != 24 {
		t.Errorf("expected default Hours 24 when exceeds max, got %d", response.Hours)
	}
}

func TestSessionHandler_GetSummary_Success(t *testing.T) {
	h, sessionStore, userStore, agentStore, mr := newTestSessionHandlerWithDB(t)
	defer mr.Close()

	userID := "user_dev123"
	agentID := "agent_123"
	ctx := context.Background()
	userStore.Create(ctx, &user.User{
		ID:          userID,
		Email:       "dev@test.com",
		IsDeveloper: true,
	})

	agentStore.Create(ctx, &agent.Agent{
		ID:          agentID,
		DeveloperID: userID,
		Name:        "My Agent",
	})

	sessionStore.IncrementSessions(ctx, agentID)
	sessionStore.IncrementSessions(ctx, agentID)
	sessionStore.IncrementUtterances(ctx, agentID)
	sessionStore.IncrementResponses(ctx, agentID)
	sessionStore.RecordLatency(ctx, agentID, 100)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/"+agentID+"/summary", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(agentID)

	setSessionAuthClaims(c, userID)

	err := h.GetSummary(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response dto.SummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.AgentID != agentID {
		t.Errorf("expected AgentID %s, got %s", agentID, response.AgentID)
	}
	if response.Period != "7d" {
		t.Errorf("expected Period '7d', got '%s'", response.Period)
	}
	if response.TotalSessions != 2 {
		t.Errorf("expected TotalSessions 2, got %d", response.TotalSessions)
	}
}

func TestSessionHandler_GetSummary_EmptyMetrics(t *testing.T) {
	h, _, userStore, agentStore, mr := newTestSessionHandlerWithDB(t)
	defer mr.Close()

	userID := "user_dev123"
	agentID := "agent_123"
	ctx := context.Background()
	userStore.Create(ctx, &user.User{
		ID:          userID,
		Email:       "dev@test.com",
		IsDeveloper: true,
	})

	agentStore.Create(ctx, &agent.Agent{
		ID:          agentID,
		DeveloperID: userID,
		Name:        "My Agent",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/"+agentID+"/summary", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(agentID)

	setSessionAuthClaims(c, userID)

	err := h.GetSummary(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var response dto.SummaryResponse
	json.Unmarshal(rec.Body.Bytes(), &response)

	if response.TotalSessions != 0 {
		t.Errorf("expected TotalSessions 0, got %d", response.TotalSessions)
	}
	if response.ErrorRate != 0 {
		t.Errorf("expected ErrorRate 0, got %f", response.ErrorRate)
	}
}

func TestSessionHandler_GetSummary_NotOwner(t *testing.T) {
	h, _, userStore, agentStore, mr := newTestSessionHandlerWithDB(t)
	defer mr.Close()

	userID := "user_dev123"
	otherUserID := "user_other456"
	ctx := context.Background()
	userStore.Create(ctx, &user.User{
		ID:          userID,
		Email:       "dev@test.com",
		IsDeveloper: true,
	})
	userStore.Create(ctx, &user.User{
		ID:          otherUserID,
		Email:       "other@test.com",
		IsDeveloper: true,
	})

	agentStore.Create(ctx, &agent.Agent{
		ID:          "agent_123",
		DeveloperID: otherUserID,
		Name:        "Other Agent",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/metrics/agents/agent_123/summary", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	setSessionAuthClaims(c, userID)

	err := h.GetSummary(c)
	if err == nil {
		t.Fatal("expected error when user doesn't own agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, httpErr.Code)
	}
}

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return NewStore(redisClient), mr
}

func TestStore_NewStore(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	if store == nil {
		t.Fatal("store should not be nil")
	}
}

func TestStore_CreateSession(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	sess := &Session{
		UserID:       "user_123",
		AgentID:      "agent_456",
		ConnectionID: "conn_789",
	}

	err := store.CreateSession(ctx, sess)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	if sess.ID == "" {
		t.Error("session ID should be generated")
	}
	if !strings.HasPrefix(sess.ID, "sess_") {
		t.Errorf("session ID should have prefix 'sess_', got %s", sess.ID)
	}
	if sess.Status != StatusActive {
		t.Errorf("expected status %s, got %s", StatusActive, sess.Status)
	}
	if sess.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
	if sess.LastActiveAt.IsZero() {
		t.Error("LastActiveAt should be set")
	}
}

func TestStore_CreateSession_WithID(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	sess := &Session{
		ID:           "sess_existing",
		UserID:       "user_123",
		AgentID:      "agent_456",
		ConnectionID: "conn_789",
	}

	err := store.CreateSession(ctx, sess)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	if sess.ID != "sess_existing" {
		t.Errorf("session ID should not be changed, got %s", sess.ID)
	}
}

func TestStore_GetSession(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	sess := &Session{
		ID:           "sess_get_test",
		UserID:       "user_123",
		AgentID:      "agent_456",
		ConnectionID: "conn_789",
	}
	store.CreateSession(ctx, sess)

	retrieved, err := store.GetSession(ctx, "sess_get_test")
	if err != nil {
		t.Fatalf("GetSession error: %v", err)
	}

	if retrieved.ID != sess.ID {
		t.Errorf("expected ID %s, got %s", sess.ID, retrieved.ID)
	}
	if retrieved.UserID != sess.UserID {
		t.Errorf("expected UserID %s, got %s", sess.UserID, retrieved.UserID)
	}
	if retrieved.AgentID != sess.AgentID {
		t.Errorf("expected AgentID %s, got %s", sess.AgentID, retrieved.AgentID)
	}
}

func TestStore_GetSession_NotFound(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	_, err := store.GetSession(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
}

func TestStore_UpdateSession(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	sess := &Session{
		ID:           "sess_update_test",
		UserID:       "user_123",
		AgentID:      "agent_456",
		ConnectionID: "conn_789",
	}
	store.CreateSession(ctx, sess)

	sess.Status = StatusEnded
	err := store.UpdateSession(ctx, sess)
	if err != nil {
		t.Fatalf("UpdateSession error: %v", err)
	}

	retrieved, _ := store.GetSession(ctx, "sess_update_test")
	if retrieved.Status != StatusEnded {
		t.Errorf("expected status %s, got %s", StatusEnded, retrieved.Status)
	}
}

func TestStore_EndSession(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	sess := &Session{
		ID:           "sess_end_test",
		UserID:       "user_123",
		AgentID:      "agent_456",
		ConnectionID: "conn_789",
	}
	store.CreateSession(ctx, sess)

	err := store.EndSession(ctx, "sess_end_test", StatusError)
	if err != nil {
		t.Fatalf("EndSession error: %v", err)
	}

	retrieved, _ := store.GetSession(ctx, "sess_end_test")
	if retrieved.Status != StatusError {
		t.Errorf("expected status %s, got %s", StatusError, retrieved.Status)
	}
}

func TestStore_EndSession_NotFound(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	err := store.EndSession(ctx, "nonexistent", StatusEnded)
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
}

func TestStore_DeleteSession(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	sess := &Session{
		ID:           "sess_delete_test",
		UserID:       "user_123",
		AgentID:      "agent_456",
		ConnectionID: "conn_789",
	}
	store.CreateSession(ctx, sess)

	err := store.DeleteSession(ctx, "sess_delete_test")
	if err != nil {
		t.Fatalf("DeleteSession error: %v", err)
	}

	_, err = store.GetSession(ctx, "sess_delete_test")
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestStore_GetActiveSessions(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()

	store.CreateSession(ctx, &Session{
		UserID:       "user_active",
		AgentID:      "agent_1",
		ConnectionID: "conn_1",
	})
	store.CreateSession(ctx, &Session{
		UserID:       "user_active",
		AgentID:      "agent_2",
		ConnectionID: "conn_2",
	})
	store.CreateSession(ctx, &Session{
		UserID:       "user_other",
		AgentID:      "agent_3",
		ConnectionID: "conn_3",
	})

	sessions, err := store.GetActiveSessions(ctx, "user_active")
	if err != nil {
		t.Fatalf("GetActiveSessions error: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestStore_IncrementMetric(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	agentID := "agent_metric"

	err := store.IncrementMetric(ctx, agentID, "sessions", 5)
	if err != nil {
		t.Fatalf("IncrementMetric error: %v", err)
	}

	err = store.IncrementMetric(ctx, agentID, "sessions", 3)
	if err != nil {
		t.Fatalf("IncrementMetric error: %v", err)
	}

	metrics, _ := store.GetMetrics(ctx, agentID, 1)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric entry, got %d", len(metrics))
	}
	if metrics[0].Sessions != 8 {
		t.Errorf("expected sessions 8, got %d", metrics[0].Sessions)
	}
}

func TestStore_IncrementSessions(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	agentID := "agent_sessions"

	store.IncrementSessions(ctx, agentID)
	store.IncrementSessions(ctx, agentID)

	metrics, _ := store.GetMetrics(ctx, agentID, 1)
	if len(metrics) == 0 {
		t.Fatal("expected metrics")
	}
	if metrics[0].Sessions != 2 {
		t.Errorf("expected sessions 2, got %d", metrics[0].Sessions)
	}
}

func TestStore_IncrementUtterances(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	agentID := "agent_utterances"

	store.IncrementUtterances(ctx, agentID)

	metrics, _ := store.GetMetrics(ctx, agentID, 1)
	if len(metrics) == 0 {
		t.Fatal("expected metrics")
	}
	if metrics[0].Utterances != 1 {
		t.Errorf("expected utterances 1, got %d", metrics[0].Utterances)
	}
}

func TestStore_IncrementResponses(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	agentID := "agent_responses"

	store.IncrementResponses(ctx, agentID)

	metrics, _ := store.GetMetrics(ctx, agentID, 1)
	if len(metrics) == 0 {
		t.Fatal("expected metrics")
	}
	if metrics[0].Responses != 1 {
		t.Errorf("expected responses 1, got %d", metrics[0].Responses)
	}
}

func TestStore_IncrementErrors(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	agentID := "agent_errors"

	store.IncrementErrors(ctx, agentID)
	store.IncrementErrors(ctx, agentID)

	metrics, _ := store.GetMetrics(ctx, agentID, 1)
	if len(metrics) == 0 {
		t.Fatal("expected metrics")
	}
	if metrics[0].ErrorCount != 2 {
		t.Errorf("expected error count 2, got %d", metrics[0].ErrorCount)
	}
}

func TestStore_TrackUniqueUser(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	agentID := "agent_unique"

	store.TrackUniqueUser(ctx, agentID, "user_1")
	store.TrackUniqueUser(ctx, agentID, "user_2")
	store.TrackUniqueUser(ctx, agentID, "user_1")

	metrics, _ := store.GetMetrics(ctx, agentID, 1)
	if len(metrics) == 0 {
		t.Fatal("expected metrics")
	}
	if metrics[0].UniqueUsers != 2 {
		t.Errorf("expected unique users 2, got %d", metrics[0].UniqueUsers)
	}
}

func TestStore_RecordLatency(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	agentID := "agent_latency"

	store.RecordLatency(ctx, agentID, 100)
	store.RecordLatency(ctx, agentID, 200)

	metrics, _ := store.GetMetrics(ctx, agentID, 1)
	if len(metrics) == 0 {
		t.Fatal("expected metrics")
	}
	if metrics[0].AvgLatencyMs != 150 {
		t.Errorf("expected avg latency 150, got %d", metrics[0].AvgLatencyMs)
	}
}

func TestStore_GetMetrics_Empty(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	metrics, err := store.GetMetrics(ctx, "nonexistent_agent", 24)
	if err != nil {
		t.Fatalf("GetMetrics error: %v", err)
	}

	if len(metrics) != 0 {
		t.Errorf("expected empty metrics, got %d entries", len(metrics))
	}
}

func TestStore_GetMetricsForLast7Days(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	ctx := context.Background()
	agentID := "agent_7days"

	store.IncrementSessions(ctx, agentID)

	metrics, err := store.GetMetricsForLast7Days(ctx, agentID)
	if err != nil {
		t.Fatalf("GetMetricsForLast7Days error: %v", err)
	}

	found := false
	for _, m := range metrics {
		if m.Sessions > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find metrics with sessions")
	}
}
