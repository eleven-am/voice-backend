package user

import (
	"context"
	"errors"

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
	return s.db.AutoMigrate(&User{})
}

func (s *Store) Create(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = shared.NewID("user_")
	}
	return s.db.WithContext(ctx).Create(u).Error
}

func (s *Store) GetByID(ctx context.Context, id string) (*User, error) {
	var u User
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, shared.ErrNotFound
	}
	return &u, err
}

func (s *Store) GetByProvider(ctx context.Context, provider, sub string) (*User, error) {
	var u User
	err := s.db.WithContext(ctx).Where("provider = ? AND provider_sub = ?", provider, sub).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, shared.ErrNotFound
	}
	return &u, err
}

func (s *Store) FindOrCreate(ctx context.Context, provider, sub, email, name, avatar string) (*User, error) {
	u, err := s.GetByProvider(ctx, provider, sub)
	if err == nil {
		if u.Email != email || u.Name != name || u.AvatarURL != avatar {
			u.Email = email
			u.Name = name
			u.AvatarURL = avatar
			if err := s.db.WithContext(ctx).Save(u).Error; err != nil {
				return nil, err
			}
		}
		return u, nil
	}

	if !errors.Is(err, shared.ErrNotFound) {
		return nil, err
	}

	u = &User{
		ID:          shared.NewID("user_"),
		Provider:    provider,
		ProviderSub: sub,
		Email:       email,
		Name:        name,
		AvatarURL:   avatar,
	}

	if err := s.db.WithContext(ctx).Create(u).Error; err != nil {
		return nil, err
	}

	return u, nil
}

func (s *Store) SetDeveloper(ctx context.Context, id string, isDeveloper bool) error {
	result := s.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("is_developer", isDeveloper)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (s *Store) FindOrCreateFromJWT(ctx context.Context, userID, email, name, avatar string) (*User, error) {
	var u User
	err := s.db.WithContext(ctx).Where("id = ?", userID).First(&u).Error
	if err == nil {
		if u.Email != email || u.Name != name || u.AvatarURL != avatar {
			u.Email = email
			u.Name = name
			u.AvatarURL = avatar
			if err := s.db.WithContext(ctx).Save(&u).Error; err != nil {
				return nil, err
			}
		}
		return &u, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	u = User{
		ID:          userID,
		Provider:    "better-auth",
		ProviderSub: userID,
		Email:       email,
		Name:        name,
		AvatarURL:   avatar,
	}

	if err := s.db.WithContext(ctx).Create(&u).Error; err != nil {
		return nil, err
	}

	return &u, nil
}

func (s *Store) SyncFromJWT(ctx context.Context, userID, email, name, avatar string) error {
	_, err := s.FindOrCreateFromJWT(ctx, userID, email, name, avatar)
	return err
}
