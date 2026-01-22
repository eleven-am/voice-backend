package main

import (
	_ "github.com/eleven-am/voice-backend/docs"
	"github.com/eleven-am/voice-backend/internal/bootstrap"
)

// @title Voice Backend API
// @version 1.0.0
// @description API server for voice agent platform

// @BasePath /v1

// @securitydefinitions.bearerauth BearerAuth
// @in header
// @name Authorization
// @description JWT token from Better Auth

// @securitydefinitions.bearerauth APIKeyAuth
// @in header
// @name Authorization
// @description API key for audio endpoints (OpenAI-compatible format: Bearer sk-voice-...)

func main() {
	bootstrap.Run()
}
