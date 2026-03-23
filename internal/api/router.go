package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/alter/internal/api/handlers"
	"github.com/john/alter/internal/auth"
	"github.com/john/alter/internal/config"
	"github.com/john/alter/internal/queue"
	redisclient "github.com/john/alter/internal/redis"
)

func NewRouter(
	db *pgxpool.Pool,
	rdb *redisclient.Client,
	rmq *queue.RabbitMQ,
	jwtSvc *auth.JWTService,
	firebaseSvc *auth.FirebaseService,
	cfg *config.Config,
) chi.Router {
	r := chi.NewRouter()

	// Global middleware
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(RequestLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Handlers
	authH := handlers.NewAuthHandler(db, firebaseSvc, jwtSvc)
	userH := handlers.NewUserHandler(db, rdb)
	contactsH := handlers.NewContactsHandler(db)
	messagesH := handlers.NewMessagesHandler(db, rmq, rdb)
	threadsH := handlers.NewThreadsHandler(db, rdb)
	feedH := handlers.NewFeedHandler(rdb)
	managedBotH := handlers.NewManagedBotHandler(db, cfg.ManagedBotServiceURL, cfg.ServiceToken)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Public routes
	r.Post("/auth/firebase/verify", authH.FirebaseVerify)

	// Protected routes (JWT only)
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

		// Threads
		r.Get("/threads", threadsH.List)
		r.Get("/threads/{id}/messages", threadsH.GetMessages)
		r.Post("/threads/{id}/takeover", threadsH.Takeover)
		r.Delete("/threads/{id}/takeover", threadsH.ReleaseTakeover)

		// WebSocket feed
		r.Get("/ws/feed", feedH.ServeWS)

		// Managed bot provisioning (JWT protected)
		r.Post("/internal/managed-bot/provision", managedBotH.Provision)
	})

	// Dual auth routes (JWT or service token)
	r.Group(func(r chi.Router) {
		r.Use(DualAuthMiddleware(jwtSvc, cfg.ServiceToken))

		r.Post("/messages", messagesH.Send)
	})

	// Service-token-only routes
	r.Group(func(r chi.Router) {
		r.Use(handlers.ServiceTokenMiddleware(cfg.ServiceToken))

		r.Get("/internal/threads/{thread_id}/messages", managedBotH.GetThreadMessages)
	})

	return r
}
