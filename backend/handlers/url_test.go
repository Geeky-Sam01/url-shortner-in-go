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

func setupTestRouter(db sqlmock.Sqlmock, rClient *redis.Client) (*gin.Engine, *handlers.URLHandler, sqlmock.Sqlmock) {
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

	router, _, mock := setupTestRouter(nil, rClient)

	// Mock sequence generation
	mock.ExpectQuery(`SELECT nextval\('urls_id_seq'\)`).
		WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(56800235584))

	// Expected Base62 for 56800235584 is "1000000"
	expectedShortKey := utils.Encode(56800235584)

	// Mock insert
	mock.ExpectExec(`INSERT INTO urls`).
		WithArgs(int64(56800235584), expectedShortKey, "https://example.com").
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

	router, _, mock := setupTestRouter(nil, rClient)

	// Test case 1: Cache miss, DB hit
	shortKey := "abc1234"
	longURL := "https://github.com"

	mock.ExpectQuery(`SELECT id, long_url FROM urls WHERE short_key = \$1`).
		WithArgs(shortKey).
		WillReturnRows(sqlmock.NewRows([]string{"id", "long_url"}).AddRow(1, longURL))

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

	router, _, _ := setupTestRouter(nil, rClient)

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
}

func TestCreateURL_RateLimit(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	router, _, _ := setupTestRouter(nil, rClient)

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


