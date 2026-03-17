package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/john/alter/internal/api"
	"github.com/john/alter/internal/auth"
	"github.com/john/alter/internal/config"
	"github.com/john/alter/internal/database"
	"github.com/john/alter/internal/logger"
	"github.com/john/alter/internal/queue"
	redisclient "github.com/john/alter/internal/redis"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	if err := logger.Init("logs/server.log", cfg.LogLevel); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	logger.Info("Starting BotsApp API server...", nil)

	// Initialize PostgreSQL
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		logger.Error("Database connection failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer db.Close()

	// Run migrations
	if err := database.RunMigrations(db); err != nil {
		logger.Error("Migration failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}

	// Initialize Redis
	rdb, err := redisclient.Connect(cfg.RedisURL)
	if err != nil {
		logger.Error("Redis connection failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer rdb.Close()

	// Initialize RabbitMQ
	rmq, err := queue.Connect(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("RabbitMQ connection failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer rmq.Close()

	// Initialize auth services
	jwtSvc := auth.NewJWTService(cfg.JWTSecret)
	otpSvc := auth.NewOTPService(rdb, cfg.OTPTtl)

	// Create router
	router := api.NewRouter(db, rdb, rmq, jwtSvc, otpSvc, cfg)

	// Start HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.APIPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("Shutting down API server...", nil)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	logger.Info("API server listening", map[string]interface{}{"port": cfg.APIPort})
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error("Server error", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info("Server stopped", nil)
}
