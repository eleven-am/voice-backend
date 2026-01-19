package apikey

import (
	"context"
	"testing"
	"time"

	"github.com/eleven-am/voice-backend/internal/shared"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	return db
}

func TestNewStore(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	if store == nil {
		t.Error("expected non-nil store")
	}
	if store.db != db {
		t.Error("expected db to be set")
	}
}

func TestStore_Migrate(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	if err := store.Migrate(); err != nil {
		t.Errorf("Migrate() error = %v", err)
	}

	if !db.Migrator().HasTable(&APIKey{}) {
		t.Error("expected APIKey table to exist")
	}
}

func TestStore_Create(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	key := &APIKey{
		OwnerID:   "agent_123",
		OwnerType: OwnerTypeAgent,
		Name:      "Test Key",
	}

	secret, err := store.Create(ctx, key)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if secret == "" {
		t.Error("expected non-empty secret")
	}

	if !hasPrefix(secret, "sk-voice-") {
		t.Errorf("expected secret to start with 'sk-voice-', got %q", secret[:15])
	}

	if key.ID == "" {
		t.Error("expected ID to be set")
	}

	if key.Prefix == "" {
		t.Error("expected prefix to be set")
	}

	if key.SecretHash == "" {
		t.Error("expected secret hash to be set")
	}
}

func TestStore_Create_WithExistingID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	existingID := "key_existing"
	key := &APIKey{
		ID:        existingID,
		OwnerID:   "agent_123",
		OwnerType: OwnerTypeAgent,
		Name:      "Test Key",
	}

	_, err := store.Create(ctx, key)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if key.ID != existingID {
		t.Errorf("ID = %q, want %q", key.ID, existingID)
	}
}

func TestStore_GetByID(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	key := &APIKey{
		OwnerID:   "agent_123",
		OwnerType: OwnerTypeAgent,
		Name:      "Test Key",
	}
	_, _ = store.Create(ctx, key)

	found, err := store.GetByID(ctx, key.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if found.ID != key.ID {
		t.Errorf("ID = %q, want %q", found.ID, key.ID)
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	_, err := store.GetByID(ctx, "nonexistent")

	if err != shared.ErrNotFound {
		t.Errorf("error = %v, want %v", err, shared.ErrNotFound)
	}
}

func TestStore_GetByOwner(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	ownerID := "agent_123"

	for i := 0; i < 3; i++ {
		key := &APIKey{
			OwnerID:   ownerID,
			OwnerType: OwnerTypeAgent,
			Name:      "Test Key",
		}
		_, _ = store.Create(ctx, key)
	}

	otherKey := &APIKey{
		OwnerID:   "agent_other",
		OwnerType: OwnerTypeAgent,
		Name:      "Other Key",
	}
	_, _ = store.Create(ctx, otherKey)

	keys, err := store.GetByOwner(ctx, ownerID, OwnerTypeAgent)
	if err != nil {
		t.Fatalf("GetByOwner() error = %v", err)
	}

	if len(keys) != 3 {
		t.Errorf("got %d keys, want 3", len(keys))
	}
}

func TestStore_Validate(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	key := &APIKey{
		OwnerID:   "agent_123",
		OwnerType: OwnerTypeAgent,
		Name:      "Test Key",
	}
	secret, _ := store.Create(ctx, key)

	found, err := store.Validate(ctx, secret)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if found.ID != key.ID {
		t.Errorf("ID = %q, want %q", found.ID, key.ID)
	}
}

func TestStore_Validate_ShortSecret(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	_, err := store.Validate(ctx, "short")

	if err != shared.ErrNotFound {
		t.Errorf("error = %v, want %v", err, shared.ErrNotFound)
	}
}

func TestStore_Validate_InvalidSecret(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	key := &APIKey{
		OwnerID:   "agent_123",
		OwnerType: OwnerTypeAgent,
		Name:      "Test Key",
	}
	secret, _ := store.Create(ctx, key)

	invalidSecret := secret[:12] + "wrongsecret123456789012345678901234567890"
	_, err := store.Validate(ctx, invalidSecret)

	if err != shared.ErrNotFound {
		t.Errorf("error = %v, want %v", err, shared.ErrNotFound)
	}
}

func TestStore_Validate_Expired(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	expired := time.Now().Add(-time.Hour)
	key := &APIKey{
		OwnerID:   "agent_123",
		OwnerType: OwnerTypeAgent,
		Name:      "Test Key",
		ExpiresAt: &expired,
	}
	secret, _ := store.Create(ctx, key)

	_, err := store.Validate(ctx, secret)

	if err != shared.ErrUnauthorized {
		t.Errorf("error = %v, want %v", err, shared.ErrUnauthorized)
	}
}

func TestStore_Validate_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	_, err := store.Validate(ctx, "sk-voice-nonexistent1234567890")

	if err != shared.ErrNotFound {
		t.Errorf("error = %v, want %v", err, shared.ErrNotFound)
	}
}

func TestStore_Delete(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	key := &APIKey{
		OwnerID:   "agent_123",
		OwnerType: OwnerTypeAgent,
		Name:      "Test Key",
	}
	_, _ = store.Create(ctx, key)

	if err := store.Delete(ctx, key.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err := store.GetByID(ctx, key.ID)
	if err != shared.ErrNotFound {
		t.Errorf("expected key to be deleted, got error = %v", err)
	}
}

func TestStore_Delete_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	err := store.Delete(ctx, "nonexistent")

	if err != shared.ErrNotFound {
		t.Errorf("error = %v, want %v", err, shared.ErrNotFound)
	}
}

func TestStore_DeleteByOwner(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	_ = store.Migrate()

	ctx := context.Background()
	ownerID := "agent_123"

	for i := 0; i < 3; i++ {
		key := &APIKey{
			OwnerID:   ownerID,
			OwnerType: OwnerTypeAgent,
			Name:      "Test Key",
		}
		_, _ = store.Create(ctx, key)
	}

	if err := store.DeleteByOwner(ctx, ownerID, OwnerTypeAgent); err != nil {
		t.Fatalf("DeleteByOwner() error = %v", err)
	}

	keys, _ := store.GetByOwner(ctx, ownerID, OwnerTypeAgent)
	if len(keys) != 0 {
		t.Errorf("expected 0 keys after delete, got %d", len(keys))
	}
}

func TestGenerateSecret(t *testing.T) {
	secret, err := generateSecret()
	if err != nil {
		t.Fatalf("generateSecret() error = %v", err)
	}

	if !hasPrefix(secret, "sk-voice-") {
		t.Errorf("expected secret to start with 'sk-voice-', got %q", secret)
	}

	if len(secret) < 64 {
		t.Errorf("secret too short: %d", len(secret))
	}

	secret2, _ := generateSecret()
	if secret == secret2 {
		t.Error("expected unique secrets")
	}
}

func TestHashSecret(t *testing.T) {
	hash1 := hashSecret("test-secret")
	hash2 := hashSecret("test-secret")
	hash3 := hashSecret("different-secret")

	if hash1 != hash2 {
		t.Error("same secret should produce same hash")
	}

	if hash1 == hash3 {
		t.Error("different secrets should produce different hashes")
	}

	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash1))
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
