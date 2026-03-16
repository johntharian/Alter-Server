package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/john/botsapp/internal/logger"
	"github.com/john/botsapp/internal/models"
)

func Connect(databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	config.MaxConns = 20
	config.MinConns = 5
	config.MaxConnLifetime = 30 * time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	logger.Info("Connected to PostgreSQL", nil)
	return pool, nil
}

func RunMigrations(pool *pgxpool.Pool) error {
	ctx := context.Background()

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			phone_number TEXT UNIQUE NOT NULL,
			display_name TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT now(),
			updated_at TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS bot_endpoints (
			user_id BIGINT UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			url TEXT NOT NULL,
			secret_key TEXT NOT NULL,
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT now(),
			updated_at TIMESTAMPTZ DEFAULT now(),
			PRIMARY KEY (user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS contacts (
			user_id BIGINT NOT NULL REFERENCES users(id),
			contact_user_id BIGINT NOT NULL REFERENCES users(id),
			added_at TIMESTAMPTZ DEFAULT now(),
			PRIMARY KEY (user_id, contact_user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_contact_user_id ON contacts (contact_user_id)`,
		`CREATE TABLE IF NOT EXISTS threads (
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			participant_a BIGINT NOT NULL REFERENCES users(id),
			participant_b BIGINT NOT NULL REFERENCES users(id),
			human_takeover_by BIGINT REFERENCES users(id),
			created_at TIMESTAMPTZ DEFAULT now(),
			UNIQUE (participant_a, participant_b)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_threads_participants ON threads (participant_a, participant_b)`,
		`CREATE INDEX IF NOT EXISTS idx_threads_participant_b ON threads (participant_b)`,
		`CREATE INDEX IF NOT EXISTS idx_threads_human_takeover_by ON threads (human_takeover_by)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			thread_id BIGINT NOT NULL REFERENCES threads(id),
			from_user_id BIGINT NOT NULL REFERENCES users(id),
			to_user_id BIGINT NOT NULL REFERENCES users(id),
			intent TEXT DEFAULT '',
			payload JSONB DEFAULT '{}',
			status TEXT DEFAULT 'queued'
				CHECK (status IN ('queued','delivered','client_delivered','processed','failed')),
			human_override BOOLEAN DEFAULT false,
			retry_count INT DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_thread ON messages (thread_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_from_user ON messages (from_user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_to_user ON messages (to_user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_status
			ON messages (status) WHERE status = 'queued'`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS is_managed_bot BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS managed_bot_url TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS managed_bot_secret TEXT`,
		`ALTER TABLE users ENABLE ROW LEVEL SECURITY`,
		`ALTER TABLE bot_endpoints ENABLE ROW LEVEL SECURITY`,
		`ALTER TABLE contacts ENABLE ROW LEVEL SECURITY`,
		`ALTER TABLE threads ENABLE ROW LEVEL SECURITY`,
		`ALTER TABLE messages ENABLE ROW LEVEL SECURITY`,
	}

	for i, m := range migrations {
		if _, err := pool.Exec(ctx, m); err != nil {
			return fmt.Errorf("migration %d failed: %w", i, err)
		}
	}

	logger.Info("Database migrations completed", nil)
	return nil
}

// SetManagedBot marks a user as a managed bot and stores the bot URL and secret.
func SetManagedBot(ctx context.Context, pool *pgxpool.Pool, userID int64, botURL, secret string) error {
	tag, err := pool.Exec(ctx,
		`UPDATE users SET is_managed_bot = true, managed_bot_url = $1, managed_bot_secret = $2, updated_at = now()
		 WHERE id = $3`,
		botURL, secret, userID,
	)
	if err != nil {
		return fmt.Errorf("set managed bot: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set managed bot: user %d not found", userID)
	}
	return nil
}

// GetUserWithBotConfig returns a user with managed bot fields populated.
func GetUserWithBotConfig(ctx context.Context, pool *pgxpool.Pool, userID int64) (*models.User, error) {
	var u models.User
	err := pool.QueryRow(ctx,
		`SELECT id, phone_number, display_name, is_managed_bot, managed_bot_url, managed_bot_secret, created_at, updated_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(&u.ID, &u.PhoneNumber, &u.DisplayName, &u.IsManagedBot, &u.ManagedBotURL, &u.ManagedBotSecret, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user with bot config: %w", err)
	}
	return &u, nil
}
