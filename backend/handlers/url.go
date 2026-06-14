package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	myredis "github.com/url-shortner/backend/redis"
	"github.com/url-shortner/backend/utils"
)

// ShortenRequest defines the payload for creating a short URL.
type ShortenRequest struct {
	URL      string `json:"url" binding:"required,url"`
	Alias    string `json:"alias" binding:"omitempty"`
	TTLHours int    `json:"ttl_hours" binding:"omitempty,min=0"`
}

// ShortenResponse defines the response after creating a short URL.
type ShortenResponse struct {
	ShortKey string `json:"short_key"`
	ShortURL string `json:"short_url"`
}

// AnalyticsEvent represents a click that we will push to the Redis queue.
type AnalyticsEvent struct {
	ShortKey  string    `json:"short_key"`
	Referrer  string    `json:"referrer"`
	UserAgent string    `json:"user_agent"`
	IPAddress string    `json:"ip_address"`
	Country   string    `json:"country"` // Requires geoip parsing in worker
	ClickedAt time.Time `json:"clicked_at"`
}

// URLHandler handles all URL-related requests.
type URLHandler struct {
	DB          *sql.DB
	Redis       *redis.Client
	FrontendURL string
}

var rateLimitScript = redis.NewScript(`
	local key = KEYS[1]
	local now = tonumber(ARGV[1])
	local window = tonumber(ARGV[2])
	local limit = tonumber(ARGV[3])
	local clearBefore = now - window

	redis.call("ZREMRANGEBYSCORE", key, "-inf", clearBefore)
	local count = redis.call("ZCARD", key)
	if count < limit then
		local seq = redis.call("INCR", key .. ":seq")
		redis.call("ZADD", key, now, seq)
		redis.call("EXPIRE", key, window)
		redis.call("EXPIRE", key .. ":seq", window)
		return count + 1
	else
		return count + 1
	end
`)

