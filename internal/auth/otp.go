package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"time"

	redisclient "github.com/john/botsapp/internal/redis"
)

const otpKeyPrefix = "otp:"

type OTPService struct {
	redis *redisclient.Client
	ttl   time.Duration
}

func NewOTPService(redis *redisclient.Client, ttl time.Duration) *OTPService {
	return &OTPService{redis: redis, ttl: ttl}
}

// GenerateAndStore creates a 6-digit OTP, stores it in Redis, and returns it.
// In production, this would send an SMS. For MVP, the code is logged.
func (s *OTPService) GenerateAndStore(ctx context.Context, phoneNumber string) (string, error) {
	code, err := generateOTP()
	if err != nil {
		return "", fmt.Errorf("generate otp: %w", err)
	}

	key := otpKeyPrefix + phoneNumber
	if err := s.redis.Set(ctx, key, code, s.ttl); err != nil {
		return "", fmt.Errorf("store otp: %w", err)
	}

	log.Printf("[OTP] Code for %s: %s", phoneNumber, code)
	return code, nil
}

// Verify checks the OTP code against what's stored in Redis.
func (s *OTPService) Verify(ctx context.Context, phoneNumber, code string) (bool, error) {
	key := otpKeyPrefix + phoneNumber
	stored, err := s.redis.Get(ctx, key)
	if err != nil {
		return false, nil // Key expired or doesn't exist
	}

	if stored != code {
		return false, nil
	}

	// Delete OTP after successful verification
	_ = s.redis.Del(ctx, key)
	return true, nil
}

func generateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
