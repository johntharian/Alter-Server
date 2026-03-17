package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/alter/internal/api/dto"
	"github.com/john/alter/internal/auth"
	"github.com/john/alter/internal/logger"
	"github.com/john/alter/internal/queue"
	redisclient "github.com/john/alter/internal/redis"
)

type MessagesHandler struct {
	db    *pgxpool.Pool
	rmq   *queue.RabbitMQ
	redis *redisclient.Client
}

func NewMessagesHandler(db *pgxpool.Pool, rmq *queue.RabbitMQ, redis *redisclient.Client) *MessagesHandler {
	return &MessagesHandler{db: db, rmq: rmq, redis: redis}
}

// Send enqueues a message for delivery. Never blocks waiting for bot response.
func (h *MessagesHandler) Send(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	var req dto.SendMessageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "invalid request body"})
		return
	}

	if req.To == "" {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "to (phone number) is required"})
		return
	}

	// Look up recipient by phone number
	var toUserID int64
	err := h.db.QueryRow(r.Context(),
		`SELECT id FROM users WHERE phone_number = $1`, req.To,
	).Scan(&toUserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, dto.ErrorRes{Error: "recipient not found on the network"})
		return
	}

	// Find or create thread (normalize: lower UUID = participant_a)
	fromID := claims.UserID
	partA, partB := fromID, toUserID
	if partA > partB {
		partA, partB = partB, partA
	}

	var threadID int64
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO threads (participant_a, participant_b)
		 VALUES ($1, $2)
		 ON CONFLICT (participant_a, participant_b) DO UPDATE SET created_at = threads.created_at
		 RETURNING id`,
		partA, partB,
	).Scan(&threadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to create thread"})
		return
	}

	// Check if thread is in human takeover mode by someone else
	var takeoverBy *int64
	err = h.db.QueryRow(r.Context(),
		`SELECT human_takeover_by FROM threads WHERE id = $1`, threadID,
	).Scan(&takeoverBy)
	if err != nil && err.Error() != "no rows in result set" {
		logger.Error("Failed to check human_takeover_by", map[string]interface{}{"thread_id": threadID, "error": err.Error()})
	}

	humanOverride := false
	if takeoverBy != nil && *takeoverBy == fromID {
		humanOverride = true
	}

	// Insert message with status "queued"
	payload := req.Payload
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}

	var messageID int64
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO messages (thread_id, from_user_id, to_user_id, intent, payload, human_override)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		threadID, fromID, toUserID, req.Intent, payload, humanOverride,
	).Scan(&messageID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to create message"})
		return
	}

	// Get sender's phone number for the envelope
	var fromPhone string
	err = h.db.QueryRow(r.Context(),
		`SELECT phone_number FROM users WHERE id = $1`, fromID,
	).Scan(&fromPhone)
	if err != nil {
		logger.Error("Failed to get sender phone number", map[string]interface{}{"user_id": fromID, "error": err.Error()})
	}

	// Enqueue for delivery
	queueMsg := map[string]interface{}{
		"message_id":     messageID,
		"thread_id":      threadID,
		"from_user_id":   fromID,
		"to_user_id":     toUserID,
		"from_phone":     fromPhone,
		"to_phone":       req.To,
		"intent":         req.Intent,
		"payload":        payload,
		"human_override": humanOverride,
	}
	body, _ := json.Marshal(queueMsg)

	if err := h.rmq.Publish(r.Context(), body); err != nil {
		logger.Error("Failed to enqueue message", map[string]interface{}{"message_id": messageID, "error": err.Error()})
		// Update status to failed
		if _, dbErr := h.db.Exec(r.Context(),
			`UPDATE messages SET status = 'failed' WHERE id = $1`, messageID); dbErr != nil {
			logger.Error("Failed to update message status to failed", map[string]interface{}{"message_id": messageID, "error": dbErr.Error()})
		}
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to enqueue message"})
		return
	}

	// Publish new_message event to real-time feeds
	event := dto.FeedEvent{
		Type: "new_message",
		Data: dto.MessageInfo{
			ID:            messageID,
			ThreadID:      threadID,
			FromUserID:    fromID,
			ToUserID:      toUserID,
			Intent:        req.Intent,
			Payload:       payload,
			Status:        "queued",
			HumanOverride: humanOverride,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		},
	}
	eventJSON, _ := json.Marshal(event)
	
	importStrconv := strconv.FormatInt
	
	if err := h.redis.Publish(r.Context(), "user:"+importStrconv(fromID, 10)+":feed", string(eventJSON)); err != nil {
		logger.Error("Failed to publish feed event", map[string]interface{}{"user_id": fromID, "error": err.Error()})
	}
	if err := h.redis.Publish(r.Context(), "user:"+importStrconv(toUserID, 10)+":feed", string(eventJSON)); err != nil {
		logger.Error("Failed to publish feed event", map[string]interface{}{"user_id": toUserID, "error": err.Error()})
	}

	// Return 202 Accepted — message is queued, not yet delivered
	writeJSON(w, http.StatusAccepted, dto.SendMessageRes{
		MessageID: messageID,
		ThreadID:  threadID,
		Status:    "queued",
	})
}
