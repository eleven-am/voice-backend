package agent

import (
	"context"
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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
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

func newTestHandlerWithDB(t *testing.T) (*Handler, *user.SessionManager, *Store, *user.Store) {
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

	sm := user.NewSessionManager([]byte("test-secret-key-32-bytes-long!!"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(store, userStore, sm, nil, logger)
	return h, sm, store, userStore
}

func createAgentSessionCookies(_ *testing.T, sm *user.SessionManager, userID string) (sessionCookie, csrfCookie *http.Cookie, csrfToken string) {
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	sm.Create(c, userID)

	cookies := rec.Result().Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "voice_session" {
			sessionCookie = cookie
		}
		if cookie.Name == "voice_csrf" {
			csrfCookie = cookie
			csrfToken = cookie.Value
		}
	}
	return
}

func TestHandler_List_Success(t *testing.T) {
	h, sm, store, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_list",
		Provider:    "google",
		ProviderSub: "sub_list",
		IsDeveloper: true,
	})

	store.Create(ctx, &Agent{
		ID:          "agent_1",
		DeveloperID: "user_list",
		Name:        "Agent 1",
	})
	store.Create(ctx, &Agent{
		ID:          "agent_2",
		DeveloperID: "user_list",
		Name:        "Agent 2",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_list")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)

	err := h.List(c)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestHandler_List_NotDeveloper(t *testing.T) {
	h, sm, _, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_not_dev",
		Provider:    "google",
		ProviderSub: "sub_not_dev",
		IsDeveloper: false,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_not_dev")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)

	err := h.List(c)
	if err == nil {
		t.Fatal("expected error for non-developer")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, httpErr.Code)
	}
}

func TestHandler_Create_Success(t *testing.T) {
	h, sm, _, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_create",
		Provider:    "google",
		ProviderSub: "sub_create",
		IsDeveloper: true,
	})

	e := echo.New()
	body := `{"name":"New Agent","description":"A new agent","category":"assistant"}`
	req := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_create")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)

	err := h.Create(c)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}
}

func TestHandler_Create_InvalidJSON(t *testing.T) {
	h, sm, _, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_create_invalid",
		Provider:    "google",
		ProviderSub: "sub_create_invalid",
		IsDeveloper: true,
	})

	e := echo.New()
	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_create_invalid")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)

	err := h.Create(c)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHandler_Get_Success(t *testing.T) {
	h, sm, store, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_get",
		Provider:    "google",
		ProviderSub: "sub_get",
		IsDeveloper: true,
	})

	store.Create(ctx, &Agent{
		ID:          "agent_get",
		DeveloperID: "user_get",
		Name:        "Get Test Agent",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/agents/agent_get", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_get")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_get")

	err := h.Get(c)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestHandler_Get_NotFound(t *testing.T) {
	h, sm, _, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_get_nf",
		Provider:    "google",
		ProviderSub: "sub_get_nf",
		IsDeveloper: true,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/agents/nonexistent", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_get_nf")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("nonexistent")

	err := h.Get(c)
	if err == nil {
		t.Fatal("expected error for non-existent agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestHandler_Get_NotOwner(t *testing.T) {
	h, sm, store, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_owner",
		Provider:    "google",
		ProviderSub: "sub_owner",
		IsDeveloper: true,
	})
	userStore.Create(ctx, &user.User{
		ID:          "user_other",
		Provider:    "google",
		ProviderSub: "sub_other",
		IsDeveloper: true,
	})

	store.Create(ctx, &Agent{
		ID:          "agent_owned",
		DeveloperID: "user_owner",
		Name:        "Owned Agent",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/agents/agent_owned", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_other")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_owned")

	err := h.Get(c)
	if err == nil {
		t.Fatal("expected error for non-owner")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, httpErr.Code)
	}
}

