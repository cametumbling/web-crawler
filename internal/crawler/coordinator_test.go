package crawler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestNewCoordinator_ValidatesStartURL(t *testing.T) {
	tests := []struct {
		name      string
		startURL  string
		wantError bool
	}{
		{
			name:      "valid http URL",
			startURL:  "http://example.com/",
			wantError: false,
		},
		{
			name:      "valid https URL",
			startURL:  "https://example.com/",
			wantError: false,
		},
		{
			name:      "invalid URL",
			startURL:  "://invalid",
			wantError: true,
		},
		{
			name:      "ftp scheme rejected",
			startURL:  "ftp://example.com/",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				StartURL:   tt.startURL,
				NumWorkers: 1,
				Fetcher:    &mockFetcher{responses: make(map[string][]byte)},
				Parser:     &mockParser{},
			}

			_, err := NewCoordinator(cfg)
			if (err != nil) != tt.wantError {
				t.Errorf("NewCoordinator() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestCoordinator_SinglePage(t *testing.T) {
	output := &bytes.Buffer{}
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/": []byte("<html><body>No links</body></html>"),
		},
	}
	parser := &mockParser{links: []string{}}

	cfg := Config{
		StartURL:   "https://example.com/",
		NumWorkers: 1,
		Fetcher:    fetcher,
		Parser:     parser,
		Output:     output,
	}

	coord, err := NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	err = coord.Crawl(context.Background())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	// Check output
	out := output.String()
	if !strings.Contains(out, "Visited: https://example.com/") {
		t.Errorf("output missing visited URL: %s", out)
	}
	if !strings.Contains(out, "Links found:") {
		t.Errorf("output missing 'Links found:': %s", out)
	}
}

func TestCoordinator_FollowsInScopeLinks(t *testing.T) {
	output := &bytes.Buffer{}
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/":     []byte("<html>page1</html>"),
			"https://example.com/page2": []byte("<html>page2</html>"),
		},
	}

	// Parser will return different links based on what's fetched
	// For the first call (page1), return /page2
	// For the second call (page2), return nothing
	callCount := 0
	parser := &mockParser{
		fn: func(r io.Reader) ([]string, error) {
			callCount++
			if callCount == 1 {
				return []string{"/page2"}, nil
			}
			return []string{}, nil
		},
	}

	cfg := Config{
		StartURL:   "https://example.com/",
		NumWorkers: 1,
		Fetcher:    fetcher,
		Parser:     parser,
		Output:     output,
	}

	coord, err := NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	err = coord.Crawl(context.Background())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "Visited: https://example.com/") {
		t.Errorf("output missing first page")
	}
	if !strings.Contains(out, "Visited: https://example.com/page2") {
		t.Errorf("output missing second page")
	}
}

func TestCoordinator_DeduplicatesURLs(t *testing.T) {
	output := &bytes.Buffer{}
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/":     []byte("<html>page1</html>"),
			"https://example.com/page": []byte("<html>page2</html>"),
		},
	}

	// Both pages link to each other (cycle)
	callCount := 0
	parser := &mockParser{
		fn: func(r io.Reader) ([]string, error) {
			callCount++
			if callCount == 1 {
				// First page links to /page
				return []string{"/page"}, nil
			}
			// Second page links back to / (should be deduplicated)
			return []string{"/"}, nil
		},
	}

	cfg := Config{
		StartURL:   "https://example.com/",
		NumWorkers: 1,
		Fetcher:    fetcher,
		Parser:     parser,
		Output:     output,
	}

	coord, err := NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	err = coord.Crawl(context.Background())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	// Count how many times we visited each page
	out := output.String()
	// Need to use newline to avoid substring matches
	rootCount := strings.Count(out, "Visited: https://example.com/\n")
	pageCount := strings.Count(out, "Visited: https://example.com/page\n")

	// Debug: print output
	if rootCount != 1 || pageCount != 1 {
		t.Logf("Output:\n%s", out)
	}

	// Each page should only be visited once (second link is deduplicated)
	if rootCount != 1 {
		t.Errorf("visited root %d times, want 1", rootCount)
	}
	if pageCount != 1 {
		t.Errorf("visited /page %d times, want 1", pageCount)
	}
}

