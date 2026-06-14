// Package redis wraps go-redis and provides application-specific helpers
// for URL caching and analytics event queuing.
//
// Design decisions:
//   - We parse the REDIS_URL to extract host, password, and TLS settings.
//     Upstash uses the "rediss://" scheme (note the double-s) which signals
//     TLS. We detect this and configure crypto/tls accordingly.
//   - URL cache uses a configurable TTL so popular URLs stay hot while
//     stale mappings eventually expire.
//   - Analytics events are queued via LPUSH / RPOP on a Redis list, giving
//     us a simple, reliable FIFO queue without extra infrastructure.
package redis

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
)

// analyticsQueueKey is the Redis list key used as a FIFO queue for click events.
// The analytics worker LPUSHes JSON blobs here; the batch inserter RPOPs them.
const analyticsQueueKey = "analytics:queue"

// Connect parses the Upstash Redis URL and returns a ready-to-use client.
//
// Expected URL format:  rediss://default:<password>@<host>:<port>
// The "rediss" scheme (with double-s) triggers TLS, which Upstash requires.
func Connect(redisURL string) (*redis.Client, error) {
	slog.Info("connecting to Redis…")

	parsed, err := url.Parse(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis: failed to parse REDIS_URL: %w", err)
	}

	password, _ := parsed.User.Password()

	opts := &redis.Options{
		Addr:     parsed.Host, // host:port
		Password: password,
	}

	// Upstash requires TLS — the URL scheme will be "rediss" (two s's).
	if parsed.Scheme == "rediss" {
		opts.TLSConfig = &tls.Config{
			// MinVersion ensures we don't negotiate anything below TLS 1.2.
			MinVersion: tls.VersionTLS12,
		}
	}

	client := redis.NewClient(opts)

	// Verify the connection is alive.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis: ping failed: %w", err)
	}

	slog.Info("Redis connection established", "addr", parsed.Host)
	return client, nil
}

// ---------------------------------------------------------------------------
// URL cache helpers
// ---------------------------------------------------------------------------

// SetURLCache stores a short_key → long_url mapping in Redis with a TTL.
// This avoids hitting PostgreSQL for every redirect on popular URLs.
func SetURLCache(ctx context.Context, client *redis.Client, key string, longURL string, ttl time.Duration) error {
	return client.Set(ctx, "url:"+key, longURL, ttl).Err()
}

// GetURLCache retrieves a cached long URL by short key.
// Returns ("", redis.Nil) on a cache miss so callers can fall through to the DB.
func GetURLCache(ctx context.Context, client *redis.Client, key string) (string, error) {
	return client.Get(ctx, "url:"+key).Result()
}

// DeleteURLCache invalidates/removes a short key mapping from Redis cache.
func DeleteURLCache(ctx context.Context, client *redis.Client, key string) error {
	return client.Del(ctx, "url:"+key).Err()
}

// ---------------------------------------------------------------------------
// Analytics queue helpers
// ---------------------------------------------------------------------------

// PushAnalyticsEvent appends a JSON-encoded click event to the analytics queue.
// We use LPUSH so new events land at the head; the consumer RPOPs from the tail,
// giving us FIFO ordering.
func PushAnalyticsEvent(ctx context.Context, client *redis.Client, eventJSON string) error {
	return client.LPush(ctx, analyticsQueueKey, eventJSON).Err()
}

// PopAnalyticsEvents removes and returns up to `count` events from the tail of
// the analytics queue.  It uses a pipeline of individual RPOP commands because
// Upstash's free tier doesn't support LMPOP (Redis 7+ command).
//
// Returns an empty slice (not an error) when the queue is empty.
func PopAnalyticsEvents(ctx context.Context, client *redis.Client, count int64) ([]string, error) {
	// First check the length of the queue to save Redis command quota on empty queues.
	llen, err := client.LLen(ctx, analyticsQueueKey).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: llen failed: %w", err)
	}
	if llen == 0 {
		return nil, nil
	}

	// Only pop up to the actual number of elements present, capped at count
	popCount := count
	if llen < popCount {
		popCount = llen
	}

	pipe := client.Pipeline()

	// Queue up popCount RPOPs in a single round-trip.
	cmds := make([]*redis.StringCmd, popCount)
	for i := int64(0); i < popCount; i++ {
		cmds[i] = pipe.RPop(ctx, analyticsQueueKey)
	}

	// Exec sends all commands at once. Individual commands may return
	// redis.Nil when the list is exhausted — that's expected, not an error.
	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("redis: pipeline exec failed: %w", err)
	}

	results := make([]string, 0, popCount)
	for _, cmd := range cmds {
		val, err := cmd.Result()
		if err == redis.Nil {
			// List exhausted — stop collecting.
			break
		}
		if err != nil {
			return results, fmt.Errorf("redis: rpop failed: %w", err)
		}
		results = append(results, val)
	}

	return results, nil
}

// RequeueAnalyticsEvents puts events back at the tail of the queue (RPush) in a pipeline.
func RequeueAnalyticsEvents(ctx context.Context, client *redis.Client, events []string) error {
	if len(events) == 0 {
		return nil
	}
	pipe := client.Pipeline()
	for _, event := range events {
		pipe.RPush(ctx, analyticsQueueKey, event)
	}
	_, err := pipe.Exec(ctx)
	return err
}

