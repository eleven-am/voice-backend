package auth

import "github.com/golang-jwt/jwt/v5"

type Claims struct {
	jwt.RegisteredClaims
	UserID    string `json:"sub"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"picture,omitempty"`
}
