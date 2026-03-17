package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	APIPort              string
	Env                  string
	DatabaseURL          string
	RedisURL             string
	RabbitMQURL          string
	JWTSecret            string
	OTPTtl               time.Duration
	ManagedBotServiceURL string
	ServiceToken         string
	LogLevel             string
	CORSAllowedOrigins   []string
}

func Load() *Config {
	return &Config{
		APIPort:     getEnv("API_PORT", "8080"),
		Env:         getEnv("ENV", "development"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://alter:alter@localhost:5432/alter?sslmode=disable"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379/0"),
		RabbitMQURL: getEnv("RABBITMQ_URL", "amqp://alter:alter@localhost:5672/"),
		JWTSecret:   getEnv("JWT_SECRET", "dev-secret-key-do-not-use-in-production"),
		OTPTtl:               time.Duration(getEnvInt("OTP_TTL_SECONDS", 300)) * time.Second,
		ManagedBotServiceURL: getEnv("MANAGED_BOT_SERVICE_URL", "http://localhost:8081"),
		ServiceToken:         getEnv("SERVICE_TOKEN", ""),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
		CORSAllowedOrigins:   getEnvStringSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
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

func getEnvStringSlice(key string, fallback []string) []string {
	if v := os.Getenv(key); v != "" {
		return strings.Split(v, ",")
	}
	return fallback
}
