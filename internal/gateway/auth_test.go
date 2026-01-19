package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/eleven-am/voice-backend/internal/apikey"
)

type mockAPIKeyValidator struct {
	validateFunc func(ctx context.Context, secret string) (*apikey.APIKey, error)
}

func (m *mockAPIKeyValidator) Validate(ctx context.Context, secret string) (*apikey.APIKey, error) {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, secret)
	}
	return nil, errors.New("not implemented")
}

func TestNewAuthenticator(t *testing.T) {
	mock := &mockAPIKeyValidator{}
	auth := NewAuthenticator(mock)
	if auth == nil {
		t.Error("expected non-nil authenticator")
	}
	if auth.apiKeyStore != mock {
		t.Error("expected apiKeyStore to be set")
	}
}

func TestAuthenticator_ValidateAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		mockKey *apikey.APIKey
		mockErr error
		wantErr error
		wantKey bool
	}{
		{
			name:   "valid key",
			secret: "sk-valid-key",
			mockKey: &apikey.APIKey{
				ID:        "key_123",
				OwnerID:   "agent_456",
				OwnerType: apikey.OwnerTypeAgent,
			},
			mockErr: nil,
			wantErr: nil,
			wantKey: true,
		},
		{
			name:    "invalid key",
			secret:  "sk-invalid",
			mockKey: nil,
			mockErr: errors.New("key not found"),
			wantErr: ErrInvalidAPIKey,
			wantKey: false,
		},
		{
			name:    "empty secret",
			secret:  "",
			mockKey: nil,
			mockErr: errors.New("empty secret"),
			wantErr: ErrInvalidAPIKey,
			wantKey: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockAPIKeyValidator{
				validateFunc: func(ctx context.Context, secret string) (*apikey.APIKey, error) {
					if secret != tt.secret {
						t.Errorf("unexpected secret: got %q, want %q", secret, tt.secret)
					}
					return tt.mockKey, tt.mockErr
				},
			}

			auth := NewAuthenticator(mock)
			key, err := auth.ValidateAPIKey(context.Background(), tt.secret)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if tt.wantKey && key == nil {
				t.Error("expected key, got nil")
			}
			if !tt.wantKey && key != nil {
				t.Error("expected nil key")
			}
		})
	}
}

func TestAuthenticator_ValidateAgentAccess(t *testing.T) {
	tests := []struct {
		name    string
		key     *apikey.APIKey
		agentID string
		wantErr error
	}{
		{
			name: "valid agent access",
			key: &apikey.APIKey{
				OwnerID:   "agent_123",
				OwnerType: apikey.OwnerTypeAgent,
			},
			agentID: "agent_123",
			wantErr: nil,
		},
		{
			name: "user key not agent",
			key: &apikey.APIKey{
				OwnerID:   "user_123",
				OwnerType: apikey.OwnerTypeUser,
			},
			agentID: "agent_123",
			wantErr: ErrNotAgentKey,
		},
		{
			name: "agent id mismatch",
			key: &apikey.APIKey{
				OwnerID:   "agent_456",
				OwnerType: apikey.OwnerTypeAgent,
			},
			agentID: "agent_123",
			wantErr: ErrAgentIDMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &Authenticator{}
			err := auth.ValidateAgentAccess(tt.key, tt.agentID)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
