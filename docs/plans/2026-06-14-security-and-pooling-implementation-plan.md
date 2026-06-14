# Security Enhancements & DB Connection Pooling Implementation Plan

> **For Antigravity:** REQUIRED WORKFLOW: Use `.agent/workflows/execute-plan.md` to execute this plan in single-flow mode.

**Goal:** Implement robust client IP extraction, sliding-window rate limiting, SSRF protection, dynamic CORS, database connection pool tuning, and Dockerfile stability improvements.

**Architecture:** Add utility functions for DNS resolution and IP classification, replace the static Redis rate limit counter with a Redis sorted set (`ZSET`) sliding-window limiter, create a dynamic CORS middleware, configure database connection limits in the pg client pool, and pin Docker base images.

**Tech Stack:** Go (Golang), Gin web framework, Redis (go-redis), PostgreSQL (lib/pq), Docker.

---

### Task 1: Clean Up Test Helper Dead Code & Fix Test Signatures in `url_test.go`

**Files:**
- Modify: [url_test.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url_test.go)

**Step 1: Write code changes**
Update the `setupTestRouter` function signature to accept and set up the DB mock correctly, and clean up any dead test helper functions that don't match the new handlers package API.

**Step 2: Verify compilation**
Run: `go test -run=TestDoesNotExist ./handlers`
Expected: Passes (compiles successfully, no tests run).

---

### Task 2: Fix Broken SQL Mock Expectations in Handler Tests

**Files:**
- Modify: [url_test.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url_test.go)

**Step 1: Write code changes**
Update `TestCreateURL` and `TestRedirectFallback` mock expectations to match the database schemas:
1. In `TestCreateURL`, update the insert expectations to expect 4 arguments (id, short_key, long_url, expires_at) instead of 3.
2. In `TestRedirectFallback`, update the select query mock to expect `id, long_url, expires_at` and return an `expires_at` column.

**Step 2: Run tests to verify they pass**
Run: `go test -run="(TestCreateURL|TestRedirectFallback)" ./handlers`
Expected: PASS

---

### Task 3: Implement Robust Client IP Extraction & Sliding-Window Rate Limiting

**Files:**
- Modify: [url.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url.go)
- Modify: [url_test.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url_test.go)

**Step 1: Implement Client IP Extraction**
Add a helper in `url.go`:
```go
func GetClientIP(c *gin.Context) string {
	if header := c.GetHeader("TRUSTED_IP_HEADER"); header != "" {
		if val := c.GetHeader(header); val != "" {
			return strings.TrimSpace(strings.Split(val, ",")[0])
		}
	}
	if realIP := c.GetHeader("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	return c.ClientIP()
}
```

**Step 2: Implement Sliding-Window Rate Limiting**
Update the Lua rate limit script in `url.go` to use a sliding window via ZSET:
```go
var rateLimitScript = redis.NewScript(`
	local key = KEYS[1]
	local now = tonumber(ARGV[1])
	local window = tonumber(ARGV[2])
	local limit = tonumber(ARGV[3])
	local clearBefore = now - window

	redis.call("ZREMRANGEBYSCORE", key, "-inf", clearBefore)
	local count = redis.call("ZCARD", key)
	if count < limit then
		redis.call("ZADD", key, now, now)
		redis.call("EXPIRE", key, window)
		return count + 1
	else
		return count + 1
	end
`)
```
Update the `CreateURL` handler to use this new Lua script with `GetClientIP(c)` and pass `now`, `window` (60), and `limit` (10) as arguments.

**Step 3: Run tests to verify**
Run: `go test -run=TestCreateURL_RateLimit ./handlers`
Expected: PASS

---

### Task 4: Implement SSRF Protection & Self-Referential Validation

**Files:**
- Modify: [url.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url.go)
- Modify: [url_test.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/handlers/url_test.go)

**Step 1: Implement IP Resolution and Classification**
Create a helper function to check for SSRF and self-referential addresses:
```go
func isSSRFOrSelfReferential(targetURL string, clientHost string, frontendURL string) bool {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return true
	}
	host := parsed.Hostname()
	
	// Resolve IPs for the target host
	ips, err := net.LookupIP(host)
	if err != nil {
		return true // Block if DNS fails or cannot resolve
	}

	// Resolve local IPs of this server / frontend
	var localIPs []net.IP
	if clientHost != "" {
		if h, _, err := net.SplitHostPort(clientHost); err == nil {
			if local, err := net.LookupIP(h); err == nil {
				localIPs = append(localIPs, local...)
			}
		} else {
			if local, err := net.LookupIP(clientHost); err == nil {
				localIPs = append(localIPs, local...)
			}
		}
	}
	if frontendURL != "" {
		if parsedFrontend, err := url.Parse(frontendURL); err == nil {
			if local, err := net.LookupIP(parsedFrontend.Hostname()); err == nil {
				localIPs = append(localIPs, local...)
			}
		}
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
			return true // Private/internal IP -> Block
		}
		// Compare with self IPs
		for _, localIP := range localIPs {
			if ip.Equal(localIP) {
				return true // Self-referential IP -> Block
			}
		}
	}
	return false
}
```
Update `CreateURL` handler to invoke this check.

**Step 2: Add and Run SSRF / Self-Referential Tests**
Write unit tests validating that localhost, metadata service (`169.254.169.254`), private network subnets, and self-referential domains are correctly rejected.
Run: `go test -run=TestCreateURL_SelfReferential ./handlers`
Expected: PASS

---

### Task 5: Implement Dynamic CORS Middleware

**Files:**
- Modify: [main.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/main.go)

**Step 1: Write dynamic CORS middleware**
Replace the static CORS middleware in `main.go` with:
```go
	// Dynamic CORS middleware supporting credentials and dynamic origin matching
	allowedOrigins := strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",")
	if os.Getenv("FRONTEND_URL") != "" {
		allowedOrigins = append(allowedOrigins, os.Getenv("FRONTEND_URL"))
	}
	
	router.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowed := false
		for _, o := range allowedOrigins {
			if strings.TrimSpace(o) == origin {
				allowed = true
				break
			}
		}
		if allowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		} else if len(allowedOrigins) > 0 && allowedOrigins[0] != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", allowedOrigins[0])
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
```

**Step 2: Verify server compilation**
Run: `go build -o /tmp/main ./main.go`
Expected: Successful compile

---

### Task 6: Configure Database Connection Pool Tuning

**Files:**
- Modify: [db.go](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/db/db.go)

**Step 1: Implement Connection Pool Tuning**
In `db.go`, update the `Connect` function to read environment variables and set pooling variables:
```go
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
```

**Step 2: Verify DB connection tests**
Run: `go test ./db`
Expected: PASS

---

### Task 7: Fix Dockerfile Fragility

**Files:**
- Modify: [Dockerfile](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/Dockerfile)
- Create: [backend/.dockerignore](file:///c:/Users/admin/Desktop/Projects/url-shortner/backend/.dockerignore)

**Step 1: Pin Alpine base image**
Modify `backend/Dockerfile` to pin `alpine:3.20` instead of `alpine:latest`.

**Step 2: Create .dockerignore**
Create `backend/.dockerignore` and add:
```text
.env
*test.go
tmp/
.git
.vscode
.idea
Dockerfile
```

**Step 3: Verify local docker build**
Run: `docker build -t url-shortener-backend backend/` (if docker is available) or verify syntax.
Expected: PASS
