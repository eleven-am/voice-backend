package user

import "time"

type User struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	Provider    string    `gorm:"not null;index:idx_provider_sub,unique" json:"provider"`
	ProviderSub string    `gorm:"not null;index:idx_provider_sub,unique" json:"-"`
	Email       string    `gorm:"index" json:"email,omitempty"`
	Name        string    `json:"name,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	IsDeveloper bool      `gorm:"default:false" json:"is_developer"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
