package handlers

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/botsapp/internal/api/dto"
	"github.com/john/botsapp/internal/auth"
	"github.com/john/botsapp/internal/database"
	"github.com/john/botsapp/internal/logger"
)

type ManagedBotHandler struct {
	db                   *pgxpool.Pool
	managedBotServiceURL string
	client               *http.Client
}

func NewManagedBotHandler(db *pgxpool.Pool, managedBotServiceURL, serviceToken string) *ManagedBotHandler {
	return &ManagedBotHandler{
		db:                   db,
		managedBotServiceURL: managedBotServiceURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Provision calls the managed bot service to provision a bot for a user,
// then stores the returned bot_url and secret_key in the users table.
func (h *ManagedBotHandler) Provision(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, dto.ErrorRes{Error: "unauthorized"})
		return
	}

	var req dto.ProvisionManagedBotReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "invalid request body"})
		return
	}

	if req.UserID == 0 {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "user_id is required"})
		return
	}

	// Fetch the user's phone number to pass to the managed bot service
	var phoneNumber string
	if err := h.db.QueryRow(r.Context(),
		`SELECT phone_number FROM users WHERE id = $1`, req.UserID,
	).Scan(&phoneNumber); err != nil {
		logger.Error("Failed to fetch phone number", map[string]interface{}{"user_id": req.UserID, "error": err.Error()})
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to look up user"})
		return
	}

	// Call the managed bot service
	provisionBody, _ := json.Marshal(map[string]string{
		"user_id":      strconv.FormatInt(req.UserID, 10),
		"phone_number": phoneNumber,
	})

	provisionReq, err := http.NewRequestWithContext(r.Context(), "POST",
		h.managedBotServiceURL+"/provision",
		bytes.NewReader(provisionBody),
	)
	if err != nil {
		logger.Error("Failed to create provision request", map[string]interface{}{"error": err.Error()})
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to provision bot"})
		return
	}
	provisionReq.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(provisionReq)
	if err != nil {
		logger.Error("Failed to call bot service", map[string]interface{}{"error": err.Error()})
		writeJSON(w, http.StatusBadGateway, dto.ErrorRes{Error: "bot service unavailable"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("Bot service returned non-200 status", map[string]interface{}{"status_code": resp.StatusCode})
		writeJSON(w, http.StatusBadGateway, dto.ErrorRes{Error: fmt.Sprintf("bot service returned status %d", resp.StatusCode)})
		return
	}

	var provisionRes struct {
		BotURL    string `json:"bot_url"`
		SecretKey string `json:"secret_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&provisionRes); err != nil {
		logger.Error("Failed to decode bot service response", map[string]interface{}{"error": err.Error()})
		writeJSON(w, http.StatusBadGateway, dto.ErrorRes{Error: "invalid bot service response"})
		return
	}

	// Store in the users table
	if err := database.SetManagedBot(r.Context(), h.db, req.UserID, provisionRes.BotURL, provisionRes.SecretKey); err != nil {
		logger.Error("Failed to store bot config", map[string]interface{}{"user_id": req.UserID, "error": err.Error()})
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to store bot configuration"})
		return
	}

	logger.Info("Provisioned managed bot", map[string]interface{}{"user_id": req.UserID, "bot_url": provisionRes.BotURL})
	writeJSON(w, http.StatusOK, dto.ProvisionManagedBotRes{BotURL: provisionRes.BotURL})
}

// GetThreadMessages returns all messages for a thread. Protected by service token.
func (h *ManagedBotHandler) GetThreadMessages(w http.ResponseWriter, r *http.Request) {
	threadIDStr := chi.URLParam(r, "thread_id")
	threadID, err := strconv.ParseInt(threadIDStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "invalid thread_id"})
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, from_user_id, to_user_id, payload, created_at
		 FROM messages WHERE thread_id = $1 ORDER BY created_at ASC`,
		threadID,
	)
	if err != nil {
		logger.Error("Failed to query messages for thread", map[string]interface{}{"thread_id": threadID, "error": err.Error()})
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to fetch messages"})
		return
	}
	defer rows.Close()

	messages := []dto.InternalMessageInfo{}
	for rows.Next() {
		var m dto.InternalMessageInfo
		var createdAt time.Time
		if err := rows.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.Content, &createdAt); err != nil {
			logger.Error("Failed to scan message row", map[string]interface{}{"error": err.Error()})
			writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to read messages"})
			return
		}
		m.CreatedAt = createdAt.Format(time.RFC3339)
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		logger.Error("Row iteration error for thread", map[string]interface{}{"thread_id": threadID, "error": err.Error()})
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to read messages"})
		return
	}

	writeJSON(w, http.StatusOK, messages)
}

// ServiceTokenMiddleware validates the X-Service-Token header against the configured service token.
func ServiceTokenMiddleware(serviceToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-Service-Token")
			if serviceToken == "" || token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(serviceToken)) != 1 {
				writeJSON(w, http.StatusUnauthorized, dto.ErrorRes{Error: "invalid service token"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
