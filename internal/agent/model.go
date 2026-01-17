package agent

import (
	"time"

	"github.com/eleven-am/voice-backend/internal/shared"
)

type Agent struct {
	ID          string `gorm:"primaryKey" json:"id"`
	DeveloperID string `gorm:"not null;index" json:"developer_id"`

	Name        string              `gorm:"not null" json:"name"`
	Description string              `json:"description,omitempty"`
	LogoURL     string              `json:"logo_url,omitempty"`
	Keywords    shared.StringSlice  `gorm:"type:json" json:"keywords,omitempty"`
	Capabilities shared.StringSlice `gorm:"type:json" json:"capabilities,omitempty"`
	Category    shared.AgentCategory `gorm:"default:'assistant'" json:"category"`

	IsPublic   bool `gorm:"default:false" json:"is_public"`
	IsVerified bool `gorm:"default:false" json:"is_verified"`

	APIKeyPrefix string `gorm:"uniqueIndex" json:"-"`
	APIKeyHash   string `json:"-"`

	TotalInstalls  int64   `gorm:"default:0" json:"total_installs"`
	ActiveInstalls int64   `gorm:"default:0" json:"active_installs"`
	TotalSessions  int64   `gorm:"default:0" json:"total_sessions"`
	TotalResponses int64   `gorm:"default:0" json:"total_responses"`
	AvgRating      float32 `gorm:"default:0" json:"avg_rating"`
	TotalReviews   int64   `gorm:"default:0" json:"total_reviews"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AgentInstall struct {
	ID            string             `gorm:"primaryKey" json:"id"`
	UserID        string             `gorm:"not null;index:idx_user_agent,unique" json:"user_id"`
	AgentID       string             `gorm:"not null;index:idx_user_agent,unique" json:"agent_id"`
	GrantedScopes shared.StringSlice `gorm:"type:json" json:"granted_scopes"`
	InstalledAt   time.Time          `json:"installed_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

type AgentReview struct {
	ID             string     `gorm:"primaryKey" json:"id"`
	AgentID        string     `gorm:"not null;index" json:"agent_id"`
	UserID         string     `gorm:"not null;index:idx_agent_user_review,unique" json:"user_id"`
	Rating         int        `gorm:"not null;check:rating >= 1 AND rating <= 5" json:"rating"`
	Body           string     `json:"body,omitempty"`
	DeveloperReply *string    `json:"developer_reply,omitempty"`
	RepliedAt      *time.Time `json:"replied_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}
