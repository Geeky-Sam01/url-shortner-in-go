# Hybrid Local Buffer and Fallback Queue Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Implement a hybrid local in-memory queue and background worker to batch-push click events to Redis (under high traffic) or write directly to PostgreSQL (under low traffic or Redis outages), reducing Redis request costs by up to 100%.

**Architecture:** We will initialize a buffered Go channel on the URL handler. Redirection writes click events to this channel. A background worker periodically drains it: events are batched and pushed to Redis if they total 100 (using a single command), or written directly to PostgreSQL if they are fewer than 100 or if Redis is down.

**Tech Stack:** Go (Go-redis, PostgreSQL sql.DB).

---

### Task 11: Implement Local Analytics Channel and Worker

**Files:**
- Modify: [url.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url.go)

**Step 1: Write a unit test verifying local queuing behavior**
Modify [url_test.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url_test.go):
- Update `setupTestRouter` to initialize `AnalyticsBufferChan`.
- Update `TestRedirectFallback` to assert on `AnalyticsBufferChan` instead of miniredis `analytics:queue`.

```go
// In backend/handlers/url_test.go setupTestRouter:
handler := &handlers.URLHandler{
	DB:                  mockDB,
	Redis:               rClient,
	FrontendURL:         "http://localhost:4200",
	AnalyticsBufferChan: make(chan handlers.AnalyticsEvent, 100),
}
```

**Step 2: Run tests to verify failure**
Run: `go test ./handlers/...`
Expected: FAIL due to missing `AnalyticsBufferChan` field in `URLHandler` struct definition.

**Step 3: Update `URLHandler` and implement local worker**
Modify [url.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url.go):
- Add `AnalyticsBufferChan chan AnalyticsEvent` field to `URLHandler`.
- Add `StartLocalBufferWorker(ctx context.Context, interval time.Duration)` method to `URLHandler`.
- Add `flushAnalyticsBatch(events []AnalyticsEvent)` method to `URLHandler`.
- Update `RedirectFallback` to push events to `AnalyticsBufferChan`.

**Step 4: Run tests to verify passes**
Run: `go test ./handlers/...`
Expected: PASS.

---

### Task 12: Wire Worker and Verify Compile

**Files:**
- Modify: [main.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/main.go)

**Step 1: Wire local worker initialization in main.go**
Modify [main.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/main.go):
- Initialize `AnalyticsBufferChan` on the `urlHandler`.
- Start the worker: `urlHandler.StartLocalBufferWorker(context.Background(), 10*time.Second)`.

**Step 2: Run all backend tests**
Run: `$env:TEST_SERVER_URL="http://localhost:9999"; go test ./...`
Expected: PASS.
