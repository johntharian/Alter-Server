package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/botsapp/internal/api/dto"
	"github.com/john/botsapp/internal/auth"
	"github.com/john/botsapp/internal/logger"
	redisclient "github.com/john/botsapp/internal/redis"
)

type UserHandler struct {
	db    *pgxpool.Pool
	redis *redisclient.Client
}

func NewUserHandler(db *pgxpool.Pool, redis *redisclient.Client) *UserHandler {
	return &UserHandler{db: db, redis: redis}
}

func (h *UserHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	var user dto.UserInfo
	err := h.db.QueryRow(r.Context(),
		`SELECT id, phone_number, display_name FROM users WHERE id = $1`,
		claims.UserID,
	).Scan(&user.ID, &user.PhoneNumber, &user.DisplayName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, dto.ErrorRes{Error: "user not found"})
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (h *UserHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	var req dto.UpdateUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "invalid request body"})
		return
	}

	_, err := h.db.Exec(r.Context(),
		`UPDATE users SET display_name = $1, updated_at = now() WHERE id = $2`,
		req.DisplayName, claims.UserID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to update user"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *UserHandler) GetBot(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	var bot dto.BotEndpointRes
	err := h.db.QueryRow(r.Context(),
		`SELECT user_id, url, is_active FROM bot_endpoints WHERE user_id = $1`,
		claims.UserID,
	).Scan(&bot.UserID, &bot.URL, &bot.IsActive)
	if err != nil {
		writeJSON(w, http.StatusNotFound, dto.ErrorRes{Error: "no bot endpoint registered"})
		return
	}

	writeJSON(w, http.StatusOK, bot)
}

func (h *UserHandler) UpdateBot(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	var req dto.BotEndpointReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "invalid request body"})
		return
	}

	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "url is required"})
		return
	}

	// Generate a secret key for webhook signing
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to generate secret"})
		return
	}
	secret := hex.EncodeToString(secretBytes)

	_, err := h.db.Exec(r.Context(),
		`INSERT INTO bot_endpoints (user_id, url, secret_key)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id) DO UPDATE SET url = $2, secret_key = $3, updated_at = now()`,
		claims.UserID, req.URL, secret,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to register bot"})
		return
	}

	// Update Redis cache (convert ID to string)
	if err := h.redis.Set(r.Context(), "bot:"+strconv.FormatInt(claims.UserID, 10), req.URL, 0); err != nil {
		logger.Error("Failed to store bot endpoint in redis cache", map[string]interface{}{
			"user_id": claims.UserID,
			"error":   err.Error(),
		})
	}

	writeJSON(w, http.StatusOK, dto.BotEndpointRes{
		UserID:   claims.UserID,
		URL:      req.URL,
		IsActive: true,
	})
}
