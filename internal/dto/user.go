package dto

type MeResponse struct {
	ID          string `json:"id" example:"usr_abc123"`
	Email       string `json:"email,omitempty" example:"user@example.com"`
	Name        string `json:"name,omitempty" example:"John Doe"`
	AvatarURL   string `json:"avatar_url,omitempty" example:"https://example.com/avatar.png"`
	IsDeveloper bool   `json:"is_developer" example:"false"`
}
