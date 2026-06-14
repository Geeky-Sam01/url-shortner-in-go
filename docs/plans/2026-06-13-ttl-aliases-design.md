# Design Document: Link Expiration (TTL) and Custom Aliases

This document outlines the technical design for adding optional Custom Aliases and Time-to-Live (TTL) expiration to shortened URLs.

---

## 1. Database Schema Changes
We will update `backend/db/db.go` to add a nullable `expires_at` column to the `urls` table:
```sql
ALTER TABLE urls ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP WITH TIME ZONE;
CREATE INDEX IF NOT EXISTS idx_urls_expires_at ON urls(expires_at);
```

---

## 2. API Payload & Validation

### Shorten Payload
We will extend `ShortenRequest` in `backend/handlers/url.go`:
```go
type ShortenRequest struct {
	URL      string `json:"url" binding:"required,url"`
	Alias    string `json:"alias" binding:"omitempty"`
	TTLHours int    `json:"ttl_hours" binding:"omitempty,min=0"`
}
```

### Custom Alias Validation
If a custom alias is provided:
1. **Format Check:** Must match regex `^[a-zA-Z0-9_-]{3,15}$`.
2. **Reserved Word Blacklist:** Reject aliases matching system routes: `api`, `health`, `dashboard`, `analytics`, `architecture`, `swagger`, `expired`.
3. **Uniqueness Check:** Reject alias if already present in PostgreSQL or Redis.

---

## 3. Caching & Redirect Handling

### Cache Invalidation via TTL
When writing a link to Redis:
- If `expires_at` is set, set the Redis key's expiration using the remaining TTL (`expires_at - time.Now()`).
- If not set, use a default 24-hour cache limit.

### Redirection Fallback
When executing `RedirectFallback`:
- Query PostgreSQL for `long_url` and `expires_at` if a cache miss occurs.
- If `expires_at` is set and the current time is past expiration, redirect the client to:
  `http://<frontend>/expired?key=<shortKey>`

---

## 4. Frontend UI & Routing

### New Route
Add `/expired` route mapping to a new `ExpiredComponent` in `app.routes.ts`.

### Dashboard Form Update
Add two optional fields under the main input box in `dashboard.component.html`:
1. Custom Alias (optional text field).
2. Expiration (select dropdown: Never, 1 Hour, 1 Day, 7 Days, 30 Days).