// CreateURL handles POST /api/shorten
// @Summary      Create a shortened URL
// @Description  Accepts a long URL, validates the schema, generates a unique Base62 key, and stores it in the database and cache.
// @Tags         URLs
// @Accept       json
// @Produce      json
// @Param        request body ShortenRequest true "Long URL Payload"
// @Success      200  {object}  ShortenResponse
// @Failure      400  {object}  map[string]string "Invalid URL format or scheme"
// @Failure      429  {object}  map[string]string "Too Many Requests"
// @Failure      500  {object}  map[string]string "Internal Server Error"
// @Router       /api/shorten [post]
func (h *URLHandler) CreateURL(c *gin.Context) {
	// IP-based Rate Limiting (Atomic Lua script)
	ip := GetClientIP(c)
	rateLimitKey := "rate_limit:shorten:" + ip
	
	now := float64(time.Now().UnixNano()) / 1e9
	window := 60
	limit := 10
	count, err := rateLimitScript.Run(c.Request.Context(), h.Redis, []string{rateLimitKey}, now, window, limit).Int64()
	if err != nil {
		slog.Error("redis rate limit error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	if count > int64(limit) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too Many Requests"})
		return
	}


	var req ShortenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or missing URL"})
		return
	}

	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL must start with http:// or https://"})
		return
	}

	// Prevent self-referential URL shortening (fast string checks)
	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid URL format"})
		return
	}
	inputHost := strings.ToLower(parsedURL.Hostname())
	apiHost := c.Request.Host
	if ah, _, err := net.SplitHostPort(c.Request.Host); err == nil {
		apiHost = ah
	}
	if inputHost == strings.ToLower(apiHost) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot shorten URLs pointing to this service"})
		return
	}
	if h.FrontendURL != "" {
		if parsedFrontend, err := url.Parse(h.FrontendURL); err == nil {
			if inputHost == strings.ToLower(parsedFrontend.Hostname()) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot shorten URLs pointing to this service"})
				return
			}
		}
	}

	// Resolve destination host IPs and reject loopback, private subnets, and self-referential addresses
	if isSSRFOrSelfReferential(req.URL, c.Request.Host, h.FrontendURL) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot shorten loopback, private, or self-referential URLs"})
		return
	}

	var shortKey string
	var expiresAt *time.Time

	if req.TTLHours > 0 {
		exp := time.Now().UTC().Add(time.Duration(req.TTLHours) * time.Hour)
		expiresAt = &exp
	}

	if req.Alias != "" {
		alias := strings.TrimSpace(req.Alias)
		// Validate alias format
		if len(alias) < 3 || len(alias) > 15 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Alias must be between 3 and 15 characters long"})
			return
		}
		for _, r := range alias {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Alias can only contain alphanumeric characters, dashes, and underscores"})
				return
			}
		}

		// Check reserved words
		blacklist := map[string]bool{
			"api": true, "health": true, "dashboard": true, "analytics": true,
			"architecture": true, "swagger": true, "expired": true,
		}
		if blacklist[strings.ToLower(alias)] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Alias is a reserved system route name"})
			return
		}

		// Check uniqueness
		var exists bool
		err = h.DB.QueryRowContext(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM urls WHERE short_key = $1)`, alias).Scan(&exists)
		if err != nil {
			slog.Error("failed to check alias uniqueness", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}
		if exists {
			c.JSON(http.StatusConflict, gin.H{"error": "Alias is already taken"})
			return
		}

		shortKey = alias

		_, err = h.DB.ExecContext(c.Request.Context(), `
			INSERT INTO urls (short_key, long_url, expires_at) 
			VALUES ($1, $2, $3)`, shortKey, req.URL, expiresAt)
		if err != nil {
			slog.Error("failed to insert url with alias", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save URL"})
			return
		}
	} else {
		var nextID int64
		err = h.DB.QueryRowContext(c.Request.Context(), `SELECT nextval('urls_id_seq')`).Scan(&nextID)
		if err != nil {
			slog.Error("failed to get next sequence value", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate ID"})
			return
		}

		shortKey = utils.Encode(nextID)

		_, err = h.DB.ExecContext(c.Request.Context(), `
			INSERT INTO urls (id, short_key, long_url, expires_at) 
			VALUES ($1, $2, $3, $4)`, nextID, shortKey, req.URL, expiresAt)
		if err != nil {
			slog.Error("failed to insert url", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save URL"})
			return
		}
	}

	// Cache the URL
	cacheTTL := 24 * time.Hour
	if expiresAt != nil {
		cacheTTL = expiresAt.Sub(time.Now().UTC())
	}
	if cacheTTL > 0 {
		if err := myredis.SetURLCache(c.Request.Context(), h.Redis, shortKey, req.URL, cacheTTL); err != nil {
			slog.Warn("failed to cache URL in redis", "error", err)
		}
	}

	c.JSON(http.StatusOK, ShortenResponse{
		ShortKey: shortKey,
		ShortURL: h.FrontendURL + "/" + shortKey,
	})
}

// RedirectFallback handles GET /:key when Vercel edge middleware misses the cache.
// @Summary      Redirect to destination URL
// @Description  Queries the cache and database for the redirect target. If found, logs a click analytics event asynchronously and redirects the user with HTTP 302.
// @Tags         Redirects
// @Produce      html
// @Param        key  path      string  true  "Base62 shortened URL key"
// @Success      302  "Redirect to target destination"
// @Failure      404  {object}  map[string]string "URL not found"
// @Failure      500  {object}  map[string]string "Internal Server Error"
// @Router       /{key} [get]
func (h *URLHandler) RedirectFallback(c *gin.Context) {
	key := c.Param("key")

	// 1. Check Redis cache first just in case
	longURL, err := myredis.GetURLCache(c.Request.Context(), h.Redis, key)
	if err != nil && !errors.Is(err, redis.Nil) {
		slog.Warn("redis cache read error", "error", err)
	}

	// 2. Cache miss -> query DB
	if longURL == "" {
		var id int64
		var expiresAt sql.NullTime
		err := h.DB.QueryRowContext(c.Request.Context(), `
			SELECT id, long_url, expires_at FROM urls WHERE short_key = $1`, key).Scan(&id, &longURL, &expiresAt)
		
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
				return
			}
			slog.Error("db query error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		// Check if expired
		if expiresAt.Valid && time.Now().UTC().After(expiresAt.Time) {
			c.Redirect(http.StatusFound, h.FrontendURL + "/expired?key=" + key)
			return
		}

		// Update cache with remaining TTL
		cacheTTL := 24 * time.Hour
		if expiresAt.Valid {
			cacheTTL = expiresAt.Time.Sub(time.Now().UTC())
		}
		if cacheTTL > 0 {
			myredis.SetURLCache(c.Request.Context(), h.Redis, key, longURL, cacheTTL)
		}
	}

	// 3. Queue Analytics Event
	event := AnalyticsEvent{
		ShortKey:  key,
		Referrer:  c.Request.Header.Get("Referer"),
		UserAgent: c.Request.UserAgent(),
		IPAddress: c.ClientIP(),
		Country:   c.Request.Header.Get("CF-IPCountry"), // Cloudflare/Vercel standard
		ClickedAt: time.Now().UTC(),
	}
	if eventBytes, err := json.Marshal(event); err == nil {
		if err := myredis.PushAnalyticsEvent(c.Request.Context(), h.Redis, string(eventBytes)); err != nil {
			slog.Warn("failed to push analytics event", "error", err)
		}
	}

	// 4. Redirect
	c.Redirect(http.StatusFound, longURL)
}

// GetURLs returns the most recent URLs
// @Summary      Get recent shortened URLs
// @Description  Fetches the 50 most recently created shortened URLs from the database.
// @Tags         URLs
// @Produce      json
// @Success      200  {array}   map[string]interface{}
// @Failure      500  {object}  map[string]string "Internal Server Error"
// @Router       /api/urls [get]
func (h *URLHandler) GetURLs(c *gin.Context) {
	rows, err := h.DB.QueryContext(c.Request.Context(), `
		SELECT id, short_key, long_url, created_at FROM urls ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		slog.Error("failed to list urls", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch URLs"})
		return
	}
	defer rows.Close()

	var urls []map[string]interface{}
	for rows.Next() {
		var id int64
		var shortKey, longURL string
		var createdAt time.Time
		if err := rows.Scan(&id, &shortKey, &longURL, &createdAt); err == nil {
			urls = append(urls, map[string]interface{}{
				"id":         id,
				"short_key":  shortKey,
				"long_url":   longURL,
				"created_at": createdAt.Format(time.RFC3339),
			})
		}
	}
	
	if urls == nil {
		urls = make([]map[string]interface{}, 0)
	}

	c.JSON(http.StatusOK, urls)
}

