package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/alter/internal/api/dto"
	"github.com/john/alter/internal/auth"
	redisclient "github.com/john/alter/internal/redis"
)

type ThreadsHandler struct {
	db    *pgxpool.Pool
	redis *redisclient.Client
}

func NewThreadsHandler(db *pgxpool.Pool, redis *redisclient.Client) *ThreadsHandler {
	return &ThreadsHandler{db: db, redis: redis}
}

// List returns all threads the current user is part of.
func (h *ThreadsHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	rows, err := h.db.Query(r.Context(),
		`SELECT id, participant_a, participant_b, human_takeover_by, created_at
		 FROM threads
		 WHERE participant_a = $1 OR participant_b = $1
		 ORDER BY created_at DESC`,
		claims.UserID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to list threads"})
		return
	}
	defer rows.Close()

	threads := []dto.ThreadInfo{}
	for rows.Next() {
		var t dto.ThreadInfo
		var createdAt time.Time
		if err := rows.Scan(&t.ID, &t.ParticipantA, &t.ParticipantB, &t.HumanTakeoverBy, &createdAt); err != nil {
			continue
		}
		t.CreatedAt = createdAt.Format(time.RFC3339)
		threads = append(threads, t)
	}

	writeJSON(w, http.StatusOK, threads)
}

// GetMessages returns messages in a thread.
func (h *ThreadsHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	threadID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	// Verify user is a participant
	var count int
	err := h.db.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM threads
		 WHERE id = $1 AND (participant_a = $2 OR participant_b = $2)`,
		threadID, claims.UserID,
	).Scan(&count)
	if err != nil || count == 0 {
		writeJSON(w, http.StatusNotFound, dto.ErrorRes{Error: "thread not found"})
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, thread_id, from_user_id, to_user_id, intent, payload, status, human_override, created_at
		 FROM messages
		 WHERE thread_id = $1
		 ORDER BY created_at ASC`,
		threadID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to list messages"})
		return
	}
	defer rows.Close()

	messages := []dto.MessageInfo{}
	for rows.Next() {
		var m dto.MessageInfo
		var createdAt time.Time
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.FromUserID, &m.ToUserID, &m.Intent, &m.Payload, &m.Status, &m.HumanOverride, &createdAt); err != nil {
			continue
		}
		m.CreatedAt = createdAt.Format(time.RFC3339)
		messages = append(messages, m)
	}

	writeJSON(w, http.StatusOK, messages)
}

// Takeover enters human-in-the-loop mode for a thread.
func (h *ThreadsHandler) Takeover(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	threadID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	result, err := h.db.Exec(r.Context(),
		`UPDATE threads SET human_takeover_by = $1
		 WHERE id = $2 AND (participant_a = $1 OR participant_b = $1)`,
		claims.UserID, threadID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to takeover thread"})
		return
	}
	if result.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, dto.ErrorRes{Error: "thread not found"})
		return
	}

	h.publishFeedEvent(r, claims.UserID, threadID, "takeover_started")

	writeJSON(w, http.StatusOK, map[string]string{
		"message":   "human takeover activated",
		"thread_id": strconv.FormatInt(threadID, 10),
	})
}

// ReleaseTakeover exits human-in-the-loop mode.
func (h *ThreadsHandler) ReleaseTakeover(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	threadID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	result, err := h.db.Exec(r.Context(),
		`UPDATE threads SET human_takeover_by = NULL
		 WHERE id = $1 AND human_takeover_by = $2`,
		threadID, claims.UserID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to release thread"})
		return
	}
	if result.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, dto.ErrorRes{Error: "thread not found or not taken over by you"})
		return
	}

	h.publishFeedEvent(r, claims.UserID, threadID, "takeover_ended")

	writeJSON(w, http.StatusOK, map[string]string{
		"message":   "thread released back to bot",
		"thread_id": strconv.FormatInt(threadID, 10),
	})
}

func (h *ThreadsHandler) publishFeedEvent(r *http.Request, userID, threadID int64, eventType string) {
	var partA, partB int64
	_ = h.db.QueryRow(r.Context(),
		`SELECT participant_a, participant_b FROM threads WHERE id = $1`, threadID,
	).Scan(&partA, &partB)

	event := dto.FeedEvent{
		Type: eventType,
		Data: map[string]string{"thread_id": strconv.FormatInt(threadID, 10), "user_id": strconv.FormatInt(userID, 10)},
	}
	eventJSON, _ := json.Marshal(event)

	_ = h.redis.Publish(r.Context(), "user:"+strconv.FormatInt(partA, 10)+":feed", string(eventJSON))
	_ = h.redis.Publish(r.Context(), "user:"+strconv.FormatInt(partB, 10)+":feed", string(eventJSON))
}
