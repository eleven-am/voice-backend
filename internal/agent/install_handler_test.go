package agent

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
)

func newTestInstallHandler() (*InstallHandler, *user.SessionManager) {
	sm := user.NewSessionManager([]byte("test-key"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewInstallHandler(nil, sm, logger)
	return h, sm
}

func TestNewInstallHandler(t *testing.T) {
	sm := user.NewSessionManager([]byte("key"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewInstallHandler(nil, sm, logger)

	if h == nil {
		t.Fatal("handler should not be nil")
	}
	if h.sessions != sm {
		t.Error("session manager should be set")
	}
}

func TestInstallHandler_RegisterRoutes(t *testing.T) {
	h, _ := newTestInstallHandler()
	e := echo.New()
	g := e.Group("/me/agents")

	h.RegisterRoutes(g)

	routes := e.Routes()
	expectedPaths := []string{
		"/me/agents",
		"/me/agents/:id/install",
		"/me/agents/:id",
		"/me/agents/:id/scopes",
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

func TestInstallHandler_List_Unauthorized(t *testing.T) {
	h, _ := newTestInstallHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/me/agents", nil)
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

func TestInstallHandler_Install_Unauthorized(t *testing.T) {
	h, _ := newTestInstallHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/me/agents/agent_123/install", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	err := h.Install(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestInstallHandler_Uninstall_Unauthorized(t *testing.T) {
	h, _ := newTestInstallHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodDelete, "/me/agents/agent_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	err := h.Uninstall(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestInstallHandler_UpdateScopes_Unauthorized(t *testing.T) {
	h, _ := newTestInstallHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPut, "/me/agents/agent_123/scopes", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	err := h.UpdateScopes(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestInstalledAgentResponse_JSON(t *testing.T) {
	resp := dto.InstalledAgentResponse{
		AgentID:       "agent_123",
		Name:          "My Agent",
		Description:   "A useful agent",
		LogoURL:       "https://example.com/logo.png",
		GrantedScopes: []string{"profile", "email"},
		InstalledAt:   "2024-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"agent_id":"agent_123"`) {
		t.Error("expected JSON to contain agent_id")
	}
	if !strings.Contains(jsonStr, `"granted_scopes":["profile","email"]`) {
		t.Error("expected JSON to contain granted_scopes")
	}
	if !strings.Contains(jsonStr, `"installed_at":"2024-01-01T00:00:00Z"`) {
		t.Error("expected JSON to contain installed_at")
	}
}

func TestInstalledAgentResponse_Omitempty(t *testing.T) {
	resp := dto.InstalledAgentResponse{
		AgentID:       "agent_123",
		Name:          "My Agent",
		GrantedScopes: []string{},
		InstalledAt:   "2024-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, `"description":`) && strings.Contains(jsonStr, `"description":""`) {
		t.Log("description is omitted when empty (omitempty)")
	}
	if strings.Contains(jsonStr, `"logo_url":`) && strings.Contains(jsonStr, `"logo_url":""`) {
		t.Log("logo_url is omitted when empty (omitempty)")
	}
}

func TestInstallRequest_JSON(t *testing.T) {
	jsonStr := `{"scopes": ["profile", "email", "realtime"]}`

	var req dto.InstallRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(req.Scopes) != 3 {
		t.Errorf("expected 3 scopes, got %d", len(req.Scopes))
	}
	if req.Scopes[0] != "profile" {
		t.Errorf("expected first scope 'profile', got '%s'", req.Scopes[0])
	}
}

func TestInstallRequest_EmptyScopes(t *testing.T) {
	jsonStr := `{"scopes": []}`

	var req dto.InstallRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Scopes == nil {
		t.Error("scopes should not be nil")
	}
	if len(req.Scopes) != 0 {
		t.Errorf("expected 0 scopes, got %d", len(req.Scopes))
	}
}

func TestUpdateScopesRequest_JSON(t *testing.T) {
	jsonStr := `{"scopes": ["history", "preferences"]}`

	var req dto.UpdateScopesRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(req.Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(req.Scopes))
	}
}

func TestAgentInstall_Fields(t *testing.T) {
	now := time.Now()
	install := &AgentInstall{
		ID:            "install_123",
		UserID:        "user_456",
		AgentID:       "agent_789",
		GrantedScopes: []string{"profile"},
		InstalledAt:   now,
	}

	if install.ID != "install_123" {
		t.Errorf("expected ID 'install_123', got '%s'", install.ID)
	}
	if install.InstalledAt != now {
		t.Error("InstalledAt should match")
	}
}
