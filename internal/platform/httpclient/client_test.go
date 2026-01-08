package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNew_Defaults(t *testing.T) {
	c := New(Config{})

	if c.userAgent != DefaultUserAgent {
		t.Errorf("userAgent = %q, want %q", c.userAgent, DefaultUserAgent)
	}
	if c.maxBodySize != DefaultMaxBodySize {
		t.Errorf("maxBodySize = %d, want %d", c.maxBodySize, DefaultMaxBodySize)
	}
	if c.httpClient.Timeout != DefaultTimeout {
		t.Errorf("timeout = %v, want %v", c.httpClient.Timeout, DefaultTimeout)
	}
	if c.rateLimiter != nil {
		t.Errorf("rateLimiter should be nil when RateLimit is 0")
	}
}

func TestNew_CustomConfig(t *testing.T) {
	cfg := Config{
		Timeout:     5 * time.Second,
		UserAgent:   "CustomBot/1.0",
		MaxBodySize: 1024,
		RateLimit:   100 * time.Millisecond,
	}
	c := New(cfg)

	if c.userAgent != "CustomBot/1.0" {
		t.Errorf("userAgent = %q, want %q", c.userAgent, "CustomBot/1.0")
	}
	if c.maxBodySize != 1024 {
		t.Errorf("maxBodySize = %d, want %d", c.maxBodySize, 1024)
	}
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want %v", c.httpClient.Timeout, 5*time.Second)
	}
	if c.rateLimiter == nil {
		t.Errorf("rateLimiter should not be nil when RateLimit > 0")
	}
}

func TestFetch_Success(t *testing.T) {
	expectedBody := "test content"
	expectedUA := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, expectedBody)
	}))
	defer server.Close()

	c := New(Config{})
	result, err := c.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if string(result.Body) != expectedBody {
		t.Errorf("Fetch() body = %q, want %q", string(result.Body), expectedBody)
	}

	if expectedUA != DefaultUserAgent {
		t.Errorf("User-Agent header = %q, want %q", expectedUA, DefaultUserAgent)
	}
}

func TestFetch_CustomUserAgent(t *testing.T) {
	expectedUA := "CustomBot/2.0"
	receivedUA := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(Config{UserAgent: expectedUA})
	_, err := c.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if receivedUA != expectedUA {
		t.Errorf("User-Agent header = %q, want %q", receivedUA, expectedUA)
	}
}

func TestFetch_Non2xxStatus(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		wantErrString string
	}{
		{"404 Not Found", http.StatusNotFound, "not found (404)"},
		{"500 Internal Server Error", http.StatusInternalServerError, "server error (500)"},
		{"403 Forbidden", http.StatusForbidden, "client error (403)"},
		{"301 Moved Permanently", http.StatusMovedPermanently, "redirect not followed (301)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			c := New(Config{})
			_, err := c.Fetch(context.Background(), server.URL)
			if err == nil {
				t.Errorf("Fetch() expected error for status %d, got nil", tt.statusCode)
			}
			if !strings.Contains(err.Error(), tt.wantErrString) {
				t.Errorf("Fetch() error = %v, want error containing %q", err, tt.wantErrString)
			}
		})
	}
}

func TestFetch_BodySizeLimit(t *testing.T) {
	// Create a large body that exceeds the limit
	largeBody := strings.Repeat("a", 2000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, largeBody)
	}))
	defer server.Close()

	// Set a small body size limit
	c := New(Config{MaxBodySize: 1000})
	result, err := c.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	// Should only read up to the limit
	if len(result.Body) != 1000 {
		t.Errorf("Fetch() body size = %d, want %d (limit)", len(result.Body), 1000)
	}
}

func TestFetch_Timeout(t *testing.T) {
	// Create a server that delays longer than the timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Set a very short timeout
	c := New(Config{Timeout: 50 * time.Millisecond})
	_, err := c.Fetch(context.Background(), server.URL)
	if err == nil {
		t.Errorf("Fetch() expected timeout error, got nil")
	}
}

func TestFetch_RateLimit(t *testing.T) {
	requestTimes := []time.Time{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestTimes = append(requestTimes, time.Now())
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Set rate limit to 100ms between requests
	c := New(Config{RateLimit: 100 * time.Millisecond})

	// Make 3 requests
	for i := 0; i < 3; i++ {
		_, err := c.Fetch(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
	}

	// Check that requests were spaced at least 100ms apart
	if len(requestTimes) < 3 {
		t.Fatalf("Expected 3 requests, got %d", len(requestTimes))
	}

	for i := 1; i < len(requestTimes); i++ {
		interval := requestTimes[i].Sub(requestTimes[i-1])
		// Allow some tolerance (95ms) for timing variations
		if interval < 95*time.Millisecond {
			t.Errorf("Request interval %d = %v, want >= 100ms", i, interval)
		}
	}
}

func TestFetch_InvalidURL(t *testing.T) {
	c := New(Config{})
	_, err := c.Fetch(context.Background(), "://invalid-url")
	if err == nil {
		t.Errorf("Fetch() expected error for invalid URL, got nil")
	}
}

func TestFetch_2xxStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"204 No Content", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				fmt.Fprint(w, "success")
			}))
			defer server.Close()

			c := New(Config{})
			_, err := c.Fetch(context.Background(), server.URL)
			if err != nil {
				t.Errorf("Fetch() unexpected error for status %d: %v", tt.statusCode, err)
			}
		})
	}
}

func TestFetch_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Don't write anything
	}))
	defer server.Close()

	c := New(Config{})
	result, err := c.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if len(result.Body) != 0 {
		t.Errorf("Fetch() body length = %d, want 0", len(result.Body))
	}
}
