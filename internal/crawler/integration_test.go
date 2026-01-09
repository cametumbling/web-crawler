package crawler_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cametumbling/web-crawler/internal/crawler"
	"github.com/cametumbling/web-crawler/internal/platform/htmlparser"
	"github.com/cametumbling/web-crawler/internal/platform/httpclient"
)

// parserAdapter adapts the htmlparser package to the Parser interface.
type parserAdapter struct{}

func (p *parserAdapter) ExtractLinks(r io.Reader) ([]string, error) {
	return htmlparser.ExtractLinks(r)
}

// TestIntegration_FullCrawl tests the complete crawl flow with a real HTTP server.
// It verifies:
// - Cycle handling (pages linking to each other)
// - Relative link resolution
// - Fragment stripping
// - External link filtering (printed but not followed)
// - Redirect handling
// - Non-HTML content handling
// - Scope enforcement
// - Deduplication
// - Termination
// - Normalized output
func TestIntegration_FullCrawl(t *testing.T) {
	// Create a test server with a small site graph:
	//
	//   /  (root)
	//   ├── /page1 (links back to /, creating cycle)
	//   ├── /page2 (relative link: "page3.html")
	//   ├── /page3.html
	//   ├── /redirect -> /page1 (HTTP redirect)
	//   ├── /document.pdf (non-HTML)
	//   └── links to https://external.com/ (out of scope)
	//
	// Root also has a fragment link: /page1#section

	mux := http.NewServeMux()

	// Root page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Root</title></head>
<body>
	<a href="/page1">Page 1</a>
	<a href="/page1#section">Page 1 with fragment</a>
	<a href="/page2">Page 2</a>
	<a href="/redirect">Redirect to Page 1</a>
	<a href="/document.pdf">PDF Document</a>
	<a href="https://external.com/page">External Link</a>
	<a href="https://EXTERNAL.COM/UPPERCASE">Uppercase Host Link</a>
	<a href="https://external.com:443/with-default-port">Link with default port</a>
</body>
</html>`))
	})

	// Page 1 - links back to root (cycle) and to page 2
	mux.HandleFunc("/page1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Page 1</title></head>
<body>
	<a href="/">Back to Root</a>
	<a href="/page2">To Page 2</a>
</body>
</html>`))
	})

	// Page 2 - uses relative link
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Page 2</title></head>
<body>
	<a href="page3.html">Relative link to Page 3</a>
