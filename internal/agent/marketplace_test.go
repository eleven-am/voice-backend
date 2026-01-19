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
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/labstack/echo/v4"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func newTestMarketplaceHandler() *MarketplaceHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewMarketplaceHandler(nil, nil, logger)
}

func setMarketplaceAuthClaims(c echo.Context, userID string) {
	claims := &auth.Claims{
		UserID: userID,
		Email:  userID + "@test.com",
		Name:   "Test User",
	}
	auth.SetClaimsForTest(c, claims)
}

func TestNewMarketplaceHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewMarketplaceHandler(nil, nil, logger)

	if h == nil {
		t.Fatal("handler should not be nil")
	}
}

func TestMarketplaceHandler_RegisterRoutes(t *testing.T) {
	h := newTestMarketplaceHandler()
	e := echo.New()
	g := e.Group("/store")

	h.RegisterRoutes(g)

	routes := e.Routes()
	expectedPaths := []string{
		"/store/agents",
		"/store/agents/:id",
		"/store/agents/search",
		"/store/agents/:id/reviews",
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

func TestMarketplaceHandler_Search_MissingQuery(t *testing.T) {
	h := newTestMarketplaceHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/store/agents/search", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Search(c)
	if err == nil {
		t.Fatal("expected error when query is missing")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, httpErr.Code)
	}
}

func TestMarketplaceHandler_Search_NoEmbeddingService(t *testing.T) {
	h := newTestMarketplaceHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/store/agents/search?q=test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Search(c)
	if err == nil {
		t.Fatal("expected error when embedding service is nil")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, httpErr.Code)
	}
}

func TestMarketplaceHandler_CreateReview_Unauthorized(t *testing.T) {
	h := newTestMarketplaceHandler()
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/store/agents/agent_123/reviews", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_123")

	err := h.CreateReview(c)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, httpErr.Code)
	}
}

func TestAgentToMarketplaceResponse(t *testing.T) {
	agent := &Agent{
		ID:            "agent_123",
		Name:          "Marketplace Agent",
		Description:   "A great agent",
		LogoURL:       "https://example.com/logo.png",
		Keywords:      []string{"helpful", "smart"},
		Category:      shared.AgentCategoryProductivity,
		IsVerified:    true,
		TotalInstalls: 5000,
		AvgRating:     4.9,
		TotalReviews:  200,
	}

	resp := agentToMarketplaceResponse(agent)

	if resp.ID != agent.ID {
		t.Errorf("expected ID %s, got %s", agent.ID, resp.ID)
	}
	if resp.Name != agent.Name {
		t.Errorf("expected Name %s, got %s", agent.Name, resp.Name)
	}
	if resp.IsVerified != agent.IsVerified {
		t.Errorf("expected IsVerified %v, got %v", agent.IsVerified, resp.IsVerified)
	}
	if resp.TotalInstalls != agent.TotalInstalls {
		t.Errorf("expected TotalInstalls %d, got %d", agent.TotalInstalls, resp.TotalInstalls)
	}
}

