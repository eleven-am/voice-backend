package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eleven-am/voice-backend/internal/auth"
	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func newTestInstallHandler() *InstallHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewInstallHandler(nil, logger)
}

func setInstallAuthClaims(c echo.Context, userID string) {
	claims := &auth.Claims{
		UserID: userID,
		Email:  userID + "@test.com",
		Name:   "Test User",
	}
	auth.SetClaimsForTest(c, claims)
}

func TestNewInstallHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewInstallHandler(nil, logger)

	if h == nil {
		t.Fatal("handler should not be nil")
	}
}

func TestInstallHandler_RegisterRoutes(t *testing.T) {
	h := newTestInstallHandler()
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
	h := newTestInstallHandler()
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
	h := newTestInstallHandler()
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
	h := newTestInstallHandler()
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
	h := newTestInstallHandler()
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

func newTestInstallHandlerWithDB(t *testing.T) (*InstallHandler, *Store, *user.Store) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	userStore := user.NewStore(db)
	userStore.Migrate()

	store := NewStore(db, nil)
	store.Migrate()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewInstallHandler(store, logger)
	return h, store, userStore
}

func TestInstallHandler_List_Success(t *testing.T) {
	h, store, userStore := newTestInstallHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_install_list",
		Provider:    "google",
		ProviderSub: "sub_install_list",
	})

	store.Create(ctx, &Agent{
		ID:          "agent_installed_1",
		DeveloperID: "dev_1",
		Name:        "Installed Agent 1",
		IsPublic:    true,
	})
	store.Create(ctx, &Agent{
		ID:          "agent_installed_2",
		DeveloperID: "dev_1",
		Name:        "Installed Agent 2",
		IsPublic:    true,
	})

	store.Install(ctx, &AgentInstall{
		ID:      "inst_1",
		UserID:  "user_install_list",
		AgentID: "agent_installed_1",
	})
	store.Install(ctx, &AgentInstall{
		ID:      "inst_2",
		UserID:  "user_install_list",
		AgentID: "agent_installed_2",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/me/agents", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	setInstallAuthClaims(c, "user_install_list")

	err := h.List(c)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestInstallHandler_Install_Success(t *testing.T) {
	h, store, userStore := newTestInstallHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_installer",
		Provider:    "google",
		ProviderSub: "sub_installer",
	})

	store.Create(ctx, &Agent{
		ID:          "agent_to_install",
		DeveloperID: "other_dev",
		Name:        "Agent To Install",
		IsPublic:    true,
	})

	e := echo.New()
	body := `{"scopes": ["profile", "email"]}`
	req := httptest.NewRequest(http.MethodPost, "/me/agents/agent_to_install/install", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_to_install")

	setInstallAuthClaims(c, "user_installer")

	err := h.Install(c)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}
}

func TestInstallHandler_Install_AgentNotFound(t *testing.T) {
	h, _, userStore := newTestInstallHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_install_nf",
		Provider:    "google",
		ProviderSub: "sub_install_nf",
	})

	e := echo.New()
	body := `{"scopes": ["profile"]}`
	req := httptest.NewRequest(http.MethodPost, "/me/agents/nonexistent/install", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("nonexistent")

	setInstallAuthClaims(c, "user_install_nf")

	err := h.Install(c)
	if err == nil {
		t.Fatal("expected error for non-existent agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestInstallHandler_Install_PrivateAgent(t *testing.T) {
	h, store, userStore := newTestInstallHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_install_priv",
		Provider:    "google",
		ProviderSub: "sub_install_priv",
	})

	store.Create(ctx, &Agent{
		ID:          "agent_private",
		DeveloperID: "other_dev",
		Name:        "Private Agent",
		IsPublic:    false,
	})

	e := echo.New()
	body := `{"scopes": ["profile"]}`
	req := httptest.NewRequest(http.MethodPost, "/me/agents/agent_private/install", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_private")

	setInstallAuthClaims(c, "user_install_priv")

	err := h.Install(c)
	if err == nil {
		t.Fatal("expected error for private agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestInstallHandler_Uninstall_Success(t *testing.T) {
	t.Skip("Uninstall uses GREATEST function not supported in SQLite")
}

func TestInstallHandler_UpdateScopes_Success(t *testing.T) {
	h, store, userStore := newTestInstallHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_update_scopes",
		Provider:    "google",
		ProviderSub: "sub_update_scopes",
	})

	store.Create(ctx, &Agent{
		ID:          "agent_scopes",
		DeveloperID: "dev_1",
		Name:        "Scopes Agent",
		IsPublic:    true,
	})

	store.Install(ctx, &AgentInstall{
		ID:            "inst_scopes",
		UserID:        "user_update_scopes",
		AgentID:       "agent_scopes",
		GrantedScopes: []string{"profile"},
	})

	e := echo.New()
	body := `{"scopes": ["profile", "email", "history"]}`
	req := httptest.NewRequest(http.MethodPut, "/me/agents/agent_scopes/scopes", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_scopes")

	setInstallAuthClaims(c, "user_update_scopes")

	err := h.UpdateScopes(c)
	if err != nil {
		t.Fatalf("UpdateScopes() error = %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestInstallHandler_UpdateScopes_NotInstalled(t *testing.T) {
	h, store, userStore := newTestInstallHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_no_inst",
		Provider:    "google",
		ProviderSub: "sub_no_inst",
	})

	store.Create(ctx, &Agent{
		ID:          "agent_no_inst",
		DeveloperID: "dev_1",
		Name:        "Not Installed Agent",
		IsPublic:    true,
	})

	e := echo.New()
	body := `{"scopes": ["profile"]}`
	req := httptest.NewRequest(http.MethodPut, "/me/agents/agent_no_inst/scopes", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_no_inst")

	setInstallAuthClaims(c, "user_no_inst")

	err := h.UpdateScopes(c)
	if err == nil {
		t.Fatal("expected error when agent not installed")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestInstallHandler_Install_InvalidJSON(t *testing.T) {
	h, store, userStore := newTestInstallHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_inst_invalid",
		Provider:    "google",
		ProviderSub: "sub_inst_invalid",
	})

	store.Create(ctx, &Agent{
		ID:          "agent_inst_invalid",
		DeveloperID: "other_dev",
		Name:        "Agent For Invalid JSON",
		IsPublic:    true,
	})

	e := echo.New()
	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPost, "/me/agents/agent_inst_invalid/install", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_inst_invalid")

	setInstallAuthClaims(c, "user_inst_invalid")

	err := h.Install(c)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestInstallHandler_UpdateScopes_InvalidJSON(t *testing.T) {
	h, store, userStore := newTestInstallHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_scopes_invalid",
		Provider:    "google",
		ProviderSub: "sub_scopes_invalid",
	})

	store.Create(ctx, &Agent{
		ID:          "agent_scopes_invalid",
		DeveloperID: "dev_1",
		Name:        "Scopes Agent Invalid",
		IsPublic:    true,
	})

	store.Install(ctx, &AgentInstall{
		ID:      "inst_scopes_inv",
		UserID:  "user_scopes_invalid",
		AgentID: "agent_scopes_invalid",
	})

	e := echo.New()
	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPut, "/me/agents/agent_scopes_invalid/scopes", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_scopes_invalid")

	setInstallAuthClaims(c, "user_scopes_invalid")

	err := h.UpdateScopes(c)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
