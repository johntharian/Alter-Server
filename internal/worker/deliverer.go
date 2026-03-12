package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	redisclient "github.com/john/botsapp/internal/redis"
)

type QueueMessage struct {
	MessageID  int64           `json:"message_id"`
	ThreadID   int64           `json:"thread_id"`
	FromUserID int64           `json:"from_user_id"`
	ToUserID   int64           `json:"to_user_id"`
	FromPhone  string          `json:"from_phone"`
	ToPhone    string          `json:"to_phone"`
	Intent     string          `json:"intent"`
	Payload    json.RawMessage `json:"payload"`
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
		log.Printf("[Deliver] Failed to parse message: %v", err)
		return true, err // Ack bad messages (don't retry)
	}

	log.Printf("[Deliver] Processing message %d from %s to %s", msg.MessageID, msg.FromPhone, msg.ToPhone)

	// Get current retry count
	var retryCount int
	err = d.db.QueryRow(ctx,
		`SELECT retry_count FROM messages WHERE id = $1`, msg.MessageID,
	).Scan(&retryCount)
	if err != nil {
		log.Printf("[Deliver] Message %d not found in DB: %v", msg.MessageID, err)
		return true, err
	}

	// Look up recipient's bot URL (Redis cache → DB fallback)
	botURL, secretKey, err := d.getBotEndpoint(ctx, msg.ToUserID)
	if err != nil {
		log.Printf("[Deliver] No bot endpoint for user %d: %v", msg.ToUserID, err)
		d.updateStatus(ctx, msg, "failed")
		return true, err
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
	req.Header.Set("X-Botsapp-Signature", signature)
	req.Header.Set("X-Botsapp-Message-ID", strconv.FormatInt(msg.MessageID, 10))

	resp, err := d.client.Do(req)
	if err != nil {
		return d.handleFailure(ctx, msg, retryCount, fmt.Errorf("http call: %w", err))
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[Deliver] Message %d delivered (status %d)", msg.MessageID, resp.StatusCode)
		d.updateStatus(ctx, msg, "delivered")
		return true, nil
	}

	return d.handleFailure(ctx, msg, retryCount,
		fmt.Errorf("bot returned status %d", resp.StatusCode))
}

func (d *Deliverer) handleFailure(ctx context.Context, msg QueueMessage, retryCount int, deliveryErr error) (bool, error) {
	retryCount++
	log.Printf("[Deliver] Message %d attempt %d failed: %v", msg.MessageID, retryCount, deliveryErr)

	// Update retry count
	_, _ = d.db.Exec(ctx,
		`UPDATE messages SET retry_count = $1 WHERE id = $2`,
		retryCount, msg.MessageID,
	)

	if retryCount >= maxRetries {
		log.Printf("[Deliver] Message %d exhausted retries, marking failed", msg.MessageID)
		d.updateStatus(ctx, msg, "failed")
		return false, deliveryErr // Nack → goes to DLQ via RabbitMQ DLX
	}

	// Calculate backoff delay
	delay := time.Duration(math.Pow(2, float64(retryCount))) * time.Second
	if delay > 60*time.Second {
		delay = 60 * time.Second
	}
	log.Printf("[Deliver] Message %d will retry in %s", msg.MessageID, delay)
	time.Sleep(delay)

	return false, deliveryErr // Nack for requeue
}

func (d *Deliverer) updateStatus(ctx context.Context, msg QueueMessage, status string) {
	_, err := d.db.Exec(ctx,
		`UPDATE messages SET status = $1 WHERE id = $2`,
		status, msg.MessageID,
	)
	if err != nil {
		log.Printf("[Deliver] Failed to update status for %d: %v", msg.MessageID, err)
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
