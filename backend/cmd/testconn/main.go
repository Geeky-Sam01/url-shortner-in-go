// Command testconn is a standalone diagnostic tool that verifies every
// external dependency (PostgreSQL, Redis) and the Base62 codec.
//
// Usage:
//
//	Set DATABASE_URL and REDIS_URL in your environment, then run:
//	  go run ./cmd/testconn
//
// Required environment variables:
//   - DATABASE_URL  — Supabase PostgreSQL connection string
//   - REDIS_URL     — Upstash Redis connection string (rediss://...)
//
// The program exits with code 0 if all tests pass, 1 otherwise.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/url-shortner/backend/config"
	"github.com/url-shortner/backend/db"
	appredis "github.com/url-shortner/backend/redis"
	"github.com/url-shortner/backend/utils"
)

func main() {
	slog.Info("=== URL Shortener Connection & Codec Test ===")

	allPassed := true

	// ── Step 1: Load configuration ──────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		fmt.Println("FAIL  Config")
		os.Exit(1)
	}
	fmt.Println("PASS  Config loaded")

	// ── Step 2: Test PostgreSQL ─────────────────────────────────────────────
	pgDB, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("PostgreSQL connection failed", "error", err)
		fmt.Println("FAIL  PostgreSQL connect")
		allPassed = false
	} else {
		defer pgDB.Close()
		fmt.Println("PASS  PostgreSQL connect")

		// Run a simple query to confirm we can actually execute SQL.
		var result int
		if err := pgDB.QueryRow("SELECT 1").Scan(&result); err != nil || result != 1 {
			slog.Error("PostgreSQL query failed", "error", err)
			fmt.Println("FAIL  PostgreSQL query")
			allPassed = false
		} else {
			fmt.Println("PASS  PostgreSQL query (SELECT 1)")
		}

		// Test migrations.
		if err := db.RunMigrations(pgDB); err != nil {
			slog.Error("migrations failed", "error", err)
			fmt.Println("FAIL  PostgreSQL migrations")
			allPassed = false
		} else {
			fmt.Println("PASS  PostgreSQL migrations")
		}
	}

	// ── Step 3: Test Redis ──────────────────────────────────────────────────
	rdb, err := appredis.Connect(cfg.RedisURL)
	if err != nil {
		slog.Error("Redis connection failed", "error", err)
		fmt.Println("FAIL  Redis connect")
		allPassed = false
	} else {
		defer rdb.Close()
		fmt.Println("PASS  Redis connect")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// SET then GET a test key to verify full read/write capability.
		testKey := "test:conn:check"
		testVal := "hello-from-testconn"

		if err := rdb.Set(ctx, testKey, testVal, 30*time.Second).Err(); err != nil {
			slog.Error("Redis SET failed", "error", err)
			fmt.Println("FAIL  Redis SET")
			allPassed = false
		} else {
			got, err := rdb.Get(ctx, testKey).Result()
			if err != nil || got != testVal {
				slog.Error("Redis GET failed", "error", err, "got", got)
				fmt.Println("FAIL  Redis GET")
				allPassed = false
			} else {
				fmt.Println("PASS  Redis SET/GET")
			}
			// Clean up the test key.
			rdb.Del(ctx, testKey)
		}
	}

	// ── Step 4: Test Base62 codec ───────────────────────────────────────────
	base62Tests := []struct {
		num     int64
		encoded string
	}{
		{0, "0"},
		{1, "1"},
		{61, "Z"},
		{62, "10"},
		{56800235584, ""}, // We'll just verify round-trip for this one.
	}

	base62Passed := true
	for _, tt := range base62Tests {
		enc := utils.Encode(tt.num)
		dec := utils.Decode(enc)

		if dec != tt.num {
			slog.Error("Base62 round-trip failed",
				"input", tt.num, "encoded", enc, "decoded", dec)
			base62Passed = false
			continue
		}

		// If we have an expected encoding, verify it matches.
		if tt.encoded != "" && enc != tt.encoded {
			slog.Error("Base62 encoding mismatch",
				"input", tt.num, "got", enc, "want", tt.encoded)
			base62Passed = false
		}
	}

	// Verify the sequence start value produces a key of length >= 6.
	seqEncoded := utils.Encode(56800235584)
	if len(seqEncoded) < 6 {
		slog.Error("sequence start key too short",
			"encoded", seqEncoded, "len", len(seqEncoded))
		base62Passed = false
	}

	if base62Passed {
		fmt.Printf("PASS  Base62 codec (sequence start → %q, len=%d)\n",
			seqEncoded, len(seqEncoded))
	} else {
		fmt.Println("FAIL  Base62 codec")
		allPassed = false
	}

	// ── Summary ─────────────────────────────────────────────────────────────
	fmt.Println()
	if allPassed {
		fmt.Println("✅ All tests passed!")
	} else {
		fmt.Println("❌ Some tests failed — see errors above.")
		os.Exit(1)
	}
}
