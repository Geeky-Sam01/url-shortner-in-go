package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
)

func TestLiveServerShorten(t *testing.T) {
	// Determine the base URL.
	// First check an environment variable, otherwise default to http://localhost:8081.
	baseURL := os.Getenv("TEST_SERVER_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8081"
	}

	// 1. Check if the server is running by doing a health check.
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Skipf("Skipping live server test: server is not reachable at %s. Error: %v", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Server at %s returned unhealthy status code: %d", baseURL, resp.StatusCode)
	}

	// 2. Perform the shorten request.
	targetURL := "https://example.com/live-test-" + string(os.Getenv("ENV"))
	if os.Getenv("ENV") == "" {
		targetURL = "https://example.com/live-test-local"
	}
	reqBody, err := json.Marshal(map[string]string{
		"url": targetURL,
	})
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	shortenURL := baseURL + "/api/shorten"
	respShorten, err := http.Post(shortenURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		t.Fatalf("Failed to send POST to %s: %v", shortenURL, err)
	}
	defer respShorten.Body.Close()

	bodyBytes, err := io.ReadAll(respShorten.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if respShorten.StatusCode != http.StatusOK {
		t.Fatalf("Expected status code 200, got %d. Response: %s", respShorten.StatusCode, string(bodyBytes))
	}

	// 3. Parse and validate the response
	var responseData struct {
		ShortKey string `json:"short_key"`
		ShortURL string `json:"short_url"`
	}

	if err := json.Unmarshal(bodyBytes, &responseData); err != nil {
		t.Fatalf("Failed to parse JSON response: %v. Response: %s", err, string(bodyBytes))
	}

	if responseData.ShortKey == "" {
		t.Error("Expected a non-empty short_key in response")
	}

	if responseData.ShortURL == "" {
		t.Error("Expected a non-empty short_url in response")
	}

	t.Logf("Successfully shortened URL. Key: %s, Short URL: %s", responseData.ShortKey, responseData.ShortURL)
}
