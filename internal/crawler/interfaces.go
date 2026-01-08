package crawler

import (
	"context"
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
