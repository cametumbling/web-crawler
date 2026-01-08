package crawler

import "io"

// WorkItem represents a single URL to be fetched and parsed by a worker.
type WorkItem struct {
	// URL is the absolute URL to fetch
	URL string
}

// Result represents the outcome of processing a single WorkItem.
// Workers must send exactly one Result per WorkItem, even on error.
type Result struct {
	// URL is the page URL that was fetched (same as WorkItem.URL)
	URL string
	// Links contains the raw href strings extracted from the HTML
	Links []string
	// Err is any error that occurred during fetch or parse (nil on success)
	Err error
}

// Fetcher is the interface for fetching HTTP content.
// This abstraction allows for testing with mock implementations.
type Fetcher interface {
	// Fetch retrieves the content from the given URL.
	// Returns the response body and any error encountered.
	Fetch(url string) ([]byte, error)
}

// Parser is the interface for parsing HTML and extracting links.
// This abstraction allows for testing with mock implementations.
type Parser interface {
	// ExtractLinks parses HTML and returns all href attributes from <a> tags.
	// Returns raw href strings exactly as they appear in the HTML.
	ExtractLinks(r io.Reader) ([]string, error)
}
