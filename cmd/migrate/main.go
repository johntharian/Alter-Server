package main

import (
	"log"

	"github.com/joho/godotenv"

	"github.com/john/botsapp/internal/config"
	"github.com/john/botsapp/internal/database"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	log.Println("Running database migrations...")

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	log.Println("Migrations completed successfully")
}
