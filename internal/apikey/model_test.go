package apikey

import (
	"testing"
	"time"
)

func TestAPIKey_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt *time.Time
		want      bool
	}{
		{
			name:      "no expiration",
			expiresAt: nil,
			want:      false,
		},
		{
			name:      "expired",
			expiresAt: timePtr(time.Now().Add(-time.Hour)),
			want:      true,
		},
		{
			name:      "not expired",
			expiresAt: timePtr(time.Now().Add(time.Hour)),
			want:      false,
		},
		{
			name:      "expired just now",
			expiresAt: timePtr(time.Now().Add(-time.Millisecond)),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &APIKey{ExpiresAt: tt.expiresAt}
			if got := key.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