func TestCoordinator_RespectsScope(t *testing.T) {
	output := &bytes.Buffer{}
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/":     []byte("<html>page1</html>"),
			"https://example.com/page": []byte("<html>page2</html>"),
		},
	}

	callCount := 0
	parser := &mockParser{
		fn: func(r io.Reader) ([]string, error) {
			callCount++
			if callCount == 1 {
				// First page links to in-scope and out-of-scope
				return []string{
					"/page",                        // in-scope
					"https://external.com/page",    // out-of-scope (different host)
					"https://sub.example.com/page", // out-of-scope (subdomain)
				}, nil
			}
			// Second page has no links
			return []string{}, nil
		},
	}

	cfg := Config{
		StartURL:   "https://example.com/",
		NumWorkers: 1,
		Fetcher:    fetcher,
		Parser:     parser,
		Output:     output,
	}

	coord, err := NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	err = coord.Crawl(context.Background())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	out := output.String()

	// Should visit start URL and /page (both in-scope), but not external or subdomain
	visitCount := strings.Count(out, "Visited:")
	if visitCount != 2 {
		t.Errorf("visited %d pages, want 2 (in-scope only)", visitCount)
	}

	// Should visit both in-scope pages
	if !strings.Contains(out, "Visited: https://example.com/") {
		t.Errorf("output missing start URL visit")
	}
	if !strings.Contains(out, "Visited: https://example.com/page") {
		t.Errorf("output missing /page visit")
	}

	// But should print all sanitized links from first page (including out-of-scope)
	if !strings.Contains(out, "https://example.com/page") {
		t.Errorf("output missing in-scope link")
	}
	if !strings.Contains(out, "https://external.com/page") {
		t.Errorf("output missing out-of-scope link (should be printed but not visited)")
	}

	// Should NOT visit external or subdomain (verify by checking visitCount is 2, not 3 or 4)
}

func TestCoordinator_RespectsMaxPages(t *testing.T) {
	output := &bytes.Buffer{}
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/":      []byte("<html>page1</html>"),
			"https://example.com/page2": []byte("<html>page2</html>"),
			"https://example.com/page3": []byte("<html>page3</html>"),
		},
	}

	callCount := 0
	parser := &mockParser{
		fn: func(r io.Reader) ([]string, error) {
			callCount++
			if callCount == 1 {
				return []string{"/page2", "/page3"}, nil
			}
			return []string{}, nil
		},
	}

	cfg := Config{
		StartURL:   "https://example.com/",
		MaxPages:   2, // Only visit 2 pages
		NumWorkers: 1,
		Fetcher:    fetcher,
		Parser:     parser,
		Output:     output,
	}

	coord, err := NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	err = coord.Crawl(context.Background())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	out := output.String()
	visitCount := strings.Count(out, "Visited:")
	if visitCount != 2 {
		t.Errorf("visited %d pages, want 2 (maxPages limit)", visitCount)
	}
}

func TestCoordinator_HandlesErrors(t *testing.T) {
	output := &bytes.Buffer{}
	fetcher := &mockFetcher{
		errors: map[string]error{
			"https://example.com/": errors.New("connection refused"),
		},
	}
	parser := &mockParser{}

	cfg := Config{
		StartURL:   "https://example.com/",
		NumWorkers: 1,
		Fetcher:    fetcher,
		Parser:     parser,
		Output:     output,
	}

	coord, err := NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	err = coord.Crawl(context.Background())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	// Should still print the visited URL even on error
	out := output.String()
	if !strings.Contains(out, "Visited: https://example.com/") {
		t.Errorf("output missing visited URL on error")
	}
	if !strings.Contains(out, "Links found:") {
		t.Errorf("output missing 'Links found:' on error")
	}
}

func TestCoordinator_ConcurrentWorkers(t *testing.T) {
	output := &bytes.Buffer{}
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/":      []byte("<html>page1</html>"),
			"https://example.com/page2": []byte("<html>page2</html>"),
			"https://example.com/page3": []byte("<html>page3</html>"),
		},
	}

	var mu sync.Mutex
	callCount := 0
	parser := &mockParser{
		fn: func(r io.Reader) ([]string, error) {
			mu.Lock()
			callCount++
			count := callCount
			mu.Unlock()

			if count == 1 {
				return []string{"/page2", "/page3"}, nil
			}
			return []string{}, nil
		},
	}

	cfg := Config{
		StartURL:   "https://example.com/",
		NumWorkers: 3, // Multiple workers
		Fetcher:    fetcher,
		Parser:     parser,
		Output:     output,
	}

	coord, err := NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	err = coord.Crawl(context.Background())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	// Should visit all 3 pages
	out := output.String()
	visitCount := strings.Count(out, "Visited:")
	if visitCount != 3 {
		t.Errorf("visited %d pages, want 3", visitCount)
	}
}

func TestCoordinator_NormalizesStartURL(t *testing.T) {
	output := &bytes.Buffer{}
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/": []byte("<html>page</html>"),
		},
	}
	parser := &mockParser{links: []string{}}

	cfg := Config{
		StartURL:   "HTTPS://EXAMPLE.COM:443/#fragment", // Should normalize
		NumWorkers: 1,
		Fetcher:    fetcher,
		Parser:     parser,
		Output:     output,
	}

	coord, err := NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	// Check that start URL was normalized
	if coord.startURL.String() != "https://example.com/" {
		t.Errorf("startURL = %q, want %q", coord.startURL.String(), "https://example.com/")
	}

	err = coord.Crawl(context.Background())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "Visited: https://example.com/") {
		t.Errorf("output has wrong normalized URL: %s", out)
	}
}
