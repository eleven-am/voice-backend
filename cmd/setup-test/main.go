package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/user"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		dsn = "postgres://voice:voice@localhost:5432/voice?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatal("connect db:", err)
	}

	ctx := context.Background()
	userID := "test-user"
	agentID := "test-agent-001"

	testUser := &user.User{
		ID:          userID,
		Provider:    "test",
		ProviderSub: "test-sub-001",
		Email:       "test@example.com",
		Name:        "Test User",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.WithContext(ctx).FirstOrCreate(testUser, "id = ?", userID).Error; err != nil {
		log.Fatal("create user:", err)
	}
	fmt.Println("User ID:", userID)

	testAgent := &agent.Agent{
		ID:          agentID,
		DeveloperID: userID,
		Name:        "Test Echo Agent",
		Description: "A simple echo agent for testing",
		Category:    shared.AgentCategoryAssistant,
		IsPublic:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.WithContext(ctx).FirstOrCreate(testAgent, "id = ?", agentID).Error; err != nil {
		log.Fatal("create agent:", err)
	}
	fmt.Println("Agent ID:", agentID)

	install := &agent.AgentInstall{
		ID:          shared.NewID("install_"),
		UserID:      userID,
		AgentID:     agentID,
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.WithContext(ctx).FirstOrCreate(install, "user_id = ? AND agent_id = ?", userID, agentID).Error; err != nil {
		log.Fatal("install agent:", err)
	}
	fmt.Println("Agent installed for user")

	apiKeyStore := apikey.NewStore(db)
	key := &apikey.APIKey{
		ID:        shared.NewID("key_"),
		OwnerID:   agentID,
		OwnerType: apikey.OwnerTypeAgent,
		Name:      "Test Agent Key",
	}

	secret, err := apiKeyStore.Create(ctx, key)
	if err != nil {
		log.Fatal("create key:", err)
	}
	fmt.Println("API Key:", secret)
}
