package models

import (
	"encoding/json"
	"time"
)

type User struct {
	ID              int64     `json:"id,string" db:"id"`
	PhoneNumber     string    `json:"phone_number" db:"phone_number"`
	DisplayName     string    `json:"display_name" db:"display_name"`
	IsManagedBot    bool      `json:"is_managed_bot" db:"is_managed_bot"`
	ManagedBotURL   *string   `json:"managed_bot_url,omitempty" db:"managed_bot_url"`
	ManagedBotSecret *string  `json:"-" db:"managed_bot_secret"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

type BotEndpoint struct {
	UserID    int64     `json:"user_id,string" db:"user_id"`
	URL       string    `json:"url" db:"url"`
	SecretKey string    `json:"-" db:"secret_key"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type Contact struct {
	UserID        int64     `json:"user_id,string" db:"user_id"`
	ContactUserID int64     `json:"contact_user_id,string" db:"contact_user_id"`
	AddedAt       time.Time `json:"added_at" db:"added_at"`
}

type Thread struct {
	ID              int64     `json:"id,string" db:"id"`
	ParticipantA    int64     `json:"participant_a,string" db:"participant_a"`
	ParticipantB    int64     `json:"participant_b,string" db:"participant_b"`
	HumanTakeoverBy *int64    `json:"human_takeover_by,string,omitempty" db:"human_takeover_by"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

type MessageStatus string

const (
	StatusQueued          MessageStatus = "queued"
	StatusDelivered       MessageStatus = "delivered"
	StatusClientDelivered MessageStatus = "client_delivered"
	StatusProcessed       MessageStatus = "processed"
	StatusFailed          MessageStatus = "failed"
)

type Message struct {
	ID            int64         `json:"id,string" db:"id"`
	ThreadID      int64         `json:"thread_id,string" db:"thread_id"`
	FromUserID    int64         `json:"from_user_id,string" db:"from_user_id"`
	ToUserID      int64         `json:"to_user_id,string" db:"to_user_id"`
	Intent        string        `json:"intent" db:"intent"`
	Payload       []byte        `json:"payload" db:"payload"`
	Status        MessageStatus `json:"status" db:"status"`
	HumanOverride bool          `json:"human_override" db:"human_override"`
	RetryCount    int           `json:"retry_count" db:"retry_count"`
	CreatedAt     time.Time     `json:"created_at" db:"created_at"`
}

// MessageEnvelope is the standard format for bot-to-bot messages.
type MessageEnvelope struct {
	From      string          `json:"from"`
	To        string          `json:"to"`
	Intent    string          `json:"intent"`
	ThreadID  int64           `json:"thread_id,string"`
	MessageID int64           `json:"message_id,string"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}
