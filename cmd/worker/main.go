package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/john/botsapp/internal/config"
	"github.com/john/botsapp/internal/database"
	"github.com/john/botsapp/internal/queue"
	redisclient "github.com/john/botsapp/internal/redis"
	"github.com/john/botsapp/internal/worker"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	log.Println("Starting BotsApp Delivery Worker...")

	// Initialize PostgreSQL
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	// Initialize Redis
	rdb, err := redisclient.Connect(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}
	defer rdb.Close()

	// Initialize RabbitMQ
	rmq, err := queue.Connect(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("RabbitMQ connection failed: %v", err)
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
		log.Println("Shutting down worker...")
		cancel()
	}()

	if err := consumer.Start(ctx); err != nil {
		log.Fatalf("Worker error: %v", err)
	}
	log.Println("Worker stopped")
}
