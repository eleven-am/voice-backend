package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/eleven-am/voice-backend/internal/agent"
	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/user"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	testUserID   = "test-user-123"
	testEmail    = "test@example.com"
	testUserName = "Test User"
)

func main() {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/voice?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	if err := db.AutoMigrate(
		&user.User{},
		&agent.Agent{},
		&agent.AgentInstall{},
		&apikey.APIKey{},
	); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to migrate: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	testUser, err := seedUser(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to seed user: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("User: %s (%s)\n", testUser.Name, testUser.ID)

	testAgent, err := seedAgent(db, testUser.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to seed agent: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Agent: %s (%s)\n", testAgent.Name, testAgent.ID)

	install, err := seedInstall(db, testUser.ID, testAgent.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to seed install: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Install: %s\n", install.ID)

	apikeyStore := apikey.NewStore(db)
	apiKey, secret, err := seedAPIKey(ctx, apikeyStore, testAgent.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to seed API key: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("")
	fmt.Println("=== Test Data Created ===")
	fmt.Println("")
	fmt.Printf("JWT User ID (sub): %s\n", testUser.ID)
	fmt.Printf("Agent ID: %s\n", testAgent.ID)
	fmt.Printf("API Key ID: %s\n", apiKey.ID)
	fmt.Println("")
	fmt.Println("=== Agent API Key ===")
	fmt.Printf("%s\n", secret)
	fmt.Println("")
	fmt.Println("Use this to connect an agent service to the gateway.")
}

func seedUser(db *gorm.DB) (*user.User, error) {
	u := &user.User{
		ID:          testUserID,
		Provider:    "better-auth",
		ProviderSub: testUserID,
		Email:       testEmail,
		Name:        testUserName,
		IsDeveloper: true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result := db.Where("id = ?", testUserID).FirstOrCreate(u)
	if result.Error != nil {
		return nil, result.Error
	}

	return u, nil
}

func seedAgent(db *gorm.DB, developerID string) (*agent.Agent, error) {
	agentID := "agt_echo_test"

	a := &agent.Agent{
		ID:          agentID,
		DeveloperID: developerID,
		Name:        "Echo Agent",
		Description: "A test agent that echoes back utterances",
		Keywords:    shared.StringSlice{"test", "echo", "debug"},
		Capabilities: shared.StringSlice{"echo", "test"},
		Category:    "assistant",
		IsPublic:    false,
		IsVerified:  false,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result := db.Where("id = ?", agentID).FirstOrCreate(a)
	if result.Error != nil {
		return nil, result.Error
	}

	return a, nil
}

func seedInstall(db *gorm.DB, userID, agentID string) (*agent.AgentInstall, error) {
	install := &agent.AgentInstall{
		ID:            uuid.NewString(),
		UserID:        userID,
		AgentID:       agentID,
		GrantedScopes: shared.StringSlice{"profile", "email", "location", "vision"},
		InstalledAt:   time.Now(),
		UpdatedAt:     time.Now(),
	}

	var existing agent.AgentInstall
	result := db.Where("user_id = ? AND agent_id = ?", userID, agentID).First(&existing)
	if result.Error == nil {
		return &existing, nil
	}

	if result := db.Create(install); result.Error != nil {
		return nil, result.Error
	}

	return install, nil
}

func seedAPIKey(ctx context.Context, store *apikey.Store, agentID string) (*apikey.APIKey, string, error) {
	existing, err := store.GetByOwner(ctx, agentID, apikey.OwnerTypeAgent)
	if err == nil && len(existing) > 0 {
		return existing[0], "(existing - secret not available)", nil
	}

	key := &apikey.APIKey{
		OwnerID:   agentID,
		OwnerType: apikey.OwnerTypeAgent,
		Name:      "Echo Agent Key",
	}

	secret, err := store.Create(ctx, key)
	if err != nil {
		return nil, "", err
	}

	return key, secret, nil
}