</body>
</html>`))
	})

	// Page 3 - end of chain
	mux.HandleFunc("/page3.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Page 3</title></head>
<body>
	<p>End of the line</p>
</body>
</html>`))
	})

	// Redirect - redirects to /page1
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/page1", http.StatusMovedPermanently)
	})

	// Non-HTML content
	mux.HandleFunc("/document.pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.4 fake pdf content"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Create real components
	client := httpclient.New(httpclient.Config{
		Timeout:     5 * time.Second,
		MaxBodySize: 1024 * 1024,
		UserAgent:   "test-crawler",
	})
	parser := &parserAdapter{}

	// Capture output
	output := &bytes.Buffer{}

	cfg := crawler.Config{
		StartURL:     server.URL + "/",
		NumWorkers:   2,
		MaxPages:     0, // unlimited
		Fetcher:      client,
		Parser:       parser,
		Output:       output,
		OutputFormat: "text",
	}

	coord, err := crawler.NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	// Run the crawl
	err = coord.Crawl(context.Background())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	result := output.String()

	// === TERMINATION ===
	// If we got here, the crawler terminated (didn't hang on cycle)
	t.Log("✓ Crawler terminated successfully")

	// === SCOPE ===
	// External link should NOT be visited
	if strings.Contains(result, "Visited: https://external.com") {
		t.Error("Crawler visited external link (out of scope)")
	}
	t.Log("✓ External links not visited")

	// === DEDUPLICATION ===
	// Count visits to each page - should be exactly once
	pages := []string{
		server.URL + "/",
		server.URL + "/page1",
		server.URL + "/page2",
		server.URL + "/page3.html",
		server.URL + "/document.pdf",
	}

	for _, page := range pages {
		count := strings.Count(result, "Visited: "+page+"\n")
		if count > 1 {
			t.Errorf("Page %s visited %d times, want 1 (deduplication failed)", page, count)
		}
	}
	t.Log("✓ No duplicate visits")

	// /redirect should NOT appear as visited (it redirects to /page1)
	// The final URL after redirect is what gets printed
	if strings.Contains(result, "Visited: "+server.URL+"/redirect") {
		t.Error("Redirect URL appeared as visited (should show final URL)")
	}
	t.Log("✓ Redirect handled correctly")

	// === NORMALIZED LINKS ===
	// Fragment should be stripped from printed links
	if strings.Contains(result, "#section") {
		t.Error("Fragment (#section) was not stripped from output")
	}
	t.Log("✓ Fragments stripped from links")

	// Uppercase host should be normalized to lowercase
	if strings.Contains(result, "EXTERNAL.COM") {
		t.Error("Uppercase host was not normalized to lowercase")
	}
	if !strings.Contains(result, "https://external.com/UPPERCASE") {
		t.Error("Uppercase host link not found in output (should be lowercase host, preserved path)")
	}
	t.Log("✓ Uppercase hosts normalized to lowercase")

	// Default port (443 for https) should be stripped
	if strings.Contains(result, ":443") {
		t.Error("Default port :443 was not stripped from output")
	}
	if !strings.Contains(result, "https://external.com/with-default-port") {
		t.Error("Link with default port not found in output (port should be stripped)")
	}
	t.Log("✓ Default ports stripped from links")

	// External link should be PRINTED (just not visited)
	if !strings.Contains(result, "https://external.com/page") {
		t.Error("External link not printed in links list")
	}
	t.Log("✓ External links printed (but not followed)")

	// Relative link should be resolved to absolute
	if !strings.Contains(result, server.URL+"/page3.html") {
		t.Error("Relative link not resolved to absolute URL")
	}
	t.Log("✓ Relative links resolved correctly")

	// === NON-HTML ===
	// PDF should be visited but have empty links
	pdfVisited := strings.Contains(result, "Visited: "+server.URL+"/document.pdf")
	if !pdfVisited {
		t.Error("PDF page not visited")
	}
	t.Log("✓ Non-HTML content visited")

	// === PAGE COUNT ===
	// Should visit exactly 5 pages: /, /page1, /page2, /page3.html, /document.pdf
	// /redirect doesn't count as separate visit (redirects to /page1)
	visitCount := strings.Count(result, "Visited: ")
	if visitCount != 5 {
		t.Errorf("Visited %d pages, want 5", visitCount)
	}
	t.Log("✓ Correct number of pages visited")

	t.Logf("\n=== Full output ===\n%s", result)
}

