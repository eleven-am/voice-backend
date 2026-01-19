package user

import (
	"testing"
)

func TestNewGoogleProvider(t *testing.T) {
	tests := []struct {
		name         string
		clientID     string
		clientSecret string
		redirectURL  string
		wantNil      bool
	}{
		{
			name:         "valid credentials",
			clientID:     "client_id",
			clientSecret: "client_secret",
			redirectURL:  "https://example.com/callback",
			wantNil:      false,
		},
		{
			name:         "empty client id",
			clientID:     "",
			clientSecret: "secret",
			redirectURL:  "https://example.com/callback",
			wantNil:      true,
		},
		{
			name:         "empty client secret",
			clientID:     "client_id",
			clientSecret: "",
			redirectURL:  "https://example.com/callback",
			wantNil:      true,
		},
		{
			name:         "both empty",
			clientID:     "",
			clientSecret: "",
			redirectURL:  "https://example.com/callback",
			wantNil:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewGoogleProvider(tt.clientID, tt.clientSecret, tt.redirectURL)
			if (p == nil) != tt.wantNil {
				t.Errorf("NewGoogleProvider() nil = %v, want %v", p == nil, tt.wantNil)
			}
		})
	}
}

func TestGoogleProvider_Name(t *testing.T) {
	p := NewGoogleProvider("id", "secret", "url")
	if p.Name() != "google" {
		t.Errorf("Name() = %v, want google", p.Name())
	}
}

func TestGoogleProvider_AuthURL(t *testing.T) {
	p := NewGoogleProvider("client_id", "client_secret", "https://example.com/callback")
	url := p.AuthURL("test_state")

	if url == "" {
		t.Error("AuthURL should not be empty")
	}
	if !containsString(url, "client_id=client_id") {
		t.Error("AuthURL should contain client_id")
	}
	if !containsString(url, "state=test_state") {
		t.Error("AuthURL should contain state")
	}
	if !containsString(url, "redirect_uri=") {
		t.Error("AuthURL should contain redirect_uri")
	}
}

func TestNewGitHubProvider(t *testing.T) {
	tests := []struct {
		name         string
		clientID     string
		clientSecret string
		redirectURL  string
		wantNil      bool
	}{
		{
			name:         "valid credentials",
			clientID:     "github_client",
			clientSecret: "github_secret",
			redirectURL:  "https://example.com/github/callback",
			wantNil:      false,
		},
		{
			name:         "empty client id",
			clientID:     "",
			clientSecret: "secret",
			redirectURL:  "url",
			wantNil:      true,
		},
		{
			name:         "empty client secret",
			clientID:     "id",
			clientSecret: "",
			redirectURL:  "url",
			wantNil:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewGitHubProvider(tt.clientID, tt.clientSecret, tt.redirectURL)
			if (p == nil) != tt.wantNil {
				t.Errorf("NewGitHubProvider() nil = %v, want %v", p == nil, tt.wantNil)
			}
		})
	}
}

func TestGitHubProvider_Name(t *testing.T) {
	p := NewGitHubProvider("id", "secret", "url")
	if p.Name() != "github" {
		t.Errorf("Name() = %v, want github", p.Name())
	}
}

func TestGitHubProvider_AuthURL(t *testing.T) {
	p := NewGitHubProvider("github_client", "github_secret", "https://example.com/callback")
	url := p.AuthURL("test_state")

	if url == "" {
		t.Error("AuthURL should not be empty")
	}
	if !containsString(url, "client_id=github_client") {
		t.Error("AuthURL should contain client_id")
	}
	if !containsString(url, "state=test_state") {
		t.Error("AuthURL should contain state")
	}
}

func TestProviderUser_Fields(t *testing.T) {
	pu := ProviderUser{
		Sub:       "sub_123",
		Email:     "test@example.com",
		Name:      "Test User",
		AvatarURL: "https://example.com/avatar.png",
	}

	if pu.Sub != "sub_123" {
		t.Error("Sub not set")
	}
	if pu.Email != "test@example.com" {
		t.Error("Email not set")
	}
	if pu.Name != "Test User" {
		t.Error("Name not set")
	}
	if pu.AvatarURL != "https://example.com/avatar.png" {
		t.Error("AvatarURL not set")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