func TestHandler_Delete_Success(t *testing.T) {
	h, sm, store, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_delete",
		Provider:    "google",
		ProviderSub: "sub_delete",
		IsDeveloper: true,
	})

	store.Create(ctx, &Agent{
		ID:          "agent_delete",
		DeveloperID: "user_delete",
		Name:        "Delete Test Agent",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/agents/agent_delete", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_delete")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_delete")

	err := h.Delete(c)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestHandler_Publish_Success(t *testing.T) {
	h, sm, store, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_publish",
		Provider:    "google",
		ProviderSub: "sub_publish",
		IsDeveloper: true,
	})

	store.Create(ctx, &Agent{
		ID:          "agent_publish",
		DeveloperID: "user_publish",
		Name:        "Publish Test Agent",
		IsPublic:    false,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/agents/agent_publish/publish", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_publish")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_publish")

	err := h.Publish(c)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestHandler_Update_Success(t *testing.T) {
	h, sm, store, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_update",
		Provider:    "google",
		ProviderSub: "sub_update",
		IsDeveloper: true,
	})

	store.Create(ctx, &Agent{
		ID:          "agent_update",
		DeveloperID: "user_update",
		Name:        "Original Name",
	})

	e := echo.New()
	body := `{"name":"Updated Name"}`
	req := httptest.NewRequest(http.MethodPut, "/agents/agent_update", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_update")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_update")

	err := h.Update(c)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestHandler_ReplyToReview_Success(t *testing.T) {
	t.Skip("ReplyToReview uses NOW() function not supported in SQLite")
}

func TestHandler_ReplyToReview_InvalidJSON(t *testing.T) {
	h, sm, store, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_reply_invalid",
		Provider:    "google",
		ProviderSub: "sub_reply_invalid",
		IsDeveloper: true,
	})

	store.Create(ctx, &Agent{
		ID:          "agent_reply_invalid",
		DeveloperID: "user_reply_invalid",
		Name:        "Reply Invalid Agent",
	})

	e := echo.New()
	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPost, "/agents/agent_reply_invalid/reviews/review_id/reply", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_reply_invalid")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id", "review_id")
	c.SetParamValues("agent_reply_invalid", "review_id")

	err := h.ReplyToReview(c)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHandler_ReplyToReview_AgentNotFound(t *testing.T) {
	h, sm, _, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_reply_nf",
		Provider:    "google",
		ProviderSub: "sub_reply_nf",
		IsDeveloper: true,
	})

	e := echo.New()
	body := `{"reply":"Thank you!"}`
	req := httptest.NewRequest(http.MethodPost, "/agents/nonexistent/reviews/review_id/reply", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_reply_nf")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id", "review_id")
	c.SetParamValues("nonexistent", "review_id")

	err := h.ReplyToReview(c)
	if err == nil {
		t.Fatal("expected error for non-existent agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestHandler_Delete_NotFound(t *testing.T) {
	h, sm, _, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_delete_nf",
		Provider:    "google",
		ProviderSub: "sub_delete_nf",
		IsDeveloper: true,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/agents/nonexistent", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_delete_nf")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("nonexistent")

	err := h.Delete(c)
	if err == nil {
		t.Fatal("expected error for non-existent agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestHandler_Update_InvalidJSON(t *testing.T) {
	h, sm, store, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_update_inv",
		Provider:    "google",
		ProviderSub: "sub_update_inv",
		IsDeveloper: true,
	})

	store.Create(ctx, &Agent{
		ID:          "agent_update_inv",
		DeveloperID: "user_update_inv",
		Name:        "Update Invalid",
	})

	e := echo.New()
	body := `{invalid}`
	req := httptest.NewRequest(http.MethodPut, "/agents/agent_update_inv", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_update_inv")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_update_inv")

	err := h.Update(c)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHandler_Update_NotFound(t *testing.T) {
	h, sm, _, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_update_nf",
		Provider:    "google",
		ProviderSub: "sub_update_nf",
		IsDeveloper: true,
	})

	e := echo.New()
	body := `{"name":"New Name"}`
	req := httptest.NewRequest(http.MethodPut, "/agents/nonexistent", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_update_nf")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("nonexistent")

	err := h.Update(c)
	if err == nil {
		t.Fatal("expected error for non-existent agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestHandler_Publish_NotFound(t *testing.T) {
	h, sm, _, userStore := newTestHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_pub_nf",
		Provider:    "google",
		ProviderSub: "sub_pub_nf",
		IsDeveloper: true,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/agents/nonexistent/publish", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAgentSessionCookies(t, sm, "user_pub_nf")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("nonexistent")

	err := h.Publish(c)
	if err == nil {
		t.Fatal("expected error for non-existent agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}
