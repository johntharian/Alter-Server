package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/alter/internal/logger"
	redisclient "github.com/john/alter/internal/redis"
)

type QueueMessage struct {
	MessageID     int64           `json:"message_id"`
	ThreadID      int64           `json:"thread_id"`
	FromUserID    int64           `json:"from_user_id"`
	ToUserID      int64           `json:"to_user_id"`
	FromPhone     string          `json:"from_phone"`
	ToPhone       string          `json:"to_phone"`
	Intent        string          `json:"intent"`
	Payload       json.RawMessage `json:"payload"`
	HumanOverride bool            `json:"human_override"`
}

type Deliverer struct {
	db     *pgxpool.Pool
	redis  *redisclient.Client
	client *http.Client
}

func NewDeliverer(db *pgxpool.Pool, redis *redisclient.Client) *Deliverer {
	return &Deliverer{
		db:    db,
		redis: redis,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

const maxRetries = 5

// Deliver attempts to deliver a message to the recipient's bot URL.
// Returns true if the message was handled (delivered or sent to DLQ).
func (d *Deliverer) Deliver(ctx context.Context, body []byte) (shouldAck bool, err error) {
	var msg QueueMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		logger.Error("Failed to parse message", map[string]interface{}{"error": err.Error(), "body": string(body)})
		return true, err // Ack bad messages (don't retry)
	}

	logger.Info("Processing message", map[string]interface{}{
		"message_id": msg.MessageID,
		"from_phone": msg.FromPhone,
		"to_phone":   msg.ToPhone,
	})

	// Get current retry count
	var retryCount int
	err = d.db.QueryRow(ctx,
		`SELECT retry_count FROM messages WHERE id = $1`, msg.MessageID,
	).Scan(&retryCount)
	if err != nil {
		logger.Error("Message not found in DB", map[string]interface{}{"message_id": msg.MessageID, "error": err.Error()})
		return true, err
	}

	// If the thread is in human takeover or recipient is a regular user, skip bot delivery.
	// The new_message WS event was already published at send time — the message is in the DB
	// and visible to the client. Just mark it client_delivered and ack.
	botURL, secretKey, isManagedBot, isRegularUser, err := d.resolveBotEndpoint(ctx, msg.ToUserID)
	if err != nil {
		logger.Error("No bot endpoint", map[string]interface{}{"user_id": msg.ToUserID, "error": err.Error()})
		d.updateStatus(ctx, msg, "failed")
		return true, err
	}

	if isRegularUser || msg.HumanOverride {
		reason := "regular user recipient"
		if msg.HumanOverride {
			reason = "human takeover active"
		}
		logger.Info("Skipping bot delivery", map[string]interface{}{
			"message_id": msg.MessageID,
			"reason":     reason,
		})
		d.updateStatus(ctx, msg, "client_delivered")
		return true, nil
	}

	// Build the message envelope
	envelope := map[string]interface{}{
		"from":       msg.FromPhone,
		"to":         msg.ToPhone,
		"intent":     msg.Intent,
		"thread_id":  msg.ThreadID,
		"message_id": msg.MessageID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"payload":    msg.Payload,
	}
	envelopeBytes, _ := json.Marshal(envelope)

	// Compute HMAC signature
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write(envelopeBytes)
	signature := hex.EncodeToString(mac.Sum(nil))

	// POST to bot URL
	req, err := http.NewRequestWithContext(ctx, "POST", botURL, bytes.NewReader(envelopeBytes))
	if err != nil {
		return d.handleFailure(ctx, msg, retryCount, fmt.Errorf("create request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	if isManagedBot {
		req.Header.Set("X-Hub-Signature-256", "sha256="+signature)
	}
	req.Header.Set("X-Alter-Signature", signature)
	req.Header.Set("X-Alter-Message-ID", strconv.FormatInt(msg.MessageID, 10))

	logger.Info("Attempting delivery", map[string]interface{}{
		"message_id": msg.MessageID,
		"bot_url":    botURL,
	})

	start := time.Now()
	resp, err := d.client.Do(req)
	if err != nil {
		return d.handleFailure(ctx, msg, retryCount, fmt.Errorf("http call: %w", err))
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	duration := time.Since(start).Milliseconds()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		io.Copy(io.Discard, resp.Body)
		logger.Info("Delivery complete", map[string]interface{}{
			"message_id":  msg.MessageID,
			"bot_url":     botURL,
			"status_code": resp.StatusCode,
			"duration_ms": duration,
		})
		d.updateStatus(ctx, msg, "delivered")
		return true, nil
	}

	// Read response body for error detail on non-2xx
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	logger.Error("Bot delivery failed", map[string]interface{}{
		"message_id":    msg.MessageID,
		"bot_url":       botURL,
		"status_code":   resp.StatusCode,
		"duration_ms":   duration,
		"response_body": string(respBody),
	})

	return d.handleFailure(ctx, msg, retryCount,
		fmt.Errorf("bot returned status %d", resp.StatusCode))
}

func (d *Deliverer) handleFailure(ctx context.Context, msg QueueMessage, retryCount int, deliveryErr error) (bool, error) {
	retryCount++
	logger.Warn("Message attempt failed", map[string]interface{}{
		"message_id": msg.MessageID,
		"attempt":    retryCount,
		"error":      deliveryErr.Error(),
	})

	// Update retry count
	_, _ = d.db.Exec(ctx,
		`UPDATE messages SET retry_count = $1 WHERE id = $2`,
		retryCount, msg.MessageID,
	)

	if retryCount >= maxRetries {
		logger.Error("Message exhausted retries, marking failed", map[string]interface{}{"message_id": msg.MessageID})
		d.updateStatus(ctx, msg, "failed")
		return false, deliveryErr // Nack → goes to DLQ via RabbitMQ DLX
	}

	// Calculate backoff delay
	delay := time.Duration(math.Pow(2, float64(retryCount))) * time.Second
	if delay > 60*time.Second {
		delay = 60 * time.Second
	}
	logger.Info("Scheduling message retry", map[string]interface{}{
		"message_id": msg.MessageID,
		"delay":      delay.String(),
	})
	time.Sleep(delay)

	return false, deliveryErr // Nack for requeue
}

func (d *Deliverer) updateStatus(ctx context.Context, msg QueueMessage, status string) {
	_, err := d.db.Exec(ctx,
		`UPDATE messages SET status = $1 WHERE id = $2`,
		status, msg.MessageID,
	)
	if err != nil {
		logger.Error("Failed to update status", map[string]interface{}{
			"message_id": msg.MessageID,
			"status":     status,
			"error":      err.Error(),
		})
	}

	// Publish status update via Redis Pub/Sub to both participants
	event := map[string]interface{}{
		"type": "status_update",
		"data": map[string]string{
			"message_id": strconv.FormatInt(msg.MessageID, 10),
			"thread_id":  strconv.FormatInt(msg.ThreadID, 10),
			"status":     status,
		},
	}
	eventJSON, _ := json.Marshal(event)

	_ = d.redis.Publish(ctx, "user:"+strconv.FormatInt(msg.FromUserID, 10)+":feed", string(eventJSON))
	_ = d.redis.Publish(ctx, "user:"+strconv.FormatInt(msg.ToUserID, 10)+":feed", string(eventJSON))
}

// managedBotCache holds cached managed bot connection details.
type managedBotCache struct {
	IsManaged bool   `json:"is_managed"`
	URL       string `json:"url,omitempty"`
	Secret    string `json:"secret,omitempty"`
}

// resolveBotEndpoint checks if the user is a managed bot first, then falls back to bot_endpoints.
// Returns isRegularUser=true (with no error) when the recipient has no bot configured at all.
func (d *Deliverer) resolveBotEndpoint(ctx context.Context, userID int64) (url, secretKey string, isManagedBot bool, isRegularUser bool, err error) {
	cacheKey := "managed_bot_config:" + strconv.FormatInt(userID, 10)
	cached, err := d.redis.Get(ctx, cacheKey)
	if err == nil && cached != "" {
		var cache managedBotCache
		if err := json.Unmarshal([]byte(cached), &cache); err == nil {
			if cache.IsManaged {
				return cache.URL, cache.Secret, true, false, nil
			}
			urlRes, secretRes, errRes := d.getBotEndpoint(ctx, userID)
			if errors.Is(errRes, pgx.ErrNoRows) {
				return "", "", false, true, nil
			}
			return urlRes, secretRes, false, false, errRes
		}
	}

	var isManaged bool
	var managedURL, managedSecret *string
	err = d.db.QueryRow(ctx,
		`SELECT is_managed_bot, managed_bot_url, managed_bot_secret FROM users WHERE id = $1`,
		userID,
	).Scan(&isManaged, &managedURL, &managedSecret)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", false, false, fmt.Errorf("user not found: %w", err)
		}
		return "", "", false, false, fmt.Errorf("query managed bot: %w", err)
	}

	cacheData := managedBotCache{
		IsManaged: isManaged,
	}
	if isManaged && managedURL != nil && managedSecret != nil {
		cacheData.URL = *managedURL
		cacheData.Secret = *managedSecret
	}

	if cacheBytes, err := json.Marshal(cacheData); err == nil {
		_ = d.redis.Set(ctx, cacheKey, string(cacheBytes), 5*time.Minute)
	}

	if cacheData.IsManaged {
		return cacheData.URL, cacheData.Secret, true, false, nil
	}

	urlRes, secretRes, errRes := d.getBotEndpoint(ctx, userID)
	if errors.Is(errRes, pgx.ErrNoRows) {
		return "", "", false, true, nil
	}
	return urlRes, secretRes, false, false, errRes
}

func (d *Deliverer) getBotEndpoint(ctx context.Context, userID int64) (url, secretKey string, err error) {
	// Try Redis cache first
	cacheKey := "bot:" + strconv.FormatInt(userID, 10)
	cached, err := d.redis.Get(ctx, cacheKey)
	if err == nil && cached != "" {
		// Cache hit — still need secret from DB
		err = d.db.QueryRow(ctx,
			`SELECT url, secret_key FROM bot_endpoints WHERE user_id = $1 AND is_active = true`,
			userID,
		).Scan(&url, &secretKey)
		if err == nil {
			return url, secretKey, nil
		}
	}

	// Cache miss — query DB
	err = d.db.QueryRow(ctx,
		`SELECT url, secret_key FROM bot_endpoints WHERE user_id = $1 AND is_active = true`,
		userID,
	).Scan(&url, &secretKey)
	if err != nil {
		return "", "", fmt.Errorf("bot endpoint not found: %w", err)
	}

	// Update cache
	_ = d.redis.Set(ctx, cacheKey, url, 5*time.Minute)

	return url, secretKey, nil
}
