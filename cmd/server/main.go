package main

import (
	_ "github.com/eleven-am/voice-backend/docs"
	"github.com/eleven-am/voice-backend/internal/bootstrap"
)

// @title Voice Backend API
// @version 1.0.0
// @description API server for voice agent platform

// @BasePath /v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description JWT token from Better Auth. Format: Bearer <token>

// @securityDefinitions.apikey APIKeyAuth
// @in header
// @name X-API-Key
// @description API key for agent connections

func main() {
	bootstrap.Run()
}
