// Package db manages the PostgreSQL connection and schema migrations.
//
// Design decisions:
//   - We use database/sql with lib/pq rather than an ORM. This keeps things
//     transparent — every query is visible — and avoids pulling in a large
//     dependency graph for what is a simple schema.
//   - Migrations run as idempotent DDL ("IF NOT EXISTS") so the app can
//     restart safely without manual migration steps.
//   - The sequence for urls.id starts at 62^6 (56 800 235 584) to guarantee
//     that Base62-encoded short keys are always at least 6 characters long.
package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	// lib/pq registers itself as the "postgres" driver via its init() function.
	// The blank import is the standard Go idiom for database drivers.
	_ "github.com/lib/pq"
)

// Connect opens a connection pool to PostgreSQL and verifies it with a ping.
// The caller owns the returned *sql.DB and should defer db.Close().
func Connect(databaseURL string) (*sql.DB, error) {
	slog.Info("connecting to PostgreSQL…")

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: failed to open connection: %w", err)
	}

	// Ping validates that the DSN is correct and the server is reachable.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("db: ping failed: %w", err)
	}

	// Set connection pooling parameters
	maxOpen := 25
	if val := os.Getenv("DB_MAX_OPEN_CONNS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			maxOpen = parsed
		}
	}
	db.SetMaxOpenConns(maxOpen)

	maxIdle := 10
	if val := os.Getenv("DB_MAX_IDLE_CONNS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			maxIdle = parsed
		}
	}
	db.SetMaxIdleConns(maxIdle)

	maxLifetime := 15 * time.Minute
	if val := os.Getenv("DB_CONN_MAX_LIFETIME"); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil {
			maxLifetime = parsed
		}
	}
	db.SetConnMaxLifetime(maxLifetime)

	maxIdleTime := 5 * time.Minute
	if val := os.Getenv("DB_CONN_MAX_IDLE_TIME"); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil {
			maxIdleTime = parsed
		}
	}
	db.SetConnMaxIdleTime(maxIdleTime)

	slog.Info("PostgreSQL connection established")
	return db, nil
}

// RunMigrations creates the required tables and indices if they don't exist.
//
// We execute each statement individually so that errors are easy to diagnose.
// All DDL uses "IF NOT EXISTS" making it safe to call on every startup.
func RunMigrations(db *sql.DB) error {
	slog.Info("running database migrations…")

	// ── urls table ──────────────────────────────────────────────────────────
	// Stores the mapping from auto-generated BIGSERIAL id → short_key → long_url.
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS urls (
			id         BIGSERIAL PRIMARY KEY,
			short_key  VARCHAR(15) UNIQUE NOT NULL,
			long_url   TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return fmt.Errorf("db: create urls table: %w", err)
	}

	// Index on short_key for fast lookups during redirect.
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_urls_short_key ON urls(short_key);`)
	if err != nil {
		return fmt.Errorf("db: create idx_urls_short_key: %w", err)
	}

	// Add expires_at column to URLs table
	_, err = db.Exec(`ALTER TABLE urls ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP WITH TIME ZONE;`)
	if err != nil {
		return fmt.Errorf("db: add expires_at column: %w", err)
	}

	// Index on expires_at for fast checks
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_urls_expires_at ON urls(expires_at);`)
	if err != nil {
		return fmt.Errorf("db: create idx_urls_expires_at: %w", err)
	}

	// ── clicks table ────────────────────────────────────────────────────────
	// Stores per-click analytics. uuid PK avoids coordination between workers.
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS clicks (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			url_id     BIGINT REFERENCES urls(id) ON DELETE CASCADE,
			short_key  VARCHAR(15) NOT NULL,
			referrer   TEXT,
			user_agent TEXT,
			ip_address VARCHAR(45),
			country    VARCHAR(10),
			clicked_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return fmt.Errorf("db: create clicks table: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_clicks_short_key ON clicks(short_key);`)
	if err != nil {
		return fmt.Errorf("db: create idx_clicks_short_key: %w", err)
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_clicks_clicked_at ON clicks(clicked_at);`)
	if err != nil {
		return fmt.Errorf("db: create idx_clicks_clicked_at: %w", err)
	}

	// ── Sequence adjustment ─────────────────────────────────────────────────
	// 62^6 = 56 800 235 584.  Starting here ensures that every Base62-encoded
	// id has at least 6 characters, giving us clean, uniform-length short URLs
	// for the first ~3.5 trillion IDs (up to 62^7 - 1).
	//
	// We use a conditional check: only restart the sequence if its current
	// value is below our minimum. This prevents resetting the sequence if
	// URLs have already been created with higher IDs.
	_, err = db.Exec(`
		DO $$
		BEGIN
			IF (SELECT last_value FROM urls_id_seq) < 56800235584 THEN
				ALTER SEQUENCE urls_id_seq RESTART WITH 56800235584;
			END IF;
		END $$;
	`)
	if err != nil {
		return fmt.Errorf("db: set urls_id_seq start: %w", err)
	}

	slog.Info("database migrations completed successfully")
	return nil
}
