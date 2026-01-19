package apikey

import "time"

type OwnerType string

const (
	OwnerTypeUser  OwnerType = "user"
	OwnerTypeAgent OwnerType = "agent"
)

type APIKey struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	OwnerID    string     `gorm:"not null;index" json:"owner_id"`
	OwnerType  OwnerType  `gorm:"not null;index" json:"owner_type"`
	Name       string     `gorm:"not null" json:"name"`
	Prefix     string     `gorm:"uniqueIndex;not null" json:"-"`
	SecretHash string     `gorm:"not null" json:"-"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}
