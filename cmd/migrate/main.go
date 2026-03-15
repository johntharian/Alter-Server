package main

import (
	"os"

	"github.com/joho/godotenv"

	"github.com/john/botsapp/internal/config"
	"github.com/john/botsapp/internal/database"
	"github.com/john/botsapp/internal/logger"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	if err := logger.Init("logs/migrate.log", cfg.LogLevel); err != nil {
		os.Exit(1)
	}
	defer logger.Close()

	logger.Info("Running database migrations...", nil)

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		logger.Error("Database connection failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		logger.Error("Migration failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}

	logger.Info("Migrations completed successfully", nil)
}
