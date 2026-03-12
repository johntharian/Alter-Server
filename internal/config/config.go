package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	APIPort     string
	Env         string
	DatabaseURL string
	RedisURL    string
	RabbitMQURL string
	JWTSecret   string
	OTPTtl      time.Duration
}

func Load() *Config {
	return &Config{
		APIPort:     getEnv("API_PORT", "8080"),
		Env:         getEnv("ENV", "development"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://botsapp:botsapp@localhost:5432/botsapp?sslmode=disable"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379/0"),
		RabbitMQURL: getEnv("RABBITMQ_URL", "amqp://botsapp:botsapp@localhost:5672/"),
		JWTSecret:   getEnv("JWT_SECRET", "dev-secret-key-do-not-use-in-production"),
		OTPTtl:      time.Duration(getEnvInt("OTP_TTL_SECONDS", 300)) * time.Second,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
