package agent

import (
	"context"
	"testing"
	"time"

	"github.com/eleven-am/voice-backend/internal/shared"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestAgentDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	return db
}

func TestNewStore(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestStore_Migrate(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)

	err := store.Migrate()
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
}

func TestStore_Create(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	tests := []struct {
		name    string
		agent   *Agent
		wantErr bool
	}{
		{
			name: "create agent with id",
			agent: &Agent{
				ID:          "agent_test1",
				DeveloperID: "dev_1",
				Name:        "Test Agent",
				Description: "Test description",
			},
			wantErr: false,
		},
		{
			name: "create agent without id",
			agent: &Agent{
				DeveloperID: "dev_1",
				Name:        "Auto ID Agent",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Create(ctx, tt.agent)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.agent.ID == "" {
				t.Error("agent ID should be generated if not provided")
			}
		})
	}
}

func TestStore_GetByID(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	agent := &Agent{
		ID:          "agent_getbyid",
		DeveloperID: "dev_1",
		Name:        "GetByID Agent",
	}
	store.Create(ctx, agent)

	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{
			name:    "existing agent",
			id:      "agent_getbyid",
			wantErr: nil,
		},
		{
			name:    "non-existent agent",
			id:      "agent_nonexistent",
			wantErr: shared.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.GetByID(ctx, tt.id)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Errorf("GetByID() unexpected error = %v", err)
				}
				if got.ID != tt.id {
					t.Errorf("GetByID() got ID = %v, want %v", got.ID, tt.id)
				}
			}
		})
	}
}

