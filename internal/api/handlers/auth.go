package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/alter/internal/api/dto"
	"github.com/john/alter/internal/auth"
)

type AuthHandler struct {
	db       *pgxpool.Pool
	firebase *auth.FirebaseService
	jwt      *auth.JWTService
}

func NewAuthHandler(db *pgxpool.Pool, firebase *auth.FirebaseService, jwt *auth.JWTService) *AuthHandler {
	return &AuthHandler{db: db, firebase: firebase, jwt: jwt}
}

// FirebaseVerify handles POST /auth/firebase/verify.
// The client completes Firebase Phone Auth and sends the resulting ID token here.
// We verify it, upsert the user, and return our own app JWT.
func (h *AuthHandler) FirebaseVerify(w http.ResponseWriter, r *http.Request) {
	var req dto.FirebaseVerifyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "invalid request body"})
		return
	}
	if req.IDToken == "" {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "id_token is required"})
		return
	}

	token, err := h.firebase.VerifyIDToken(r.Context(), req.IDToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, dto.ErrorRes{Error: "invalid or expired firebase token"})
		return
	}

	phoneNumber, _ := token.Claims["phone_number"].(string)
	if phoneNumber == "" {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "token does not contain phone_number"})
		return
	}

	var userID int64
	var displayName string
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO users (phone_number) VALUES ($1)
		 ON CONFLICT (phone_number) DO UPDATE SET updated_at = now()
		 RETURNING id, display_name`, phoneNumber,
	).Scan(&userID, &displayName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to create user"})
		return
	}

	appToken, err := h.jwt.GenerateToken(userID, phoneNumber)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to generate token"})
		return
	}

	writeJSON(w, http.StatusOK, dto.OTPVerifyRes{
		Token: appToken,
		User: dto.UserInfo{
			ID:          userID,
			PhoneNumber: phoneNumber,
			DisplayName: displayName,
		},
	})
}