// TestIntegration_MaxPages verifies the max-pages cap works correctly.
func TestIntegration_MaxPages(t *testing.T) {
	mux := http.NewServeMux()

	// Create a chain of pages: / -> /page1 -> /page2 -> /page3 -> ...
	for i := 0; i < 10; i++ {
		i := i // capture
		path := "/"
		if i > 0 {
			path = "/page" + string(rune('0'+i))
		}
		nextPath := "/page" + string(rune('0'+i+1))

		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><body><a href="` + nextPath + `">Next</a></body></html>`))
		})
	}

	server := httptest.NewServer(mux)
	defer server.Close()

	client := httpclient.New(httpclient.Config{
		Timeout:     5 * time.Second,
		MaxBodySize: 1024 * 1024,
	})
	parser := &parserAdapter{}
	output := &bytes.Buffer{}

	cfg := crawler.Config{
		StartURL:     server.URL + "/",
		NumWorkers:   1,
		MaxPages:     3, // Only visit 3 pages
		Fetcher:      client,
		Parser:       parser,
		Output:       output,
		OutputFormat: "text",
	}

	coord, err := crawler.NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	err = coord.Crawl(context.Background())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	result := output.String()
	visitCount := strings.Count(result, "Visited: ")

	if visitCount != 3 {
		t.Errorf("Visited %d pages, want 3 (max-pages cap)", visitCount)
	}
}

// TestIntegration_ContextCancellation verifies graceful shutdown on context cancellation.
func TestIntegration_ContextCancellation(t *testing.T) {
	mux := http.NewServeMux()

	// Create pages that link to each other endlessly
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><a href="/page1">Next</a></body></html>`))
	})

	pageCount := 0
	mux.HandleFunc("/page1", func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		w.Header().Set("Content-Type", "text/html")
		// Link to a new page each time (infinite pages)
		w.Write([]byte(`<html><body><a href="/page` + string(rune('0'+pageCount)) + `">Next</a></body></html>`))
	})

	// Catch-all for /page2, /page3, etc.
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><a href="/page1">Back</a></body></html>`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := httpclient.New(httpclient.Config{
		Timeout:     5 * time.Second,
		MaxBodySize: 1024 * 1024,
	})
	parser := &parserAdapter{}
	output := &bytes.Buffer{}

	cfg := crawler.Config{
		StartURL:     server.URL + "/",
		NumWorkers:   1,
		MaxPages:     0, // unlimited
		Fetcher:      client,
		Parser:       parser,
		Output:       output,
		OutputFormat: "text",
	}

	coord, err := crawler.NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Run crawl in goroutine
	done := make(chan error, 1)
	go func() {
		done <- coord.Crawl(ctx)
	}()

	// Cancel after a short time
	cancel()

	// Wait for crawl to finish
	err = <-done

	// Should not error on cancellation (graceful shutdown)
	if err != nil && err != context.Canceled {
		t.Errorf("Crawl() error = %v, want nil or context.Canceled", err)
	}

	// The key assertion: it terminated (didn't hang)
	t.Log("✓ Crawler terminated gracefully on context cancellation")
}

// TestIntegration_RedirectDeduplication verifies that when /old redirects to /new,
// and we later discover a direct link to /new, we don't re-fetch /new.
func TestIntegration_RedirectDeduplication(t *testing.T) {
	fetchCount := make(map[string]int)
	var mu sync.Mutex

	mux := http.NewServeMux()

	// Root page links to /old (which redirects) and /page2
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		fetchCount["/"]++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body>
			<a href="/old">Old link (redirects)</a>
			<a href="/page2">Page 2</a>
		</body></html>`))
	})

	// /old redirects to /final
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fetchCount["/old"]++
		mu.Unlock()
		http.Redirect(w, r, "/final", http.StatusMovedPermanently)
	})

	// /final is the redirect target
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fetchCount["/final"]++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><p>Final destination</p></body></html>`))
	})

	// /page2 has a direct link to /final (which we should have already visited via redirect)
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fetchCount["/page2"]++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body>
			<a href="/final">Direct link to final</a>
		</body></html>`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := httpclient.New(httpclient.Config{
		Timeout:     5 * time.Second,
		MaxBodySize: 1024 * 1024,
	})
	parser := &parserAdapter{}
	output := &bytes.Buffer{}

	cfg := crawler.Config{
		StartURL:     server.URL + "/",
		NumWorkers:   1, // Single worker to ensure deterministic ordering
		MaxPages:     0,
		Fetcher:      client,
		Parser:       parser,
		Output:       output,
		OutputFormat: "text",
	}

	coord, err := crawler.NewCoordinator(cfg)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}

	err = coord.Crawl(context.Background())
	if err != nil {
		t.Fatalf("Crawl() error = %v", err)
	}

	// /final should only be fetched ONCE, even though:
	// 1. /old redirects to /final
	// 2. /page2 has a direct link to /final
	mu.Lock()
	finalFetchCount := fetchCount["/final"]
	mu.Unlock()

	if finalFetchCount != 1 {
		t.Errorf("/final was fetched %d times, want 1 (redirect deduplication failed)", finalFetchCount)
	}
	t.Log("✓ Redirect target not re-fetched when discovered via direct link")

	// Also verify /final only appears once in output
	result := output.String()
	visitedCount := strings.Count(result, "Visited: "+server.URL+"/final")
	if visitedCount != 1 {
		t.Errorf("/final appeared in output %d times, want 1", visitedCount)
	}
	t.Log("✓ Redirect target only printed once")
}
