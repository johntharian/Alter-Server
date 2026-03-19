package dto

import "encoding/json"

// Auth

type OTPRequestReq struct {
	PhoneNumber string `json:"phone_number"`
}

type OTPRequestRes struct {
	Message string `json:"message"`
}

type OTPVerifyReq struct {
	PhoneNumber string `json:"phone_number"`
	Code        string `json:"code"`
}

type OTPVerifyRes struct {
	Token string   `json:"token"`
	User  UserInfo `json:"user"`
}

// User

type UserInfo struct {
	ID          int64  `json:"id,string"`
	PhoneNumber string `json:"phone_number"`
	DisplayName string `json:"display_name"`
}

type UpdateUserReq struct {
	DisplayName string `json:"display_name"`
}

// Bot Endpoint

type BotEndpointReq struct {
	URL string `json:"url"`
}

type BotEndpointRes struct {
	UserID   int64  `json:"user_id,string"`
	URL      string `json:"url"`
	IsActive bool   `json:"is_active"`
}

// Contacts

type ContactSyncReq struct {
	PhoneNumbers []string `json:"phone_numbers"`
}

type ContactInfo struct {
	UserID      int64  `json:"user_id,string"`
	PhoneNumber string `json:"phone_number"`
	DisplayName string `json:"display_name"`
}

type ContactSyncRes struct {
	Found []ContactInfo `json:"found"`
}

// Messages

type SendMessageReq struct {
	To      string          `json:"to"`
	Intent  string          `json:"intent"`
	Payload json.RawMessage `json:"payload"`
}

type SendMessageRes struct {
	MessageID int64  `json:"message_id,string"`
	ThreadID  int64  `json:"thread_id,string"`
	Status    string `json:"status"`
}

// Threads

type ThreadInfo struct {
	ID                int64   `json:"id,string"`
	ParticipantA      int64   `json:"participant_a,string"`
	ParticipantB      int64   `json:"participant_b,string"`
	ParticipantAName  string  `json:"participant_a_name"`
	ParticipantAPhone string  `json:"participant_a_phone"`
	ParticipantBName  string  `json:"participant_b_name"`
	ParticipantBPhone string  `json:"participant_b_phone"`
	HumanTakeoverBy   *int64  `json:"human_takeover_by,string,omitempty"`
	LastMessage       *string `json:"last_message,omitempty"`
	CreatedAt         string  `json:"created_at"`
}

type MessageInfo struct {
	ID            int64           `json:"id,string"`
	ThreadID      int64           `json:"thread_id,string"`
	FromUserID    int64           `json:"from_user_id,string"`
	ToUserID      int64           `json:"to_user_id,string"`
	Intent        string          `json:"intent"`
	Payload       json.RawMessage `json:"payload"`
	Status        string          `json:"status"`
	HumanOverride bool            `json:"human_override"`
	CreatedAt     string          `json:"created_at"`
}

// Feed events (WebSocket)

type FeedEvent struct {
	Type string      `json:"type"` // new_message, status_update, takeover_started, takeover_ended
	Data interface{} `json:"data"`
}

type StatusUpdateEvent struct {
	MessageID int64  `json:"message_id,string"`
	ThreadID  int64  `json:"thread_id,string"`
	Status    string `json:"status"`
}

// Managed Bot

type ProvisionManagedBotRes struct {
	BotURL string `json:"bot_url"`
}

type InternalMessageInfo struct {
	ID          int64           `json:"id,string"`
	SenderID    int64           `json:"sender_id,string"`
	RecipientID int64           `json:"recipient_id,string"`
	Content     json.RawMessage `json:"content"`
	CreatedAt   string          `json:"created_at"`
}

// Generic error

type ErrorRes struct {
	Error string `json:"error"`
}
