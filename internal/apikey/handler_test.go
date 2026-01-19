package apikey

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

	"github.com/eleven-am/voice-backend/internal/dto"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func newTestAPIKeyHandler() (*Handler, *user.SessionManager) {
	sm := user.NewSessionManager([]byte("test-key"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(nil, nil, sm, logger)
	return h, sm
}

func TestNewAPIKeyHandler(t *testing.T) {
	sm := user.NewSessionManager([]byte("key"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(nil, nil, sm, logger)

	if h == nil {
		t.Fatal("handler should not be nil")
	}
	if h.sessions != sm {
		t.Error("session manager should be set")
	}
}

func TestAPIKeyHandler_RegisterRoutes(t *testing.T) {
	h, _ := newTestAPIKeyHandler()
	e := echo.New()
	g := e.Group("/apikeys")

	h.RegisterRoutes(g)

	routes := e.Routes()
	expectedPaths := []string{
		"/apikeys",
		"/apikeys/:id",
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

func TestAPIKeyHandler_List_Unauthorized(t *testing.T) {
	h, _ := newTestAPIKeyHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/apikeys", nil)
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

func TestAPIKeyHandler_Create_Unauthorized(t *testing.T) {
	h, _ := newTestAPIKeyHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/apikeys", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Create(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestAPIKeyHandler_Delete_Unauthorized(t *testing.T) {
	h, _ := newTestAPIKeyHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodDelete, "/apikeys/key_123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("key_123")

	err := h.Delete(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestAPIKeyResponse_JSON(t *testing.T) {
	expiresAt := "2024-12-31T23:59:59Z"
	lastUsed := "2024-01-15T10:30:00Z"
	resp := dto.APIKeyResponse{
		ID:        "key_123",
		Name:      "My API Key",
		Prefix:    "sk-voice-abc",
		CreatedAt: "2024-01-01T00:00:00Z",
		ExpiresAt: &expiresAt,
		LastUsed:  &lastUsed,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"id":"key_123"`) {
		t.Error("expected JSON to contain id")
	}
	if !strings.Contains(jsonStr, `"prefix":"sk-voice-abc"`) {
		t.Error("expected JSON to contain prefix")
	}
	if !strings.Contains(jsonStr, `"expires_at":"2024-12-31T23:59:59Z"`) {
		t.Error("expected JSON to contain expires_at")
	}
	if !strings.Contains(jsonStr, `"last_used_at":"2024-01-15T10:30:00Z"`) {
		t.Error("expected JSON to contain last_used_at")
	}
}

func TestAPIKeyResponse_OmitEmpty(t *testing.T) {
	resp := dto.APIKeyResponse{
		ID:        "key_123",
		Name:      "My API Key",
		Prefix:    "sk-voice-xyz",
		CreatedAt: "2024-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, `"expires_at"`) {
		t.Error("expires_at should be omitted when nil")
	}
	if strings.Contains(jsonStr, `"last_used_at"`) {
		t.Error("last_used_at should be omitted when nil")
	}
}

func TestCreateAPIKeyResponse_JSON(t *testing.T) {
	resp := dto.CreateAPIKeyResponse{
		ID:        "key_123",
		Name:      "New Key",
		Prefix:    "sk-voice-new",
		CreatedAt: "2024-01-01T00:00:00Z",
		Secret:    "sk-voice-newXXXXXXXXXXXXXXXXXXXXX",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"secret":"sk-voice-newXXXXXXXXXXXXXXXXXXXXX"`) {
		t.Error("expected JSON to contain secret")
	}
	if !strings.Contains(jsonStr, `"id":"key_123"`) {
		t.Error("expected JSON to contain embedded fields")
	}
}

func TestCreateAPIKeyRequest_JSON(t *testing.T) {
	jsonStr := `{"name": "Production Key", "expires_in_days": 90}`

	var req dto.CreateAPIKeyRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Name != "Production Key" {
		t.Errorf("expected name 'Production Key', got '%s'", req.Name)
	}
	if req.ExpiresIn == nil || *req.ExpiresIn != 90 {
		t.Error("expected expires_in_days to be 90")
	}
}

func TestCreateAPIKeyRequest_NoExpiry(t *testing.T) {
	jsonStr := `{"name": "Never Expires"}`

	var req dto.CreateAPIKeyRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.ExpiresIn != nil {
		t.Error("expected expires_in to be nil")
	}
}

func TestKeyToResponse(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)
	lastUsed := now.Add(-time.Hour)

	key := &APIKey{
		ID:        "key_123",
		Name:      "Test Key",
		Prefix:    "sk-voice-abc",
		CreatedAt: now,
		ExpiresAt: &expiresAt,
		LastUsedAt: &lastUsed,
	}

	resp := keyToResponse(key)

	if resp.ID != key.ID {
		t.Errorf("expected ID %s, got %s", key.ID, resp.ID)
	}
	if resp.Name != key.Name {
		t.Errorf("expected Name %s, got %s", key.Name, resp.Name)
	}
	if resp.Prefix != key.Prefix {
		t.Errorf("expected Prefix %s, got %s", key.Prefix, resp.Prefix)
	}
	if resp.ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set")
	}
	if resp.LastUsed == nil {
		t.Error("expected LastUsed to be set")
	}
}

func TestKeyToResponse_NoOptionalFields(t *testing.T) {
	now := time.Now()
	key := &APIKey{
		ID:        "key_123",
		Name:      "Test Key",
		Prefix:    "sk-voice-xyz",
		CreatedAt: now,
	}

	resp := keyToResponse(key)

	if resp.ExpiresAt != nil {
		t.Error("expected ExpiresAt to be nil")
	}
	if resp.LastUsed != nil {
		t.Error("expected LastUsed to be nil")
	}
}

func TestOwnerType(t *testing.T) {
	if OwnerTypeUser != "user" {
		t.Errorf("expected OwnerTypeUser to be 'user', got '%s'", OwnerTypeUser)
	}
	if OwnerTypeAgent != "agent" {
		t.Errorf("expected OwnerTypeAgent to be 'agent', got '%s'", OwnerTypeAgent)
	}
}

func newTestAPIKeyHandlerWithDB(t *testing.T) (*Handler, *user.SessionManager, *Store, *user.Store) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	userStore := user.NewStore(db)
	userStore.Migrate()

	store := NewStore(db)
	store.Migrate()

	sm := user.NewSessionManager([]byte("test-secret-key-32-bytes-long!!"), false, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(store, userStore, sm, logger)
	return h, sm, store, userStore
}

func createAPIKeySessionCookies(_ *testing.T, sm *user.SessionManager, userID string) (sessionCookie, csrfCookie *http.Cookie, csrfToken string) {
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

func TestAPIKeyHandler_List_Success(t *testing.T) {
	h, sm, store, userStore := newTestAPIKeyHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_apikey_list",
		Provider:    "google",
		ProviderSub: "sub_apikey_list",
		IsDeveloper: true,
	})

	store.Create(ctx, &APIKey{
		OwnerID:   "user_apikey_list",
		OwnerType: OwnerTypeUser,
		Name:      "Key 1",
	})
	store.Create(ctx, &APIKey{
		OwnerID:   "user_apikey_list",
		OwnerType: OwnerTypeUser,
		Name:      "Key 2",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/apikeys", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAPIKeySessionCookies(t, sm, "user_apikey_list")
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

func TestAPIKeyHandler_List_NotDeveloper(t *testing.T) {
	h, sm, _, userStore := newTestAPIKeyHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_not_dev",
		Provider:    "google",
		ProviderSub: "sub_not_dev",
		IsDeveloper: false,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/apikeys", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAPIKeySessionCookies(t, sm, "user_not_dev")
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

func TestAPIKeyHandler_Create_Success(t *testing.T) {
	h, sm, _, userStore := newTestAPIKeyHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_create_key",
		Provider:    "google",
		ProviderSub: "sub_create_key",
		IsDeveloper: true,
	})

	e := echo.New()
	body := `{"name":"New API Key"}`
	req := httptest.NewRequest(http.MethodPost, "/apikeys", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAPIKeySessionCookies(t, sm, "user_create_key")
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

func TestAPIKeyHandler_Create_WithExpiry(t *testing.T) {
	h, sm, _, userStore := newTestAPIKeyHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_create_exp",
		Provider:    "google",
		ProviderSub: "sub_create_exp",
		IsDeveloper: true,
	})

	e := echo.New()
	body := `{"name":"Expiring Key", "expires_in_days": 30}`
	req := httptest.NewRequest(http.MethodPost, "/apikeys", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAPIKeySessionCookies(t, sm, "user_create_exp")
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

func TestAPIKeyHandler_Create_InvalidJSON(t *testing.T) {
	h, sm, _, userStore := newTestAPIKeyHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_create_inv",
		Provider:    "google",
		ProviderSub: "sub_create_inv",
		IsDeveloper: true,
	})

	e := echo.New()
	body := `{invalid}`
	req := httptest.NewRequest(http.MethodPost, "/apikeys", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAPIKeySessionCookies(t, sm, "user_create_inv")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)

	err := h.Create(c)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestAPIKeyHandler_Delete_Success(t *testing.T) {
	h, sm, store, userStore := newTestAPIKeyHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_delete_key",
		Provider:    "google",
		ProviderSub: "sub_delete_key",
		IsDeveloper: true,
	})

	key := &APIKey{
		OwnerID:   "user_delete_key",
		OwnerType: OwnerTypeUser,
		Name:      "Key to delete",
	}
	store.Create(ctx, key)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/apikeys/"+key.ID, nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAPIKeySessionCookies(t, sm, "user_delete_key")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(key.ID)

	err := h.Delete(c)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestAPIKeyHandler_Delete_NotFound(t *testing.T) {
	h, sm, _, userStore := newTestAPIKeyHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_del_nf",
		Provider:    "google",
		ProviderSub: "sub_del_nf",
		IsDeveloper: true,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/apikeys/nonexistent", nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAPIKeySessionCookies(t, sm, "user_del_nf")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("nonexistent")

	err := h.Delete(c)
	if err == nil {
		t.Fatal("expected error for non-existent key")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestAPIKeyHandler_Delete_NotOwner(t *testing.T) {
	h, sm, store, userStore := newTestAPIKeyHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_owner_key",
		Provider:    "google",
		ProviderSub: "sub_owner_key",
		IsDeveloper: true,
	})
	userStore.Create(ctx, &user.User{
		ID:          "user_other_key",
		Provider:    "google",
		ProviderSub: "sub_other_key",
		IsDeveloper: true,
	})

	key := &APIKey{
		OwnerID:   "user_owner_key",
		OwnerType: OwnerTypeUser,
		Name:      "Owned key",
	}
	store.Create(ctx, key)

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/apikeys/"+key.ID, nil)
	rec := httptest.NewRecorder()

	sessionCookie, csrfCookie, csrfToken := createAPIKeySessionCookies(t, sm, "user_other_key")
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)

	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(key.ID)

	err := h.Delete(c)
	if err == nil {
		t.Fatal("expected error for non-owner")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, httpErr.Code)
	}
}
