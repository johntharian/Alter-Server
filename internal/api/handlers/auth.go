package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/alter/internal/api/dto"
	"github.com/john/alter/internal/auth"
)

type AuthHandler struct {
	db  *pgxpool.Pool
	otp *auth.OTPService
	jwt *auth.JWTService
}

func NewAuthHandler(db *pgxpool.Pool, otp *auth.OTPService, jwt *auth.JWTService) *AuthHandler {
	return &AuthHandler{db: db, otp: otp, jwt: jwt}
}

func (h *AuthHandler) RequestOTP(w http.ResponseWriter, r *http.Request) {
	var req dto.OTPRequestReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "invalid request body"})
		return
	}

	if req.PhoneNumber == "" {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "phone_number is required"})
		return
	}

	_, err := h.otp.GenerateAndStore(r.Context(), req.PhoneNumber)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to generate OTP"})
		return
	}

	writeJSON(w, http.StatusOK, dto.OTPRequestRes{Message: "OTP sent successfully"})
}

func (h *AuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req dto.OTPVerifyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "invalid request body"})
		return
	}

	if req.PhoneNumber == "" || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "phone_number and code are required"})
		return
	}

	valid, err := h.otp.Verify(r.Context(), req.PhoneNumber, req.Code)
	if err != nil || !valid {
		writeJSON(w, http.StatusUnauthorized, dto.ErrorRes{Error: "invalid or expired OTP"})
		return
	}

	// Upsert user
	var userID int64
	var displayName string
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO users (phone_number) VALUES ($1)
		 ON CONFLICT (phone_number) DO UPDATE SET updated_at = now()
		 RETURNING id, display_name`, req.PhoneNumber,
	).Scan(&userID, &displayName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to create user"})
		return
	}

	token, err := h.jwt.GenerateToken(userID, req.PhoneNumber)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to generate token"})
		return
	}

	writeJSON(w, http.StatusOK, dto.OTPVerifyRes{
		Token: token,
		User: dto.UserInfo{
			ID:          userID,
			PhoneNumber: req.PhoneNumber,
			DisplayName: displayName,
		},
	})
}