func TestStore_GetByDeveloper(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	store.Create(ctx, &Agent{ID: "agent_dev_1", DeveloperID: "dev_1", Name: "Agent 1"})
	store.Create(ctx, &Agent{ID: "agent_dev_2", DeveloperID: "dev_1", Name: "Agent 2"})
	store.Create(ctx, &Agent{ID: "agent_dev_3", DeveloperID: "dev_2", Name: "Agent 3"})

	agents, err := store.GetByDeveloper(ctx, "dev_1")
	if err != nil {
		t.Fatalf("GetByDeveloper failed: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}

	agents, err = store.GetByDeveloper(ctx, "dev_nonexistent")
	if err != nil {
		t.Fatalf("GetByDeveloper for non-existent failed: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestStore_ListPublic(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	store.Create(ctx, &Agent{ID: "agent_pub_1", DeveloperID: "dev_1", Name: "Public 1", IsPublic: true, Category: shared.AgentCategoryAssistant, TotalInstalls: 100})
	store.Create(ctx, &Agent{ID: "agent_pub_2", DeveloperID: "dev_1", Name: "Public 2", IsPublic: true, Category: shared.AgentCategoryAssistant, TotalInstalls: 50})
	store.Create(ctx, &Agent{ID: "agent_priv", DeveloperID: "dev_1", Name: "Private", IsPublic: false, Category: shared.AgentCategoryAssistant})
	store.Create(ctx, &Agent{ID: "agent_pub_3", DeveloperID: "dev_1", Name: "Public 3", IsPublic: true, Category: shared.AgentCategoryDeveloper, TotalInstalls: 200})

	agents, err := store.ListPublic(ctx, nil, 10, 0)
	if err != nil {
		t.Fatalf("ListPublic failed: %v", err)
	}
	if len(agents) != 3 {
		t.Errorf("expected 3 public agents, got %d", len(agents))
	}
	if len(agents) > 0 && agents[0].ID != "agent_pub_3" {
		t.Error("should be sorted by total_installs DESC")
	}

	cat := shared.AgentCategoryAssistant
	agents, err = store.ListPublic(ctx, &cat, 10, 0)
	if err != nil {
		t.Fatalf("ListPublic with category failed: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("expected 2 assistant agents, got %d", len(agents))
	}

	agents, err = store.ListPublic(ctx, nil, 1, 0)
	if err != nil {
		t.Fatalf("ListPublic with limit failed: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 agent with limit, got %d", len(agents))
	}
}

func TestStore_Update(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	agent := &Agent{ID: "agent_update", DeveloperID: "dev_1", Name: "Original"}
	store.Create(ctx, agent)

	agent.Name = "Updated"
	agent.Description = "New description"
	err := store.Update(ctx, agent)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, _ := store.GetByID(ctx, "agent_update")
	if updated.Name != "Updated" {
		t.Errorf("expected name 'Updated', got '%s'", updated.Name)
	}
}

func TestStore_Delete(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	agent := &Agent{ID: "agent_delete", DeveloperID: "dev_1", Name: "To Delete"}
	store.Create(ctx, agent)

	err := store.Delete(ctx, "agent_delete")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.GetByID(ctx, "agent_delete")
	if err != shared.ErrNotFound {
		t.Error("deleted agent should not be found")
	}

	err = store.Delete(ctx, "agent_nonexistent")
	if err != shared.ErrNotFound {
		t.Errorf("Delete non-existent should return ErrNotFound, got %v", err)
	}
}

func TestStore_IncrementInstalls(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	agent := &Agent{ID: "agent_installs", DeveloperID: "dev_1", Name: "Install Test"}
	store.Create(ctx, agent)

	err := store.IncrementInstalls(ctx, "agent_installs")
	if err != nil {
		t.Fatalf("IncrementInstalls failed: %v", err)
	}

	updated, _ := store.GetByID(ctx, "agent_installs")
	if updated.TotalInstalls != 1 {
		t.Errorf("expected TotalInstalls 1, got %d", updated.TotalInstalls)
	}
	if updated.ActiveInstalls != 1 {
		t.Errorf("expected ActiveInstalls 1, got %d", updated.ActiveInstalls)
	}
}

func TestStore_DecrementActiveInstalls(t *testing.T) {
	t.Skip("GREATEST function not supported in SQLite")
}

func TestStore_Install(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	agent := &Agent{ID: "agent_install", DeveloperID: "dev_1", Name: "Install Test"}
	store.Create(ctx, agent)

	install := &AgentInstall{
		UserID:        "user_1",
		AgentID:       "agent_install",
		GrantedScopes: []string{"read", "write"},
		InstalledAt:   time.Now(),
	}

	err := store.Install(ctx, install)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if install.ID == "" {
		t.Error("install ID should be generated")
	}

	updated, _ := store.GetByID(ctx, "agent_install")
	if updated.TotalInstalls != 1 {
		t.Errorf("expected TotalInstalls 1, got %d", updated.TotalInstalls)
	}
}

func TestStore_Uninstall(t *testing.T) {
	t.Skip("GREATEST function not supported in SQLite")
}

func TestStore_GetInstall(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	agent := &Agent{ID: "agent_getinst", DeveloperID: "dev_1", Name: "Test"}
	store.Create(ctx, agent)

	install := &AgentInstall{UserID: "user_1", AgentID: "agent_getinst"}
	store.Install(ctx, install)

	got, err := store.GetInstall(ctx, "user_1", "agent_getinst")
	if err != nil {
		t.Fatalf("GetInstall failed: %v", err)
	}
	if got.UserID != "user_1" {
		t.Errorf("expected UserID 'user_1', got '%s'", got.UserID)
	}

	_, err = store.GetInstall(ctx, "user_nonexistent", "agent_getinst")
	if err != shared.ErrNotFound {
		t.Errorf("GetInstall non-existent should return ErrNotFound, got %v", err)
	}
}

func TestStore_GetUserInstalls(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	store.Create(ctx, &Agent{ID: "agent_ui_1", DeveloperID: "dev_1", Name: "Agent 1"})
	store.Create(ctx, &Agent{ID: "agent_ui_2", DeveloperID: "dev_1", Name: "Agent 2"})

	store.Install(ctx, &AgentInstall{UserID: "user_ui", AgentID: "agent_ui_1"})
	store.Install(ctx, &AgentInstall{UserID: "user_ui", AgentID: "agent_ui_2"})

	installs, err := store.GetUserInstalls(ctx, "user_ui")
	if err != nil {
		t.Fatalf("GetUserInstalls failed: %v", err)
	}
	if len(installs) != 2 {
		t.Errorf("expected 2 installs, got %d", len(installs))
	}
}

func TestStore_GetInstalledAgents(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	store.Create(ctx, &Agent{ID: "agent_ia_1", DeveloperID: "dev_1", Name: "Installed 1"})
	store.Create(ctx, &Agent{ID: "agent_ia_2", DeveloperID: "dev_1", Name: "Installed 2"})
	store.Create(ctx, &Agent{ID: "agent_ia_3", DeveloperID: "dev_1", Name: "Not Installed"})

	store.Install(ctx, &AgentInstall{UserID: "user_ia", AgentID: "agent_ia_1"})
	store.Install(ctx, &AgentInstall{UserID: "user_ia", AgentID: "agent_ia_2"})

	agents, err := store.GetInstalledAgents(ctx, "user_ia")
	if err != nil {
		t.Fatalf("GetInstalledAgents failed: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("expected 2 installed agents, got %d", len(agents))
	}
}

func TestStore_UpdateInstallScopes(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	store.Create(ctx, &Agent{ID: "agent_scopes", DeveloperID: "dev_1", Name: "Scopes Test"})
	store.Install(ctx, &AgentInstall{UserID: "user_scopes", AgentID: "agent_scopes", GrantedScopes: []string{"read"}})

	err := store.UpdateInstallScopes(ctx, "user_scopes", "agent_scopes", []string{"read", "write", "admin"})
	if err != nil {
		t.Fatalf("UpdateInstallScopes failed: %v", err)
	}

	install, _ := store.GetInstall(ctx, "user_scopes", "agent_scopes")
	if len(install.GrantedScopes) != 3 {
		t.Errorf("expected 3 scopes, got %d", len(install.GrantedScopes))
	}

	err = store.UpdateInstallScopes(ctx, "user_nonexistent", "agent_scopes", []string{})
	if err != shared.ErrNotFound {
		t.Errorf("UpdateInstallScopes non-existent should return ErrNotFound, got %v", err)
	}
}

func TestStore_Reviews(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	store.Create(ctx, &Agent{ID: "agent_review", DeveloperID: "dev_1", Name: "Review Test"})

	review := &AgentReview{
		AgentID: "agent_review",
		UserID:  "user_review",
		Rating:  5,
		Body:    "Great agent!",
	}

	err := store.CreateReview(ctx, review)
	if err != nil {
		t.Fatalf("CreateReview failed: %v", err)
	}
	if review.ID == "" {
		t.Error("review ID should be generated")
	}

	agent, _ := store.GetByID(ctx, "agent_review")
	if agent.TotalReviews != 1 {
		t.Errorf("expected TotalReviews 1, got %d", agent.TotalReviews)
	}
	if agent.AvgRating != 5.0 {
		t.Errorf("expected AvgRating 5.0, got %f", agent.AvgRating)
	}

	reviews, err := store.GetReviews(ctx, "agent_review", 10, 0)
	if err != nil {
		t.Fatalf("GetReviews failed: %v", err)
	}
	if len(reviews) != 1 {
		t.Errorf("expected 1 review, got %d", len(reviews))
	}

	got, err := store.GetUserReview(ctx, "user_review", "agent_review")
	if err != nil {
		t.Fatalf("GetUserReview failed: %v", err)
	}
	if got.Rating != 5 {
		t.Errorf("expected rating 5, got %d", got.Rating)
	}

	_, err = store.GetUserReview(ctx, "user_nonexistent", "agent_review")
	if err != shared.ErrNotFound {
		t.Errorf("GetUserReview non-existent should return ErrNotFound, got %v", err)
	}
}

func TestStore_AddDeveloperReply(t *testing.T) {
	t.Skip("NOW function not supported in SQLite")
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)
	store.Migrate()
	ctx := context.Background()

	store.Create(ctx, &Agent{ID: "agent_reply", DeveloperID: "dev_1", Name: "Reply Test"})

	review := &AgentReview{
		ID:      "review_reply",
		AgentID: "agent_reply",
		UserID:  "user_1",
		Rating:  4,
	}
	store.CreateReview(ctx, review)

	err := store.AddDeveloperReply(ctx, "agent_reply", "review_reply", "Thank you!")
	if err != nil {
		t.Fatalf("AddDeveloperReply failed: %v", err)
	}

	updated, _ := store.GetUserReview(ctx, "user_1", "agent_reply")
	if updated.DeveloperReply == nil || *updated.DeveloperReply != "Thank you!" {
		t.Error("developer reply should be set")
	}

	err = store.AddDeveloperReply(ctx, "agent_reply", "review_nonexistent", "Reply")
	if err != shared.ErrNotFound {
		t.Errorf("AddDeveloperReply non-existent should return ErrNotFound, got %v", err)
	}
}

func TestStore_SearchByEmbedding_NilQdrant(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)

	_, err := store.SearchByEmbedding(context.Background(), []float32{0.1, 0.2}, 10)
	if err == nil {
		t.Error("expected error when qdrant is nil")
	}
}

func TestStore_UpsertEmbedding_NilQdrant(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)

	err := store.UpsertEmbedding(context.Background(), "agent_1", []float32{0.1, 0.2})
	if err == nil {
		t.Error("expected error when qdrant is nil")
	}
}

func TestStore_DeleteEmbedding_NilQdrant(t *testing.T) {
	db := setupTestAgentDB(t)
	store := NewStore(db, nil)

	err := store.DeleteEmbedding(context.Background(), "agent_1")
	if err == nil {
		t.Error("expected error when qdrant is nil")
	}
}

func TestAgent_Fields(t *testing.T) {
	now := time.Now()
	a := Agent{
		ID:             "agent_123",
		DeveloperID:    "dev_456",
		Name:           "Test Agent",
		Description:    "Description",
		LogoURL:        "https://example.com/logo.png",
		Keywords:       []string{"test"},
		Capabilities:   []string{"chat"},
		Category:       shared.AgentCategoryAssistant,
		IsPublic:       true,
		IsVerified:     true,
		TotalInstalls:  100,
		ActiveInstalls: 50,
		AvgRating:      4.5,
		TotalReviews:   10,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if a.ID != "agent_123" {
		t.Error("ID not set")
	}
	if a.DeveloperID != "dev_456" {
		t.Error("DeveloperID not set")
	}
	if !a.IsPublic {
		t.Error("IsPublic should be true")
	}
}

func TestAgentInstall_StructFields(t *testing.T) {
	now := time.Now()
	install := AgentInstall{
		ID:            "install_123",
		UserID:        "user_456",
		AgentID:       "agent_789",
		GrantedScopes: []string{"read", "write"},
		InstalledAt:   now,
		UpdatedAt:     now,
	}

	if install.ID != "install_123" {
		t.Error("ID not set")
	}
	if install.UserID != "user_456" {
		t.Error("UserID not set")
	}
	if len(install.GrantedScopes) != 2 {
		t.Error("GrantedScopes not set")
	}
}

func TestAgentReview_Fields(t *testing.T) {
	now := time.Now()
	reply := "Thank you!"
	replyTime := now
	review := AgentReview{
		ID:             "review_123",
		AgentID:        "agent_456",
		UserID:         "user_789",
		Rating:         5,
		Body:           "Great!",
		DeveloperReply: &reply,
		RepliedAt:      &replyTime,
		CreatedAt:      now,
	}

	if review.ID != "review_123" {
		t.Error("ID not set")
	}
	if review.Rating != 5 {
		t.Error("Rating not set")
	}
	if review.DeveloperReply == nil || *review.DeveloperReply != "Thank you!" {
		t.Error("DeveloperReply not set")
	}
}
