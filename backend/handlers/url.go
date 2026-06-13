package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	myredis "github.com/url-shortner/backend/redis"
	"github.com/url-shortner/backend/utils"
)

// ShortenRequest defines the payload for creating a short URL.
type ShortenRequest struct {
	URL string `json:"url" binding:"required,url"`
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
	// IP-based Rate Limiting
	ip := c.ClientIP()
	rateLimitKey := "rate_limit:shorten:" + ip
	
	count, err := h.Redis.Incr(c.Request.Context(), rateLimitKey).Result()
	if err != nil {
		slog.Error("redis rate limit error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	if count == 1 {
		h.Redis.Expire(c.Request.Context(), rateLimitKey, time.Minute)
	}
	if count > 10 {
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

	// 1. Insert into DB to generate the sequential ID.
	// We insert 'pending' for the short_key initially because it requires UNIQUE constraint,
	// but we don't know the ID yet. Actually, since short_key is UNIQUE, inserting 'pending'
	// will fail on concurrent requests.
	// Better approach: We can use a transaction, or we can use a dummy UUID temporarily,
	// or we can insert without it if it's nullable. Since schema says NOT NULL UNIQUE,
	// we will insert the long_url and a temporary random uuid, then update it.
	
	// Wait! We can use a CTE or simply rely on RETURNING id, but we must provide a unique string.
	// A simpler way: get the next value from the sequence first.
	var nextID int64
	err = h.DB.QueryRowContext(c.Request.Context(), `SELECT nextval('urls_id_seq')`).Scan(&nextID)
	if err != nil {
		slog.Error("failed to get next sequence value", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate ID"})
		return
	}

	shortKey := utils.Encode(nextID)

	// Now insert the row with the known ID and shortKey
	_, err = h.DB.ExecContext(c.Request.Context(), `
		INSERT INTO urls (id, short_key, long_url) 
		VALUES ($1, $2, $3)`, nextID, shortKey, req.URL)
	if err != nil {
		slog.Error("failed to insert url", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save URL"})
		return
	}

	// Cache the URL
	if err := myredis.SetURLCache(c.Request.Context(), h.Redis, shortKey, req.URL, 24*time.Hour); err != nil {
		slog.Warn("failed to cache URL in redis", "error", err)
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
		err := h.DB.QueryRowContext(c.Request.Context(), `
			SELECT id, long_url FROM urls WHERE short_key = $1`, key).Scan(&id, &longURL)
		
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
				return
			}
			slog.Error("db query error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		// Update cache
		myredis.SetURLCache(c.Request.Context(), h.Redis, key, longURL, 24*time.Hour)
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

