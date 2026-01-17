package main

import (
	_ "github.com/eleven-am/voice-backend/docs"
	"github.com/eleven-am/voice-backend/internal/bootstrap"
)

// @title Voice Backend API
// @version 1.0.0
// @description API server for voice agent platform

// @host api.voice.example.com
// @BasePath /api/v1

// @securityDefinitions.apikey SessionAuth
// @in cookie
// @name session

// @securityDefinitions.apikey APIKeyAuth
// @in header
// @name X-API-Key

func main() {
	bootstrap.Run()
}
