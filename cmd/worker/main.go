package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/john/alter/internal/config"
	"github.com/john/alter/internal/database"
	"github.com/john/alter/internal/logger"
	"github.com/john/alter/internal/queue"
	redisclient "github.com/john/alter/internal/redis"
	"github.com/john/alter/internal/worker"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	if err := logger.Init("logs/worker.log", cfg.LogLevel); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	logger.Info("Starting Alter Delivery Worker...", nil)

	// Initialize PostgreSQL
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		logger.Error("Database connection failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer db.Close()

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

	// Create consumer
	consumer := worker.NewConsumer(db, rdb, rmq)

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("Shutting down worker...", nil)
		cancel()
	}()

	if err := consumer.Start(ctx); err != nil {
		logger.Error("Worker error", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	logger.Info("Worker stopped", nil)
}
