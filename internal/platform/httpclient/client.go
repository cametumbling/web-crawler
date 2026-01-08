package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cametumbling/web-crawler/internal/crawler"
)

const (
	// DefaultTimeout is the default HTTP request timeout
	DefaultTimeout = 10 * time.Second
	// DefaultMaxBodySize is the default maximum response body size (2MB)
	DefaultMaxBodySize = 2 * 1024 * 1024
	// DefaultUserAgent is the default User-Agent header
	DefaultUserAgent = "MonzoCrawler/1.0"
)

// Client is an HTTP client with timeout, rate limiting, and body size limits.
// It is safe for concurrent use by multiple goroutines.
type Client struct {
	httpClient  *http.Client
	userAgent   string
	maxBodySize int64
	rateLimiter <-chan time.Time
}

// Config contains configuration options for the HTTP client.
type Config struct {
	// Timeout is the total request timeout (default: 10s)
	Timeout time.Duration
	// UserAgent is the User-Agent header to send (default: "MonzoCrawler/1.0")
	UserAgent string
	// MaxBodySize is the maximum response body size in bytes (default: 2MB)
	MaxBodySize int64
	// RateLimit is the minimum duration between requests (0 = no limit)
	RateLimit time.Duration
}

// New creates a new HTTP client with the given configuration.
func New(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	if cfg.MaxBodySize == 0 {
		cfg.MaxBodySize = DefaultMaxBodySize
	}

	c := &Client{
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		userAgent:   cfg.UserAgent,
		maxBodySize: cfg.MaxBodySize,
	}

	// Set up rate limiter if configured
	if cfg.RateLimit > 0 {
		c.rateLimiter = time.Tick(cfg.RateLimit)
	}

	return c
}

// Fetch retrieves the content from the given URL.
// Returns the fetch result (with final URL and content-type) and any error encountered.
// Applies rate limiting, sets User-Agent, and enforces body size limits.
// Respects context cancellation.
func (c *Client) Fetch(ctx context.Context, url string) (*crawler.FetchResult, error) {
	// Apply rate limiting if configured
	if c.rateLimiter != nil {
		select {
		case <-c.rateLimiter:
			// Rate limit satisfied, continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set User-Agent header
	req.Header.Set("User-Agent", c.userAgent)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &crawler.HTTPError{
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}

	// Read body with size limit
	limitedReader := io.LimitReader(resp.Body, c.maxBodySize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	// Get final URL after redirects
	finalURL := resp.Request.URL.String()

	// Get Content-Type header
	contentType := resp.Header.Get("Content-Type")

	return &crawler.FetchResult{
		Body:        body,
		FinalURL:    finalURL,
		ContentType: contentType,
	}, nil
}
