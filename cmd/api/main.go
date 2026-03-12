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

	"github.com/john/botsapp/internal/api"
	"github.com/john/botsapp/internal/auth"
	"github.com/john/botsapp/internal/config"
	"github.com/john/botsapp/internal/database"
	"github.com/john/botsapp/internal/queue"
	redisclient "github.com/john/botsapp/internal/redis"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	log.Println("Starting BotsApp API server...")

	// Initialize PostgreSQL
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

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

	// Initialize auth services
	jwtSvc := auth.NewJWTService(cfg.JWTSecret)
	otpSvc := auth.NewOTPService(rdb, cfg.OTPTtl)

	// Create router
	router := api.NewRouter(db, rdb, rmq, jwtSvc, otpSvc)

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
		log.Println("Shutting down API server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	log.Printf("API server listening on :%s", cfg.APIPort)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Server stopped")
}
