package crawler

import (
	"context"
	"fmt"
	"io"
)

// WorkItem represents a single URL to be fetched and parsed by a worker.
type WorkItem struct {
	// URL is the absolute URL to fetch
	URL string
}

// Result represents the outcome of processing a single WorkItem.
// Workers must send exactly one Result per WorkItem, even on error.
type Result struct {
	// URL is the original requested URL (same as WorkItem.URL)
	URL string
	// FinalURL is the URL after following redirects (use this for base URL resolution)
	FinalURL string
	// Links contains the raw href strings extracted from the HTML
	Links []string
	// Err is any error that occurred during fetch or parse (nil on success)
	Err error
}

// FetchResult contains the result of an HTTP fetch operation.
type FetchResult struct {
	// Body is the response body content
	Body []byte
	// FinalURL is the URL after following redirects
	FinalURL string
	// ContentType is the Content-Type header value
	ContentType string
}

// Fetcher is the interface for fetching HTTP content.
// This abstraction allows for testing with mock implementations.
type Fetcher interface {
	// Fetch retrieves the content from the given URL.
	// Returns the fetch result (with final URL and content-type) and any error encountered.
	// The context can be used for cancellation and timeouts.
	Fetch(ctx context.Context, url string) (*FetchResult, error)
}

// Parser is the interface for parsing HTML and extracting links.
// This abstraction allows for testing with mock implementations.
type Parser interface {
	// ExtractLinks parses HTML and returns all href attributes from <a> tags.
	// Returns raw href strings exactly as they appear in the HTML.
	ExtractLinks(r io.Reader) ([]string, error)
}

// HTTPError represents an HTTP error with status code information.
type HTTPError struct {
	StatusCode int
	URL        string
}

func (e *HTTPError) Error() string {
	var msg string
	switch {
	case e.StatusCode == 404:
		msg = "not found (404)"
	case e.StatusCode >= 500 && e.StatusCode < 600:
		msg =  fmt.Sprintf("server error (%d)", e.StatusCode)
	case e.StatusCode >= 400 && e.StatusCode < 500:
		msg =  fmt.Sprintf("client error (%d)", e.StatusCode)
	case e.StatusCode >= 300 && e.StatusCode < 400:
		msg =  fmt.Sprintf("redirect not followed (%d)", e.StatusCode)
	default:
		msg =  fmt.Sprintf("HTTP error (%d)", e.StatusCode)
	}
	return fmt.Sprintf("%s: %s", e.URL, msg)
}

// Category returns a human-readable error category.
func (e *HTTPError) Category() string {
	switch {
	case e.StatusCode == 404:
		return "dead link"
	case e.StatusCode == 408 || e.StatusCode == 504:
		return "timeout"
	case e.StatusCode >= 500 && e.StatusCode < 600:
		return "server error (retry-able)"
	default:
		return "http error"
	}
}
