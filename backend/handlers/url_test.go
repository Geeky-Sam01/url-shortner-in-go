package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/url-shortner/backend/handlers"
	"github.com/url-shortner/backend/utils"
)

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

func TestCreateURL(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	router, _, mock := setupTestRouter(rClient)

	// Mock sequence generation
	mock.ExpectQuery(`SELECT nextval\('urls_id_seq'\)`).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(56800235584))

	// Expected Base62 for 56800235584 is "1000000"
	expectedShortKey := utils.Encode(56800235584)

	// Mock insert
	mock.ExpectExec(`INSERT INTO urls`).
		WithArgs(int64(56800235584), expectedShortKey, "https://example.com", nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	reqBody := `{"url": "https://example.com"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %v: %s", w.Code, w.Body.String())
	}

	var resp handlers.ShortenResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.ShortKey != expectedShortKey {
		t.Errorf("expected short key %s, got %s", expectedShortKey, resp.ShortKey)
	}

	// Verify it was cached in redis
	val, err := mr.Get("url:" + expectedShortKey)
	if err != nil || val != "https://example.com" {
		t.Errorf("redis cache missing or wrong: val=%v err=%v", val, err)
	}
}

func TestRedirectFallback(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	router, _, mock := setupTestRouter(rClient)

	// Test case 1: Cache miss, DB hit
	shortKey := "abc1234"
	longURL := "https://github.com"

	mock.ExpectQuery(`SELECT id, long_url, expires_at FROM urls WHERE short_key = \$1`).
		WithArgs(shortKey).
		WillReturnRows(sqlmock.NewRows([]string{"id", "long_url", "expires_at"}).AddRow(1, longURL, nil))

	req, _ := http.NewRequest(http.MethodGet, "/"+shortKey, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %v", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != longURL {
		t.Errorf("expected location %s, got %s", longURL, loc)
	}

	// Verify cache was set
	mr.FastForward(time.Millisecond) // Ensure async push works if any
	val, _ := mr.Get("url:" + shortKey)
	if val != longURL {
		t.Errorf("expected cache to be set to %s, got %s", longURL, val)
	}

	// Verify analytics event was queued
	list, _ := mr.Lpop("analytics:queue")
	if list == "" {
		t.Errorf("expected analytics event in queue")
	}
}

func TestCreateURL_SelfReferential(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	router, _, _ := setupTestRouter(rClient)

	t.Run("API Host Check", func(t *testing.T) {
		reqBody := `{"url": "http://localhost:8080/abc"}`
		req, _ := http.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Host = "localhost:8080"

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Frontend Host Check", func(t *testing.T) {
		reqBody := `{"url": "http://localhost:4200/dashboard"}`
		req, _ := http.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Loopback IP Check", func(t *testing.T) {
		reqBody := `{"url": "http://127.0.0.1/metadata"}`
		req, _ := http.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Metadata IP Check", func(t *testing.T) {
		reqBody := `{"url": "http://169.254.169.254/latest/meta-data"}`
		req, _ := http.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Private Subnet IP Check", func(t *testing.T) {
		reqBody := `{"url": "http://192.168.1.50/admin"}`
		req, _ := http.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestCreateURL_RateLimit(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	router, _, _ := setupTestRouter(rClient)

	// Make 10 requests, all should get 400 Bad Request (not 429)
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(`invalid json`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("unexpected rate limit hit at request %d", i+1)
		}
	}

	// 11th request should get 429 Too Many Requests
	req, _ := http.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(`invalid json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 Too Many Requests on 11th request, got %d: %s", w.Code, w.Body.String())
	}
}
func TestGetClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("TRUSTED_IP_HEADER from env", func(t *testing.T) {
		t.Setenv("TRUSTED_IP_HEADER", "CF-Connecting-IP")
		
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("CF-Connecting-IP", "203.0.113.195, 70.41.3.18")
		c.Request = req

		ip := handlers.GetClientIP(c)
		if ip != "203.0.113.195" {
			t.Errorf("expected 203.0.113.195, got %s", ip)
		}
	})

	t.Run("TRUSTED_IP_HEADER from request header fallback", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("TRUSTED_IP_HEADER", "X-Custom-IP")
		req.Header.Set("X-Custom-IP", "198.51.100.1")
		c.Request = req

		ip := handlers.GetClientIP(c)
		if ip != "198.51.100.1" {
			t.Errorf("expected 198.51.100.1, got %s", ip)
		}
	})

	t.Run("X-Real-IP header", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Real-IP", "198.51.100.42")
		c.Request = req

		ip := handlers.GetClientIP(c)
		if ip != "198.51.100.42" {
			t.Errorf("expected 198.51.100.42, got %s", ip)
		}
	})

	t.Run("X-Forwarded-For header", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-For", "198.51.100.99, 10.0.0.1")
		c.Request = req

		ip := handlers.GetClientIP(c)
		if ip != "198.51.100.99" {
			t.Errorf("expected 198.51.100.99, got %s", ip)
		}
	})

	t.Run("c.ClientIP fallback", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.50:12345"
		c.Request = req

		ip := handlers.GetClientIP(c)
		if ip != "203.0.113.50" {
			t.Errorf("expected 203.0.113.50, got %s", ip)
		}
	})
}
