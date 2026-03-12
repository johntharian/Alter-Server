package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/botsapp/internal/api/handlers"
	"github.com/john/botsapp/internal/auth"
	"github.com/john/botsapp/internal/queue"
	redisclient "github.com/john/botsapp/internal/redis"
)

func NewRouter(
	db *pgxpool.Pool,
	rdb *redisclient.Client,
	rmq *queue.RabbitMQ,
	jwtSvc *auth.JWTService,
	otpSvc *auth.OTPService,
) chi.Router {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Handlers
	authH := handlers.NewAuthHandler(db, otpSvc, jwtSvc)
	userH := handlers.NewUserHandler(db, rdb)
	contactsH := handlers.NewContactsHandler(db)
	messagesH := handlers.NewMessagesHandler(db, rmq, rdb)
	threadsH := handlers.NewThreadsHandler(db, rdb)
	feedH := handlers.NewFeedHandler(rdb)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Public routes
	r.Post("/auth/otp/request", authH.RequestOTP)
	r.Post("/auth/otp/verify", authH.VerifyOTP)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(jwtSvc.Middleware)

		// User
		r.Get("/users/me", userH.GetMe)
		r.Put("/users/me", userH.UpdateMe)
		r.Get("/users/me/bot", userH.GetBot)
		r.Put("/users/me/bot", userH.UpdateBot)

		// Contacts
		r.Post("/contacts/sync", contactsH.Sync)
		r.Get("/contacts", contactsH.List)

		// Messages
		r.Post("/messages", messagesH.Send)

		// Threads
		r.Get("/threads", threadsH.List)
		r.Get("/threads/{id}/messages", threadsH.GetMessages)
		r.Post("/threads/{id}/takeover", threadsH.Takeover)
		r.Delete("/threads/{id}/takeover", threadsH.ReleaseTakeover)

		// WebSocket feed
		r.Get("/ws/feed", feedH.ServeWS)
	})

	return r
}