func TestMarketplaceAgentResponse_JSON(t *testing.T) {
	resp := dto.MarketplaceAgentResponse{
		ID:            "agent_123",
		Name:          "Test Agent",
		Description:   "A test agent",
		Category:      string(shared.AgentCategoryEducation),
		IsVerified:    true,
		TotalInstalls: 1234,
		AvgRating:     4.5,
		TotalReviews:  100,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"id":"agent_123"`) {
		t.Error("expected JSON to contain id")
	}
	if !strings.Contains(jsonStr, `"is_verified":true`) {
		t.Error("expected JSON to contain is_verified")
	}
	if !strings.Contains(jsonStr, `"category":"education"`) {
		t.Error("expected JSON to contain category")
	}
}

func TestReviewToResponse(t *testing.T) {
	now := time.Now()
	reply := "Thank you!"
	repliedAt := now.Add(time.Hour)

	review := &AgentReview{
		ID:             "review_123",
		UserID:         "user_456",
		Rating:         5,
		Body:           "Great agent!",
		DeveloperReply: &reply,
		RepliedAt:      &repliedAt,
		CreatedAt:      now,
	}

	resp := reviewToResponse(review)

	if resp.ID != review.ID {
		t.Errorf("expected ID %s, got %s", review.ID, resp.ID)
	}
	if resp.UserID != review.UserID {
		t.Errorf("expected UserID %s, got %s", review.UserID, resp.UserID)
	}
	if resp.Rating != review.Rating {
		t.Errorf("expected Rating %d, got %d", review.Rating, resp.Rating)
	}
	if resp.Body != review.Body {
		t.Errorf("expected Body %s, got %s", review.Body, resp.Body)
	}
	if resp.DeveloperReply == nil || *resp.DeveloperReply != reply {
		t.Error("expected DeveloperReply to be set")
	}
	if resp.RepliedAt == nil {
		t.Error("expected RepliedAt to be set")
	}
}

func TestReviewToResponse_NoReply(t *testing.T) {
	review := &AgentReview{
		ID:        "review_123",
		UserID:    "user_456",
		Rating:    4,
		Body:      "Good agent",
		CreatedAt: time.Now(),
	}

	resp := reviewToResponse(review)

	if resp.DeveloperReply != nil {
		t.Error("expected DeveloperReply to be nil")
	}
	if resp.RepliedAt != nil {
		t.Error("expected RepliedAt to be nil")
	}
}

func TestReviewResponse_JSON(t *testing.T) {
	reply := "Thanks!"
	repliedAt := "2024-01-15T12:00:00Z"
	resp := dto.ReviewResponse{
		ID:             "review_123",
		UserID:         "user_456",
		Rating:         5,
		Body:           "Excellent!",
		DeveloperReply: &reply,
		RepliedAt:      &repliedAt,
		CreatedAt:      "2024-01-01T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"rating":5`) {
		t.Error("expected JSON to contain rating")
	}
	if !strings.Contains(jsonStr, `"developer_reply":"Thanks!"`) {
		t.Error("expected JSON to contain developer_reply")
	}
}

func TestCreateReviewRequest_JSON(t *testing.T) {
	jsonStr := `{"rating": 4, "body": "Good experience"}`

	var req dto.CreateReviewRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Rating != 4 {
		t.Errorf("expected rating 4, got %d", req.Rating)
	}
	if req.Body != "Good experience" {
		t.Errorf("expected body 'Good experience', got '%s'", req.Body)
	}
}

func newTestMarketplaceHandlerWithDB(t *testing.T) (*MarketplaceHandler, *Store, *user.Store) {
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
	h := NewMarketplaceHandler(store, nil, logger)
	return h, store, userStore
}

