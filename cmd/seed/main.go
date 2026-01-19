package main

import (
	"context"
	"fmt"
	"os"

	"github.com/eleven-am/voice-backend/internal/apikey"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

	if err := db.AutoMigrate(&apikey.APIKey{}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to migrate: %v\n", err)
		os.Exit(1)
	}

	store := apikey.NewStore(db)

	key := &apikey.APIKey{
		OwnerID:   "admin",
		OwnerType: apikey.OwnerTypeUser,
		Name:      "Admin API Key",
	}

	secret, err := store.Create(context.Background(), key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create API key: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Admin API key created successfully!")
	fmt.Println("")
	fmt.Printf("API Key: %s\n", secret)
	fmt.Println("")
	fmt.Println("Use this key in the Authorization header:")
	fmt.Printf("  Authorization: Bearer %s\n", secret)
}
