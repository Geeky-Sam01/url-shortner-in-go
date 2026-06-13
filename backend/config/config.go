// Package config centralizes environment-based configuration for the URL shortener.
//
// Design decisions:
//   - All config comes from environment variables so the same binary runs
//     in local dev, CI, and Railway without code changes.
//   - We validate required vars eagerly at startup rather than letting
//     random packages fail later with cryptic errors.
//   - PORT defaults to 8080 which is the Railway convention.
package config

import (
	"fmt"
	"os"
)

// Config holds every tunable the application needs.
// Add new fields here rather than reading os.Getenv in random places.
type Config struct {
	// DatabaseURL is the full Supabase PostgreSQL connection string.
	// Example: "postgresql://user:pass@host:5432/postgres"
	DatabaseURL string

	// RedisURL is the Upstash Redis URL (must use rediss:// for TLS).
	// Example: "rediss://default:token@hostname:6379"
	RedisURL string

	// Port is the HTTP port Gin listens on. Railway sets this automatically.
	Port string

	// FrontendURL is the origin allowed by CORS (e.g. "https://myapp.vercel.app").
	FrontendURL string
}

// Load reads config from the environment and returns a validated Config.
// It returns an error if any required variable is missing so the caller
// can fail fast with a clear message.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		Port:        os.Getenv("PORT"),
		FrontendURL: os.Getenv("FRONTEND_URL"),
	}

	// --- Defaults -----------------------------------------------------------
	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	// --- Validation ---------------------------------------------------------
	// DATABASE_URL and REDIS_URL are hard requirements; the app is useless
	// without them, so we surface the error immediately.
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL environment variable is required")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("config: REDIS_URL environment variable is required")
	}

	return cfg, nil
}