func TestMarketplaceHandler_List_Success(t *testing.T) {
	h, store, _ := newTestMarketplaceHandlerWithDB(t)
	ctx := context.Background()

	store.Create(ctx, &Agent{
		ID:          "agent_mp_1",
		DeveloperID: "dev_1",
		Name:        "Public Agent 1",
		IsPublic:    true,
	})
	store.Create(ctx, &Agent{
		ID:          "agent_mp_2",
		DeveloperID: "dev_1",
		Name:        "Public Agent 2",
		IsPublic:    true,
	})
	store.Create(ctx, &Agent{
		ID:          "agent_mp_private",
		DeveloperID: "dev_1",
		Name:        "Private Agent",
		IsPublic:    false,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/store/agents", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.List(c)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestMarketplaceHandler_Get_Success(t *testing.T) {
	h, store, _ := newTestMarketplaceHandlerWithDB(t)
	ctx := context.Background()

	store.Create(ctx, &Agent{
		ID:          "agent_mp_get",
		DeveloperID: "dev_1",
		Name:        "Public Agent",
		IsPublic:    true,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/store/agents/agent_mp_get", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_mp_get")

	err := h.Get(c)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestMarketplaceHandler_Get_NotFound(t *testing.T) {
	h, _, _ := newTestMarketplaceHandlerWithDB(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/store/agents/nonexistent", nil)
	rec := httptest.NewRecorder()
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

func TestMarketplaceHandler_Get_NotPublic(t *testing.T) {
	h, store, _ := newTestMarketplaceHandlerWithDB(t)
	ctx := context.Background()

	store.Create(ctx, &Agent{
		ID:          "agent_private",
		DeveloperID: "dev_1",
		Name:        "Private Agent",
		IsPublic:    false,
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/store/agents/agent_private", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_private")

	err := h.Get(c)
	if err == nil {
		t.Fatal("expected error for private agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestMarketplaceHandler_GetReviews_Success(t *testing.T) {
	h, store, _ := newTestMarketplaceHandlerWithDB(t)
	ctx := context.Background()

	store.Create(ctx, &Agent{
		ID:          "agent_reviews",
		DeveloperID: "dev_1",
		Name:        "Agent With Reviews",
		IsPublic:    true,
	})

	store.CreateReview(ctx, &AgentReview{
		ID:      "review_1",
		AgentID: "agent_reviews",
		UserID:  "user_1",
		Rating:  5,
		Body:    "Great agent!",
	})
	store.CreateReview(ctx, &AgentReview{
		ID:      "review_2",
		AgentID: "agent_reviews",
		UserID:  "user_2",
		Rating:  4,
		Body:    "Good agent",
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/store/agents/agent_reviews/reviews", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_reviews")

	err := h.GetReviews(c)
	if err != nil {
		t.Fatalf("GetReviews() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestMarketplaceHandler_GetReviews_AgentNotFound(t *testing.T) {
	h, _, _ := newTestMarketplaceHandlerWithDB(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/store/agents/nonexistent/reviews", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("nonexistent")

	err := h.GetReviews(c)
	if err == nil {
		t.Fatal("expected error for non-existent agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}

func TestMarketplaceHandler_CreateReview_Success(t *testing.T) {
	h, store, userStore := newTestMarketplaceHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_reviewer",
		Provider:    "google",
		ProviderSub: "sub_reviewer",
	})

	store.Create(ctx, &Agent{
		ID:          "agent_to_review",
		DeveloperID: "other_dev",
		Name:        "Agent To Review",
		IsPublic:    true,
	})

	store.Install(ctx, &AgentInstall{
		ID:      "install_review",
		UserID:  "user_reviewer",
		AgentID: "agent_to_review",
	})

	e := echo.New()
	body := `{"rating": 5, "body": "Amazing agent!"}`
	req := httptest.NewRequest(http.MethodPost, "/store/agents/agent_to_review/reviews", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_to_review")

	setMarketplaceAuthClaims(c, "user_reviewer")

	err := h.CreateReview(c)
	if err != nil {
		t.Fatalf("CreateReview() error = %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}
}

func TestMarketplaceHandler_CreateReview_NotInstalled(t *testing.T) {
	h, store, userStore := newTestMarketplaceHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_no_install",
		Provider:    "google",
		ProviderSub: "sub_no_install",
	})

	store.Create(ctx, &Agent{
		ID:          "agent_no_install",
		DeveloperID: "other_dev",
		Name:        "Agent Not Installed",
		IsPublic:    true,
	})

	e := echo.New()
	body := `{"rating": 5, "body": "Great!"}`
	req := httptest.NewRequest(http.MethodPost, "/store/agents/agent_no_install/reviews", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_no_install")

	setMarketplaceAuthClaims(c, "user_no_install")

	err := h.CreateReview(c)
	if err == nil {
		t.Fatal("expected error when agent not installed")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, httpErr.Code)
	}
}

func TestMarketplaceHandler_CreateReview_InvalidJSON(t *testing.T) {
	h, store, userStore := newTestMarketplaceHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_review_inv",
		Provider:    "google",
		ProviderSub: "sub_review_inv",
	})

	store.Create(ctx, &Agent{
		ID:          "agent_review_inv",
		DeveloperID: "other_dev",
		Name:        "Review Invalid Agent",
		IsPublic:    true,
	})

	store.Install(ctx, &AgentInstall{
		ID:      "install_rev_inv",
		UserID:  "user_review_inv",
		AgentID: "agent_review_inv",
	})

	e := echo.New()
	body := `{invalid}`
	req := httptest.NewRequest(http.MethodPost, "/store/agents/agent_review_inv/reviews", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("agent_review_inv")

	setMarketplaceAuthClaims(c, "user_review_inv")

	err := h.CreateReview(c)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestMarketplaceHandler_CreateReview_AgentNotFound(t *testing.T) {
	h, _, userStore := newTestMarketplaceHandlerWithDB(t)
	ctx := context.Background()

	userStore.Create(ctx, &user.User{
		ID:          "user_review_anf",
		Provider:    "google",
		ProviderSub: "sub_review_anf",
	})

	e := echo.New()
	body := `{"rating": 5, "body": "Great!"}`
	req := httptest.NewRequest(http.MethodPost, "/store/agents/nonexistent/reviews", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("nonexistent")

	setMarketplaceAuthClaims(c, "user_review_anf")

	err := h.CreateReview(c)
	if err == nil {
		t.Fatal("expected error for non-existent agent")
	}
	httpErr := err.(*echo.HTTPError)
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, httpErr.Code)
	}
}
