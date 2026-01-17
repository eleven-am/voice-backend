package dto

type APIKeyResponse struct {
	ID        string  `json:"id" example:"key_abc123"`
	Name      string  `json:"name" example:"Production Key"`
	Prefix    string  `json:"prefix" example:"sk-voice-abc"`
	CreatedAt string  `json:"created_at" example:"2024-01-15T10:30:00Z"`
	ExpiresAt *string `json:"expires_at,omitempty" example:"2024-12-31T23:59:59Z"`
	LastUsed  *string `json:"last_used_at,omitempty" example:"2024-01-20T15:45:00Z"`
}

type APIKeyListResponse struct {
	APIKeys []APIKeyResponse `json:"api_keys"`
}

type CreateAPIKeyRequest struct {
	Name      string `json:"name" example:"Production Key"`
	ExpiresIn *int   `json:"expires_in_days,omitempty" example:"90"`
}

type CreateAPIKeyResponse struct {
	ID        string  `json:"id" example:"key_abc123"`
	Name      string  `json:"name" example:"Production Key"`
	Prefix    string  `json:"prefix" example:"sk-voice-abc"`
	CreatedAt string  `json:"created_at" example:"2024-01-15T10:30:00Z"`
	ExpiresAt *string `json:"expires_at,omitempty" example:"2024-12-31T23:59:59Z"`
	LastUsed  *string `json:"last_used_at,omitempty"`
	Secret    string  `json:"secret" example:"sk-voice-abcXXXXXXXXXXXXXXXXXXXXX"`
}
