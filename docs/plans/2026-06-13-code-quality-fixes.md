# Code Quality and Bug Fixes Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Fix the broken unit tests, clean up test helper dead code, tune database connection pooling, resolve Dockerfile build fragility, and implement robust sliding-window rate limiting.

**Architecture:** 
1. **Clean up Test Helpers**: Remove the unused shadowed `db` parameter from `setupTestRouter` in `backend/handlers/url_test.go`.
2. **Fix Tests**: Update sqlmock expectations for url creation and redirect queries to include the `expires_at` column.
3. **Database Tuning**: Configure connection limit metrics on the Postgres driver.
4. **Sliding Window Rate Limiter**: Use a Redis Sorted Set (ZSET) inside a transactional pipeline to implement a sliding-window rate limiter, preventing window-boundary bursts.
5. **Dockerfile**: Standardize Go build targets to use `.` instead of a single file.

**Tech Stack:** Go, database/sql (PostgreSQL), Redis (ZSET), Gin, sqlmock

---

### Task 1: Clean Up Test Helper Dead Code & Fix Test Signatures

**Files:**
- Modify: [url_test.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url_test.go)

**Step 1: Write the clean code changes**
Update `setupTestRouter` function signature to remove the unused first parameter:
```go
func setupTestRouter(rClient *redis.Client) (*gin.Engine, *handlers.URLHandler, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	router := gin.Default()

	mockDB, mock, _ := sqlmock.New()
	handler := &handlers.URLHandler{
		DB:          mockDB,
		Redis:       rClient,
		FrontendURL: "http://localhost:4200",
	}

	router.POST("/api/shorten", handler.CreateURL)
	router.GET("/:key", handler.RedirectFallback)
	router.GET("/api/urls", handler.GetURLs)

	return router, handler, mock
}
```
Update all test functions to invoke it as `setupTestRouter(rClient)`.

**Step 2: Run tests and ensure compilation**
Run: `go test ./...` in `backend`
Expected: Compilation works, but SQL mock expectations fail (since SQL structure was changed by the TTL feature).

**Step 3: Commit**
```bash
git add backend/handlers/url_test.go
git commit -m "test: clean up unused parameter from setupTestRouter helper"
```

---

### Task 2: Fix Broken SQL Mock Expectations in Handler Tests

**Files:**
- Modify: [url_test.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url_test.go)

**Step 1: Update expectations in `TestCreateURL`**
Update the insert mock statement to match 4 parameters instead of 3:
```go
	// Mock insert with expires_at (arg 4, nil)
	mock.ExpectExec(`INSERT INTO urls`).
		WithArgs(int64(56800235584), expectedShortKey, "https://example.com", nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
```

**Step 2: Update expectations in `TestRedirectFallback`**
Update the SELECT query string to include `expires_at` and return a NullTime:
```go
	mock.ExpectQuery(`SELECT id, long_url, expires_at FROM urls WHERE short_key = \$1`).
		WithArgs(shortKey).
		WillReturnRows(sqlmock.NewRows([]string{"id", "long_url", "expires_at"}).AddRow(1, longURL, nil))
```

**Step 3: Run the tests**
Run: `go test ./...` in `backend`
Expected: PASS

**Step 4: Commit**
```bash
git add backend/handlers/url_test.go
git commit -m "test: fix mock expectations to include expires_at column"
```

---

### Task 3: Implement Sliding-Window Rate Limiting

**Files:**
- Modify: [url.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url.go)

**Step 1: Implement ZSET-based sliding window**
Replace the rate limiting logic in `CreateURL` with a Redis Sorted Set transaction pipeline:
```go
	// IP-based Rate Limiting (Sliding Window Log using Redis ZSET)
	ip := c.ClientIP()
	rateLimitKey := "rate_limit:shorten:" + ip
	now := time.Now().UnixNano()
	window := int64(time.Minute)
	limit := int64(10)
	clearBefore := now - window

	pipe := h.Redis.TxPipeline()
	// Remove events outside the current sliding window
	pipe.ZRemRangeByScore(c.Request.Context(), rateLimitKey, "-inf", fmt.Sprintf("%d", clearBefore))
	// Count events in the window
	pipe.ZCard(c.Request.Context(), rateLimitKey)
	// Add current request event
	pipe.ZAdd(c.Request.Context(), rateLimitKey, redis.Z{Score: float64(now), Member: now})
	// Set TTL to cleanup idle keys
	pipe.Expire(c.Request.Context(), rateLimitKey, time.Minute)

	cmds, err := pipe.Exec(c.Request.Context())
	if err != nil {
		slog.Error("redis rate limit error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// The second command (index 1) in pipeline returns the ZCard count before adding the current member
	countCmd, ok := cmds[1].(*redis.IntCmd)
	if !ok {
		slog.Error("redis rate limit parse error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	count, err := countCmd.Result()
	if err != nil {
		slog.Error("redis rate limit count error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if count >= limit {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too Many Requests"})
		return
	}
```

**Step 2: Run tests**
Run: `go test ./...` in `backend`
Expected: PASS

**Step 3: Commit**
```bash
git add backend/handlers/url.go
git commit -m "feat: migrate from fixed-window to robust sliding-window rate limiting"
```

---

### Task 4: Configure Database Connection Pool Tuning

**Files:**
- Modify: [db.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/db/db.go)

**Step 1: Set database limits**
Update the `Connect` function in `backend/db/db.go` to explicitly configure the pool properties:
```go
func Connect(databaseURL string) (*sql.DB, error) {
	slog.Info("connecting to PostgreSQL…")

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: failed to open connection: %w", err)
	}

	// Tune connection pool settings
	db.SetMaxOpenConns(25)                 // Limit concurrent connections to avoid Supabase/Railway exhaustion
	db.SetMaxIdleConns(25)                 // Keep idle connections ready to serve immediate traffic
	db.SetConnMaxLifetime(5 * time.Minute) // Periodically recycle connections to free up resource leaks

	// Ping validates that the DSN is correct and the server is reachable.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("db: ping failed: %w", err)
	}

	slog.Info("PostgreSQL connection established")
	return db, nil
}
```

**Step 2: Compile backend**
Run: `go build` in `backend`
Expected: Compilation works.

**Step 3: Commit**
```bash
git add backend/db/db.go
git commit -m "perf: tune database connection pool limits"
```

---

### Task 5: Fix Dockerfile Fragility

**Files:**
- Modify: [Dockerfile](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/Dockerfile)

**Step 1: Change build statement**
Change:
```dockerfile
# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./main.go
```
To:
```dockerfile
# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server .
```

**Step 2: Commit**
```bash
git add backend/Dockerfile
git commit -m "build: fix fragile Dockerfile build command"
```

---

### Verification and Sanity Verification

**Verification Commands:**
- Run all backend tests: `go test ./... -v`
- Confirm server boots locally: `go run main.go`
