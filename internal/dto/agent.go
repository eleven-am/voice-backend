package dto

type CreateAgentRequest struct {
	Name         string   `json:"name" example:"My Voice Agent"`
	Description  string   `json:"description,omitempty" example:"A helpful voice assistant"`
	LogoURL      string   `json:"logo_url,omitempty" example:"https://example.com/logo.png"`
	Keywords     []string `json:"keywords,omitempty" example:"helpful,assistant,voice"`
	Capabilities []string `json:"capabilities,omitempty" example:"search,weather,calendar"`
	Category     string   `json:"category,omitempty" example:"productivity" enums:"productivity,education,entertainment,lifestyle,utility,social,finance,health,news,assistant"`
}

type UpdateAgentRequest struct {
	Name         *string  `json:"name,omitempty" example:"Updated Agent Name"`
	Description  *string  `json:"description,omitempty" example:"Updated description"`
	LogoURL      *string  `json:"logo_url,omitempty" example:"https://example.com/new-logo.png"`
	Keywords     []string `json:"keywords,omitempty" example:"updated,keywords"`
	Capabilities []string `json:"capabilities,omitempty" example:"new,capabilities"`
	Category     *string  `json:"category,omitempty" example:"education" enums:"productivity,education,entertainment,lifestyle,utility,social,finance,health,news,assistant"`
}

type AgentResponse struct {
	ID             string   `json:"id" example:"agt_abc123"`
	DeveloperID    string   `json:"developer_id" example:"usr_xyz789"`
	Name           string   `json:"name" example:"My Voice Agent"`
	Description    string   `json:"description,omitempty" example:"A helpful voice assistant"`
	LogoURL        string   `json:"logo_url,omitempty" example:"https://example.com/logo.png"`
	Keywords       []string `json:"keywords,omitempty" example:"helpful,assistant"`
	Capabilities   []string `json:"capabilities,omitempty" example:"search,weather"`
	Category       string   `json:"category" example:"productivity"`
	IsPublic       bool     `json:"is_public" example:"true"`
	IsVerified     bool     `json:"is_verified" example:"false"`
	TotalInstalls  int64    `json:"total_installs" example:"1000"`
	ActiveInstalls int64    `json:"active_installs" example:"500"`
	AvgRating      float32  `json:"avg_rating" example:"4.5"`
	TotalReviews   int64    `json:"total_reviews" example:"100"`
	CreatedAt      string   `json:"created_at" example:"2024-01-15T10:30:00Z"`
	UpdatedAt      string   `json:"updated_at" example:"2024-01-20T15:45:00Z"`
}

type AgentListResponse struct {
	Agents []AgentResponse `json:"agents"`
}

type ReplyToReviewRequest struct {
	Reply string `json:"reply" example:"Thank you for your feedback!"`
}

type MarketplaceAgentResponse struct {
	ID            string   `json:"id" example:"agt_abc123"`
	Name          string   `json:"name" example:"My Voice Agent"`
	Description   string   `json:"description,omitempty" example:"A helpful voice assistant"`
	LogoURL       string   `json:"logo_url,omitempty" example:"https://example.com/logo.png"`
	Keywords      []string `json:"keywords,omitempty" example:"helpful,assistant"`
	Category      string   `json:"category" example:"productivity"`
	IsVerified    bool     `json:"is_verified" example:"true"`
	TotalInstalls int64    `json:"total_installs" example:"5000"`
	AvgRating     float32  `json:"avg_rating" example:"4.8"`
	TotalReviews  int64    `json:"total_reviews" example:"200"`
}

type MarketplaceListResponse struct {
	Agents []MarketplaceAgentResponse `json:"agents"`
	Limit  int                        `json:"limit" example:"20"`
	Offset int                        `json:"offset" example:"0"`
}

type MarketplaceSearchResponse struct {
	Agents []MarketplaceAgentResponse `json:"agents"`
}

type ReviewResponse struct {
	ID             string  `json:"id" example:"rev_abc123"`
	UserID         string  `json:"user_id" example:"usr_xyz789"`
	Rating         int     `json:"rating" example:"5"`
	Body           string  `json:"body,omitempty" example:"Great agent!"`
	DeveloperReply *string `json:"developer_reply,omitempty" example:"Thank you!"`
	RepliedAt      *string `json:"replied_at,omitempty" example:"2024-01-16T12:00:00Z"`
	CreatedAt      string  `json:"created_at" example:"2024-01-15T10:30:00Z"`
}

type ReviewListResponse struct {
	Reviews []ReviewResponse `json:"reviews"`
	Limit   int              `json:"limit" example:"20"`
	Offset  int              `json:"offset" example:"0"`
}

type CreateReviewRequest struct {
	Rating int    `json:"rating" example:"5" minimum:"1" maximum:"5"`
	Body   string `json:"body,omitempty" example:"Great agent, very helpful!"`
}

type InstalledAgentResponse struct {
	AgentID       string   `json:"agent_id" example:"agt_abc123"`
	Name          string   `json:"name" example:"My Voice Agent"`
	Description   string   `json:"description,omitempty" example:"A helpful voice assistant"`
	LogoURL       string   `json:"logo_url,omitempty" example:"https://example.com/logo.png"`
	GrantedScopes []string `json:"granted_scopes" example:"profile,email"`
	InstalledAt   string   `json:"installed_at" example:"2024-01-15T10:30:00Z"`
}

type InstalledAgentsResponse struct {
	Agents []InstalledAgentResponse `json:"agents"`
}

type InstallRequest struct {
	Scopes []string `json:"scopes" example:"profile,email,realtime"`
}

type UpdateScopesRequest struct {
	Scopes []string `json:"scopes" example:"profile,email"`
}
