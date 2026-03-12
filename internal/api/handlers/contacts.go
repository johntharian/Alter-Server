package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/botsapp/internal/api/dto"
	"github.com/john/botsapp/internal/auth"
)

type ContactsHandler struct {
	db *pgxpool.Pool
}

func NewContactsHandler(db *pgxpool.Pool) *ContactsHandler {
	return &ContactsHandler{db: db}
}

// Sync takes a list of phone numbers, finds which ones are on the platform,
// and adds them as contacts for the current user.
func (h *ContactsHandler) Sync(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	var req dto.ContactSyncReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "invalid request body"})
		return
	}

	if len(req.PhoneNumbers) == 0 {
		writeJSON(w, http.StatusBadRequest, dto.ErrorRes{Error: "phone_numbers is required"})
		return
	}

	// Find users matching the phone numbers (excluding self)
	rows, err := h.db.Query(r.Context(),
		`SELECT id, phone_number, display_name FROM users
		 WHERE phone_number = ANY($1) AND id != $2`,
		req.PhoneNumbers, claims.UserID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to query contacts"})
		return
	}
	defer rows.Close()

	var found []dto.ContactInfo
	for rows.Next() {
		var c dto.ContactInfo
		if err := rows.Scan(&c.UserID, &c.PhoneNumber, &c.DisplayName); err != nil {
			continue
		}
		found = append(found, c)

		// Insert contact relationship (ignore duplicates)
		_, _ = h.db.Exec(r.Context(),
			`INSERT INTO contacts (user_id, contact_user_id)
			 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			claims.UserID, c.UserID,
		)
	}

	if found == nil {
		found = []dto.ContactInfo{}
	}

	writeJSON(w, http.StatusOK, dto.ContactSyncRes{Found: found})
}

// List returns all contacts for the current user.
func (h *ContactsHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	rows, err := h.db.Query(r.Context(),
		`SELECT u.id, u.phone_number, u.display_name
		 FROM contacts c
		 JOIN users u ON u.id = c.contact_user_id
		 WHERE c.user_id = $1
		 ORDER BY u.display_name`,
		claims.UserID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.ErrorRes{Error: "failed to list contacts"})
		return
	}
	defer rows.Close()

	contacts := []dto.ContactInfo{}
	for rows.Next() {
		var c dto.ContactInfo
		if err := rows.Scan(&c.UserID, &c.PhoneNumber, &c.DisplayName); err != nil {
			continue
		}
		contacts = append(contacts, c)
	}

	writeJSON(w, http.StatusOK, contacts)
}