// DeleteURL deletes a shortened URL
// @Summary      Delete a shortened URL
// @Description  Removes a shortened URL mapping from both the database and Redis cache by its short key.
// @Tags         URLs
// @Param        key  path      string  true  "Base62 shortened URL key"
// @Success      200  {object}  map[string]string "Success message"
// @Failure      404  {object}  map[string]string "URL not found"
// @Failure      500  {object}  map[string]string "Internal Server Error"
// @Router       /api/urls/{key} [delete]
func (h *URLHandler) DeleteURL(c *gin.Context) {
	key := c.Param("key")

	// 1. Delete from PostgreSQL
	res, err := h.DB.ExecContext(c.Request.Context(), `DELETE FROM urls WHERE short_key = $1`, key)
	if err != nil {
		slog.Error("failed to delete url from db", "key", key, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete URL"})
		return
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		slog.Error("failed to verify deleted rows", "key", key, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
		return
	}

	// 2. Invalidate Redis Cache
	if err := myredis.DeleteURLCache(c.Request.Context(), h.Redis, key); err != nil {
		slog.Warn("failed to invalidate redis cache on delete", "key", key, "error", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "URL deleted successfully"})
}

// GetClientIP extracts client IP using trusted headers, falling back to Gin's ClientIP
func GetClientIP(c *gin.Context) string {
	if envHeader := os.Getenv("TRUSTED_IP_HEADER"); envHeader != "" {
		if val := c.GetHeader(envHeader); val != "" {
			return strings.TrimSpace(strings.Split(val, ",")[0])
		}
	}
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

// isSSRFOrSelfReferential resolves target host IPs and blocks loopback, private subnets, and self-referential addresses
func isSSRFOrSelfReferential(targetURL string, apiHost string, frontendURL string) bool {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return true
	}
	host := parsed.Hostname()

	var ips []net.IP
	if parsedIP := net.ParseIP(host); parsedIP != nil {
		ips = append(ips, parsedIP)
	} else {
		resolved, err := net.LookupIP(host)
		if err != nil {
			return true // Block if DNS fails or cannot resolve
		}
		ips = resolved
	}

	var localIPs []net.IP
	if apiHost != "" {
		var h string
		var err error
		if strings.Contains(apiHost, ":") {
			h, _, err = net.SplitHostPort(apiHost)
			if err != nil {
				h = apiHost
			}
		} else {
			h = apiHost
		}
		if local, err := net.LookupIP(h); err == nil {
			localIPs = append(localIPs, local...)
		}
	}
	if frontendURL != "" {
		if parsedFrontend, err := url.Parse(frontendURL); err == nil {
			fHost := parsedFrontend.Hostname()
			if local, err := net.LookupIP(fHost); err == nil {
				localIPs = append(localIPs, local...)
			}
		}
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
			return true
		}
		for _, localIP := range localIPs {
			if ip.Equal(localIP) {
				return true
			}
		}
	}
	return false
}

