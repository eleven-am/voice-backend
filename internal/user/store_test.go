package user

import (
	"context"
	"testing"

	"github.com/eleven-am/voice-backend/internal/shared"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestUserDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	return db
}

func TestNewStore(t *testing.T) {
	db := setupTestUserDB(t)
	store := NewStore(db)
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestStore_Migrate(t *testing.T) {
	db := setupTestUserDB(t)
	store := NewStore(db)

	err := store.Migrate()
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	var tables []string
	db.Raw("SELECT name FROM sqlite_master WHERE type='table'").Scan(&tables)
	found := false
	for _, table := range tables {
		if table == "users" {
			found = true
			break
		}
	}
	if !found {
		t.Error("users table should exist after migration")
	}
}

func TestStore_Create(t *testing.T) {
	db := setupTestUserDB(t)
	store := NewStore(db)
	store.Migrate()
	ctx := context.Background()

	tests := []struct {
		name    string
		user    *User
		wantErr bool
	}{
		{
			name: "create user with id",
			user: &User{
				ID:          "user_test123",
				Provider:    "google",
				ProviderSub: "sub123",
				Email:       "test@example.com",
				Name:        "Test User",
			},
			wantErr: false,
		},
		{
			name: "create user without id",
			user: &User{
				Provider:    "github",
				ProviderSub: "sub456",
				Email:       "test2@example.com",
				Name:        "Test User 2",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Create(ctx, tt.user)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.user.ID == "" {
				t.Error("user ID should be generated if not provided")
			}
		})
	}
}

func TestStore_GetByID(t *testing.T) {
	db := setupTestUserDB(t)
	store := NewStore(db)
	store.Migrate()
	ctx := context.Background()

	user := &User{
		ID:          "user_getbyid",
		Provider:    "google",
		ProviderSub: "sub_getbyid",
		Email:       "getbyid@example.com",
		Name:        "GetByID User",
	}
	store.Create(ctx, user)

	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{
			name:    "existing user",
			id:      "user_getbyid",
			wantErr: nil,
		},
		{
			name:    "non-existent user",
			id:      "user_nonexistent",
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

func TestStore_GetByProvider(t *testing.T) {
	db := setupTestUserDB(t)
	store := NewStore(db)
	store.Migrate()
	ctx := context.Background()

	user := &User{
		ID:          "user_provider",
		Provider:    "google",
		ProviderSub: "google_sub_123",
		Email:       "provider@example.com",
	}
	store.Create(ctx, user)

	tests := []struct {
		name     string
		provider string
		sub      string
		wantErr  error
	}{
		{
			name:     "existing provider",
			provider: "google",
			sub:      "google_sub_123",
			wantErr:  nil,
		},
		{
			name:     "wrong provider",
			provider: "github",
			sub:      "google_sub_123",
			wantErr:  shared.ErrNotFound,
		},
		{
			name:     "wrong sub",
			provider: "google",
			sub:      "wrong_sub",
			wantErr:  shared.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.GetByProvider(ctx, tt.provider, tt.sub)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("GetByProvider() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Errorf("GetByProvider() unexpected error = %v", err)
				}
				if got.Provider != tt.provider || got.ProviderSub != tt.sub {
					t.Errorf("GetByProvider() got wrong user")
				}
			}
		})
	}
}

func TestStore_FindOrCreate(t *testing.T) {
	db := setupTestUserDB(t)
	store := NewStore(db)
	store.Migrate()
	ctx := context.Background()

	user, err := store.FindOrCreate(ctx, "google", "new_sub", "new@example.com", "New User", "https://avatar.url")
	if err != nil {
		t.Fatalf("FindOrCreate failed: %v", err)
	}
	if user.ID == "" {
		t.Error("user ID should be set")
	}
	if user.Email != "new@example.com" {
		t.Error("email should match")
	}

	user2, err := store.FindOrCreate(ctx, "google", "new_sub", "updated@example.com", "Updated Name", "https://new-avatar.url")
	if err != nil {
		t.Fatalf("FindOrCreate second call failed: %v", err)
	}
	if user2.ID != user.ID {
		t.Error("should return same user")
	}
	if user2.Email != "updated@example.com" {
		t.Error("email should be updated")
	}
	if user2.Name != "Updated Name" {
		t.Error("name should be updated")
	}
	if user2.AvatarURL != "https://new-avatar.url" {
		t.Error("avatar URL should be updated")
	}

	user3, err := store.FindOrCreate(ctx, "google", "new_sub", "updated@example.com", "Updated Name", "https://new-avatar.url")
	if err != nil {
		t.Fatalf("FindOrCreate no-change call failed: %v", err)
	}
	if user3.ID != user.ID {
		t.Error("should return same user when no changes")
	}
}

func TestStore_SetDeveloper(t *testing.T) {
	db := setupTestUserDB(t)
	store := NewStore(db)
	store.Migrate()
	ctx := context.Background()

	user := &User{
		ID:          "user_dev",
		Provider:    "google",
		ProviderSub: "dev_sub",
		IsDeveloper: false,
	}
	store.Create(ctx, user)

	err := store.SetDeveloper(ctx, "user_dev", true)
	if err != nil {
		t.Fatalf("SetDeveloper failed: %v", err)
	}

	updated, _ := store.GetByID(ctx, "user_dev")
	if !updated.IsDeveloper {
		t.Error("user should be developer")
	}

	err = store.SetDeveloper(ctx, "user_dev", false)
	if err != nil {
		t.Fatalf("SetDeveloper(false) failed: %v", err)
	}

	updated, _ = store.GetByID(ctx, "user_dev")
	if updated.IsDeveloper {
		t.Error("user should not be developer")
	}

	err = store.SetDeveloper(ctx, "nonexistent_user", true)
	if err != shared.ErrNotFound {
		t.Errorf("SetDeveloper non-existent should return ErrNotFound, got %v", err)
	}
}

func TestUser_Fields(t *testing.T) {
	u := User{
		ID:          "user_123",
		Provider:    "google",
		ProviderSub: "sub_456",
		Email:       "test@example.com",
		Name:        "Test User",
		AvatarURL:   "https://example.com/avatar.png",
		IsDeveloper: true,
	}

	if u.ID != "user_123" {
		t.Error("ID not set")
	}
	if u.Provider != "google" {
		t.Error("Provider not set")
	}
	if u.Email != "test@example.com" {
		t.Error("Email not set")
	}
	if u.Name != "Test User" {
		t.Error("Name not set")
	}
	if !u.IsDeveloper {
		t.Error("IsDeveloper should be true")
	}
}
