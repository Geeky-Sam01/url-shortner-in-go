package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/url-shortner/backend/handlers"
	myredis "github.com/url-shortner/backend/redis"
)

// AnalyticsWorker processes click events from Redis and batches them to PostgreSQL.
type AnalyticsWorker struct {
	DB        *sql.DB
	Redis     *redis.Client
	BatchSize int64
	Interval  time.Duration
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

func NewAnalyticsWorker(db *sql.DB, rClient *redis.Client) *AnalyticsWorker {
	return &AnalyticsWorker{
		DB:        db,
		Redis:     rClient,
		BatchSize: 100,
		Interval:  5 * time.Second,
		stopCh:    make(chan struct{}),
	}
}

func (w *AnalyticsWorker) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-w.stopCh:
				// Process one last batch before shutting down
				w.processBatch()
				return
			case <-ticker.C:
				w.processBatch()
			}
		}
	}()
	slog.Info("analytics worker started")
}

func (w *AnalyticsWorker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	slog.Info("analytics worker stopped")
}

func (w *AnalyticsWorker) processBatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventsJSON, err := myredis.PopAnalyticsEvents(ctx, w.Redis, w.BatchSize)
	if err != nil {
		slog.Error("analytics worker: failed to pop events from redis", "error", err)
		return
	}

	if len(eventsJSON) == 0 {
		return // Nothing to do
	}

	// Prepare batch insert
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("analytics worker: failed to begin tx", "error", err)
		w.requeueEvents(ctx, eventsJSON)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO clicks (url_id, short_key, referrer, user_agent, ip_address, country, clicked_at)
		VALUES ((SELECT id FROM urls WHERE short_key = $1), $1, $2, $3, $4, $5, $6)`)
	if err != nil {
		slog.Error("analytics worker: failed to prepare stmt", "error", err)
		w.requeueEvents(ctx, eventsJSON)
		return
	}
	defer stmt.Close()

	for _, eventStr := range eventsJSON {
		var event handlers.AnalyticsEvent
		if err := json.Unmarshal([]byte(eventStr), &event); err != nil {
			slog.Warn("analytics worker: failed to decode event JSON", "error", err, "json", eventStr)
			continue
		}

		_, err = stmt.ExecContext(ctx, event.ShortKey, event.Referrer, event.UserAgent, event.IPAddress, event.Country, event.ClickedAt)
		if err != nil {
			slog.Warn("analytics worker: failed to insert event", "error", err, "short_key", event.ShortKey)
			// Continue with other events instead of failing the whole batch
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("analytics worker: failed to commit tx", "error", err)
		w.requeueEvents(ctx, eventsJSON)
	} else {
		slog.Info("analytics worker: batch processed", "count", len(eventsJSON))
	}
}

func (w *AnalyticsWorker) requeueEvents(ctx context.Context, events []string) {
	slog.Warn("analytics worker: requeueing events due to database batch insert failure", "count", len(events))
	if err := myredis.RequeueAnalyticsEvents(ctx, w.Redis, events); err != nil {
		slog.Error("analytics worker: failed to requeue events to redis", "error", err)
	}
}

