package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/eleven-am/voice-backend/internal/shared"
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Migrate() error {
	return s.db.AutoMigrate(&APIKey{})
}

func (s *Store) Create(ctx context.Context, key *APIKey) (secret string, err error) {
	if key.ID == "" {
		key.ID = shared.NewID("key_")
	}

	secret, err = generateSecret()
	if err != nil {
		return "", err
	}

	key.Prefix = secret[:12]
	key.SecretHash = hashSecret(secret)

	if err := s.db.WithContext(ctx).Create(key).Error; err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Store) GetByID(ctx context.Context, id string) (*APIKey, error) {
	var key APIKey
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, shared.ErrNotFound
	}
	return &key, err
}

func (s *Store) GetByOwner(ctx context.Context, ownerID string, ownerType OwnerType) ([]*APIKey, error) {
	var keys []*APIKey
	err := s.db.WithContext(ctx).Where("owner_id = ? AND owner_type = ?", ownerID, ownerType).Find(&keys).Error
	return keys, err
}

func (s *Store) Validate(ctx context.Context, secret string) (*APIKey, error) {
	if len(secret) < 12 {
		return nil, shared.ErrNotFound
	}

	prefix := secret[:12]
	var key APIKey
	err := s.db.WithContext(ctx).Where("prefix = ?", prefix).First(&key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if key.SecretHash != hashSecret(secret) {
		return nil, shared.ErrNotFound
	}

	if key.IsExpired() {
		return nil, shared.ErrUnauthorized
	}

	go s.updateLastUsed(key.ID)

	return &key, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	result := s.db.WithContext(ctx).Delete(&APIKey{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteByOwner(ctx context.Context, ownerID string, ownerType OwnerType) error {
	return s.db.WithContext(ctx).Delete(&APIKey{}, "owner_id = ? AND owner_type = ?", ownerID, ownerType).Error
}

func (s *Store) updateLastUsed(id string) {
	s.db.Model(&APIKey{}).Where("id = ?", id).Update("last_used_at", time.Now())
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sk-voice-" + hex.EncodeToString(b), nil
}

func hashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}
