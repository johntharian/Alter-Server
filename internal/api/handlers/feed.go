package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"

	"github.com/john/botsapp/internal/auth"
	"github.com/john/botsapp/internal/logger"
	redisclient "github.com/john/botsapp/internal/redis"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for MVP
	},
}

type FeedHandler struct {
	redis *redisclient.Client
}

func NewFeedHandler(redis *redisclient.Client) *FeedHandler {
	return &FeedHandler{redis: redis}
}

// ServeWS upgrades to WebSocket and streams real-time feed events via Redis Pub/Sub.
func (h *FeedHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("WebSocket upgrade failed", map[string]interface{}{"error": err.Error()})
		return
	}
	defer conn.Close()

	userID := claims.UserID
	channel := "user:" + strconv.FormatInt(userID, 10) + ":feed"

	logger.Info("User connected to feed", map[string]interface{}{"user_id": userID})

	// Subscribe to user's feed channel
	ctx := r.Context()
	pubsub := h.redis.Subscribe(ctx, channel)
	defer pubsub.Close()

	ch := pubsub.Channel()

	// Read from WebSocket (just to detect close)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Forward Redis messages to WebSocket
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// Parse and forward the event
			var event json.RawMessage
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				// Forward raw string
				conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload))
				continue
			}
			conn.WriteJSON(event)
		case <-done:
			logger.Info("User disconnected from feed", map[string]interface{}{"user_id": userID})
			return
		case <-ctx.Done():
			return
		}
	}
}
