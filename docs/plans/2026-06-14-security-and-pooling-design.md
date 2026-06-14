# Design Document: Security Enhancements & DB Connection Pooling

This document outlines the design and architecture details for improving the security posture, rate-limiting accuracy, SSRF prevention, CORS flexibility, database durability, and build stability of the Go-based URL shortener application.

---

## 1. Client IP Extraction & Sliding-Window Rate Limiting

### Problem
Behind proxies (Render load balancers, Vercel edge/serverless gateways), client IPs can be obscured, making standard rate-limiting (by socket IP) ineffective. Additionally, simple rate limit counters are susceptible to traffic bursts at boundary windows.

### Solution
1. **IP Extraction Utility**:
   Implement a `GetClientIP(c *gin.Context)` helper:
   - Check if `TRUSTED_IP_HEADER` is set in the environment (e.g. `X-Real-IP`, `CF-Connecting-IP`). If present, use it.
   - If not set, check `X-Real-IP` then split `X-Forwarded-For` and take the first IP address.
   - Fall back to Gin's standard `c.ClientIP()`.
2. **Sliding-Window Rate Limiter**:
   - Replace the static window Lua script in `backend/handlers/url.go` with a sliding-window rate limit using a Redis sorted set (`ZSET`).
   - For a given client IP, the key will be `rate_limit:shorten:<ip>`.
   - Each request adds a timestamp member (`ZADD`) using the current Unix time.
   - Remove elements older than the window (60s) using `ZREMRANGEBYSCORE`.
   - Count the cardinality of the set (`ZCARD`). If it exceeds the limit (e.g., 10 requests), block the request.
   - Set an expiration on the ZSET itself to clean up memory automatically.

---

## 2. SSRF Protection & Self-Referential Validation

### Problem
Attackers can input loopback, link-local, private network, or metadata URLs (e.g., `http://127.0.0.1:5432`, `http://169.254.169.254/latest/meta-data/`) to map them or bypass restrictions. They can also input URLs pointing back to the url-shortener itself, causing routing loops.

### Solution
1. **IP Resolution**:
   - Parse the incoming long URL to extract the hostname.
   - Run a DNS lookup using `net.DefaultResolver.LookupIPAddr` with a 2-second timeout context.
2. **IP Classification**:
   - Check if any resolved IP address belongs to:
     - Loopback (`ip.IsLoopback()`)
     - Private subnets (RFC 1918)
     - Link-Local Unicast / Multicast (e.g., `169.254.169.254`)
     - Multicast / Unspecified (`0.0.0.0`)
   - Reject the URL if any of these match.
3. **Self-Referential Match**:
   - Resolve the IPs of `c.Request.Host` (the host running the API) and the host parsed from the configured `FRONTEND_URL`.
   - If any resolved target IP matches any of our own host IPs, reject the URL as self-referential.

---

## 3. Dynamic CORS Middleware

### Problem
The current CORS middleware hardcodes a single origin from `FRONTEND_URL` and has no support for credentials or preflight request customization.

### Solution
- Parse `ALLOWED_ORIGINS` from the environment as a comma-separated list of origins (e.g. `http://localhost:4200,https://my-app.vercel.app`).
- For each request, if the `Origin` header matches one of the allowed origins, set `Access-Control-Allow-Origin` to that exact origin.
- Set `Access-Control-Allow-Credentials: true`.
- Support standard HTTP methods (`GET, POST, DELETE, OPTIONS`) and standard headers (`Content-Type, Authorization, X-API-Key`).
- Handle `OPTIONS` preflight requests by returning `204 No Content` immediately.

---

## 4. Database Connection Pool Tuning

### Problem
Without limits, the Go database connection pool can open infinite connections, potentially crashing PostgreSQL (Supabase pooler limits) under heavy load. Also, low default idle connections can lead to socket thrashing.

### Solution
In `backend/db/db.go`, configure the `sql.DB` instance with:
- `SetMaxOpenConns(maxOpen)`: Limit max concurrent active connections (default `25`).
- `SetMaxIdleConns(maxIdle)`: Limit idle connections in the pool (default `10`).
- `SetConnMaxLifetime(maxLifetime)`: Recycles connections periodically (default `15m`).
- `SetConnMaxIdleTime(maxIdleTime)`: Cleans up idle connections (default `5m`).

These limits will be configurable via environment variables:
- `DB_MAX_OPEN_CONNS`
- `DB_MAX_IDLE_CONNS`
- `DB_CONN_MAX_LIFETIME`
- `DB_CONN_MAX_IDLE_TIME`

---

## 5. Dockerfile Fragility Fixes

### Problem
Using `alpine:latest` can introduce unexpected breaking updates. Additionally, lacking a `.dockerignore` causes local build artifacts and configuration files (like `.env`) to be copied into the Docker image, increasing image size and risking secret exposure.

### Solution
1. Update `backend/Dockerfile` to pin a stable base image: `alpine:3.20`.
2. Add a `.dockerignore` file in `backend/` to exclude:
   - `.env`
   - `bin/` or local binaries
   - `.git` and IDE directories (`.vscode`, `.idea`)
   - Logs or temporary files
