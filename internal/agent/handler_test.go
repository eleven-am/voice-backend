package agent

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
)

func newTestAgentHandler() (*Handler, *user.SessionManager) {
	sm := user.NewSessionManager([]byte("test-key"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(nil, nil, sm, nil, logger)
	return h, sm
}

func TestNewHandler(t *testing.T) {
	sm := user.NewSessionManager([]byte("key"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(nil, nil, sm, nil, logger)

	if h == nil {
		t.Fatal("handler should not be nil")
	}
	if h.sessions != sm {
		t.Error("session manager should be set")
	}
}

func TestHandler_RegisterRoutes(t *testing.T) {
	h, _ := newTestAgentHandler()
	e := echo.New()
	g := e.Group("/agents")

	h.RegisterRoutes(g)

	routes := e.Routes()
	expectedPaths := []string{
		"/agents",
		"/agents/:id",
		"/agents/:id/publish",
		"/agents/:id/reviews/:review_id/reply",
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

func TestHandler_List_Unauthorized(t *testing.T) {
	h, _ := newTestAgentHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.List(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestHandler_Create_Unauthorized(t *testing.T) {
	h, _ := newTestAgentHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/agents", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Create(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	var httpErr *echo.HTTPError
	errors.As(err, &httpErr)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestHandler_Get_Unauthorized(t *testing.T) {
	h, _ := newTestAgentHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/agents/agent_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	err := h.Get(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
}

func TestHandler_Update_Unauthorized(t *testing.T) {
	h, _ := newTestAgentHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPut, "/agents/agent_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	err := h.Update(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
}

func TestHandler_Delete_Unauthorized(t *testing.T) {
	h, _ := newTestAgentHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodDelete, "/agents/agent_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	err := h.Delete(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
}

func TestHandler_Publish_Unauthorized(t *testing.T) {
	h, _ := newTestAgentHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/agents/agent_123/publish", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	err := h.Publish(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
}

func TestHandler_ReplyToReview_Unauthorized(t *testing.T) {
	h, _ := newTestAgentHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/agents/agent_123/reviews/review_456/reply", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "review_id")
	c.SetParamValues("agent_123", "review_456")

	err := h.ReplyToReview(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
}

func TestAgentToResponse(t *testing.T) {
	agent := &Agent{
		ID:             "agent_123",
		DeveloperID:    "dev_456",
		Name:           "Test Agent",
		Description:    "A test agent",
		LogoURL:        "https://example.com/logo.png",
		Keywords:       []string{"test", "agent"},
		Capabilities:   []string{"chat", "voice"},
		Category:       shared.AgentCategoryAssistant,
		IsPublic:       true,
		IsVerified:     false,
		TotalInstalls:  100,
		ActiveInstalls: 50,
		AvgRating:      4.5,
		TotalReviews:   10,
	}

	resp := agentToResponse(agent)

	if resp.ID != agent.ID {
		t.Errorf("expected ID %s, got %s", agent.ID, resp.ID)
	}
	if resp.DeveloperID != agent.DeveloperID {
		t.Errorf("expected DeveloperID %s, got %s", agent.DeveloperID, resp.DeveloperID)
	}
	if resp.Name != agent.Name {
		t.Errorf("expected Name %s, got %s", agent.Name, resp.Name)
	}
	if resp.TotalInstalls != agent.TotalInstalls {
		t.Errorf("expected TotalInstalls %d, got %d", agent.TotalInstalls, resp.TotalInstalls)
	}
	if resp.AvgRating != agent.AvgRating {
		t.Errorf("expected AvgRating %f, got %f", agent.AvgRating, resp.AvgRating)
	}
}

func TestCreateAgentRequest_JSON(t *testing.T) {
	jsonStr := `{
		"name": "Test Agent",
		"description": "A test description",
		"logo_url": "https://example.com/logo.png",
		"keywords": ["test", "demo"],
		"capabilities": ["chat"],
		"category": "assistant"
	}`

	var req dto.CreateAgentRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Name != "Test Agent" {
		t.Errorf("expected name 'Test Agent', got '%s'", req.Name)
	}
	if req.Description != "A test description" {
		t.Errorf("expected description 'A test description', got '%s'", req.Description)
	}
	if len(req.Keywords) != 2 {
		t.Errorf("expected 2 keywords, got %d", len(req.Keywords))
	}
	if req.Category != string(shared.AgentCategoryAssistant) {
		t.Errorf("expected category 'assistant', got '%s'", req.Category)
	}
}

func TestUpdateAgentRequest_JSON(t *testing.T) {
	jsonStr := `{
		"name": "Updated Name",
		"description": "Updated description"
	}`

	var req dto.UpdateAgentRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Name == nil || *req.Name != "Updated Name" {
		t.Error("expected name to be 'Updated Name'")
	}
	if req.Description == nil || *req.Description != "Updated description" {
		t.Error("expected description to be 'Updated description'")
	}
	if req.LogoURL != nil {
		t.Error("expected logo_url to be nil")
	}
}

func TestReplyToReviewRequest_JSON(t *testing.T) {
	jsonStr := `{"reply": "Thank you for your feedback!"}`

	var req dto.ReplyToReviewRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Reply != "Thank you for your feedback!" {
		t.Errorf("expected reply 'Thank you for your feedback!', got '%s'", req.Reply)
	}
}

func TestAgentResponse_JSON(t *testing.T) {
	resp := dto.AgentResponse{
		ID:            "agent_123",
		DeveloperID:   "dev_456",
		Name:          "Test Agent",
		Description:   "Description",
		Category:      string(shared.AgentCategoryDeveloper),
		IsPublic:      true,
		TotalInstalls: 1000,
		AvgRating:     4.8,
		TotalReviews:  50,
		CreatedAt:     "2024-01-01T00:00:00Z",
		UpdatedAt:     "2024-01-02T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"id":"agent_123"`) {
		t.Error("expected JSON to contain id")
	}
	if !strings.Contains(jsonStr, `"is_public":true`) {
		t.Error("expected JSON to contain is_public")
	}
	if !strings.Contains(jsonStr, `"total_installs":1000`) {
		t.Error("expected JSON to contain total_installs")
	}
}
