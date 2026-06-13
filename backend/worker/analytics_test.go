package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestAnalyticsWorkerRecovery_OnTxFailure(t *testing.T) {
	t.Run("BeginTx Failure", func(t *testing.T) {
		mr, err := miniredis.Run()
		if err != nil {
			t.Fatalf("failed to start miniredis: %v", err)
		}
		defer mr.Close()

		rClient := redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
		})
		defer rClient.Close()

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		// BeginTx fails
		mock.ExpectBegin().WillReturnError(errors.New("db begin failure"))

		worker := &AnalyticsWorker{
			DB:        db,
			Redis:     rClient,
			BatchSize: 10,
			Interval:  5 * time.Second,
		}

		events := []string{
			`{"short_key":"abc","referrer":"direct","user_agent":"Mozilla","ip_address":"127.0.0.1","country":"US","clicked_at":"2026-06-13T12:00:00Z"}`,
			`{"short_key":"xyz","referrer":"google","user_agent":"Chrome","ip_address":"127.0.0.2","country":"CA","clicked_at":"2026-06-13T12:01:00Z"}`,
		}

		for _, ev := range events {
			err := rClient.LPush(context.Background(), "analytics:queue", ev).Err()
			if err != nil {
				t.Fatalf("failed to push event: %v", err)
			}
		}

		// Process the batch
		worker.processBatch()

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}

		// Verify the events are back in the Redis queue
		listElems, err := rClient.LRange(context.Background(), "analytics:queue", 0, -1).Result()
		if err != nil {
			t.Fatalf("failed to LRange: %v", err)
		}

		if len(listElems) != 2 {
			t.Fatalf("expected 2 elements in list, got %d", len(listElems))
		}
		if listElems[0] != events[0] || listElems[1] != events[1] {
			t.Errorf("mismatched queue order or content. expected: %v, got: %v", events, listElems)
		}
	})

	t.Run("PrepareContext Failure", func(t *testing.T) {
		mr, err := miniredis.Run()
		if err != nil {
			t.Fatalf("failed to start miniredis: %v", err)
		}
		defer mr.Close()

		rClient := redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
		})
		defer rClient.Close()

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		// BeginTx succeeds
		mock.ExpectBegin()

		// Prepare statement fails
		mock.ExpectPrepare(`INSERT INTO clicks`).WillReturnError(errors.New("prepare statement failure"))

		worker := &AnalyticsWorker{
			DB:        db,
			Redis:     rClient,
			BatchSize: 10,
			Interval:  5 * time.Second,
		}

		event := `{"short_key":"abc","referrer":"direct","user_agent":"Mozilla","ip_address":"127.0.0.1","country":"US","clicked_at":"2026-06-13T12:00:00Z"}`
		err = rClient.LPush(context.Background(), "analytics:queue", event).Err()
		if err != nil {
			t.Fatalf("failed to push event: %v", err)
		}

		// Process the batch
		worker.processBatch()

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}

		// Verify the event is back in the Redis queue
		listElems, err := rClient.LRange(context.Background(), "analytics:queue", 0, -1).Result()
		if err != nil {
			t.Fatalf("failed to LRange: %v", err)
		}

		if len(listElems) != 1 {
			t.Fatalf("expected 1 element in list, got %d", len(listElems))
		}
		if listElems[0] != event {
			t.Errorf("mismatched queue content. expected: %v, got: %v", event, listElems[0])
		}
	})

	t.Run("Commit Failure", func(t *testing.T) {
		mr, err := miniredis.Run()
		if err != nil {
			t.Fatalf("failed to start miniredis: %v", err)
		}
		defer mr.Close()

		rClient := redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
		})
		defer rClient.Close()

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		// BeginTx succeeds
		mock.ExpectBegin()

		// Prepare statement succeeds
		mock.ExpectPrepare(`INSERT INTO clicks`).
			ExpectExec().
			WithArgs("abc", "direct", "Mozilla", "127.0.0.1", "US", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Commit fails
		mock.ExpectCommit().WillReturnError(errors.New("db commit failure"))

		worker := &AnalyticsWorker{
			DB:        db,
			Redis:     rClient,
			BatchSize: 10,
			Interval:  5 * time.Second,
		}

		event := `{"short_key":"abc","referrer":"direct","user_agent":"Mozilla","ip_address":"127.0.0.1","country":"US","clicked_at":"2026-06-13T12:00:00Z"}`
		err = rClient.LPush(context.Background(), "analytics:queue", event).Err()
		if err != nil {
			t.Fatalf("failed to push event: %v", err)
		}

		// Process the batch
		worker.processBatch()

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}

		// Verify the event is back in the Redis queue
		listElems, err := rClient.LRange(context.Background(), "analytics:queue", 0, -1).Result()
		if err != nil {
			t.Fatalf("failed to LRange: %v", err)
		}

		if len(listElems) != 1 {
			t.Fatalf("expected 1 element in list, got %d", len(listElems))
		}
		if listElems[0] != event {
			t.Errorf("mismatched queue content. expected: %v, got: %v", event, listElems[0])
		}
	})
}
