package crawler

import (
	"context"
	"errors"
	"io"
	"testing"
)

// mockFetcher is a mock implementation of the Fetcher interface for testing.
type mockFetcher struct {
	responses    map[string][]byte
	errors       map[string]error
	contentTypes map[string]string // Optional content types per URL
	finalURLs    map[string]string // Optional redirected URLs
}

func (m *mockFetcher) Fetch(ctx context.Context, url string) (*FetchResult, error) {
	if err, ok := m.errors[url]; ok {
		return nil, err
	}
	if body, ok := m.responses[url]; ok {
		finalURL := url
		if fu, ok := m.finalURLs[url]; ok {
			finalURL = fu
		}
		contentType := "text/html"
		if ct, ok := m.contentTypes[url]; ok {
			contentType = ct
		}
		return &FetchResult{
			Body:        body,
			FinalURL:    finalURL,
			ContentType: contentType,
		}, nil
	}
	return nil, errors.New("url not found in mock")
}

// mockParser is a mock implementation of the Parser interface for testing.
type mockParser struct {
	links []string
	err   error
	// fn is an optional callback for custom behavior
	fn func(io.Reader) ([]string, error)
}

func (m *mockParser) ExtractLinks(r io.Reader) ([]string, error) {
	if m.fn != nil {
		return m.fn(r)
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.links, nil
}

func TestProcessWorkItem_Success(t *testing.T) {
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/page": []byte("<html><body><a href='/link1'>Link</a></body></html>"),
		},
	}
	parser := &mockParser{
		links: []string{"/link1", "/link2"},
	}

	item := WorkItem{URL: "https://example.com/page"}
	result := processWorkItem(context.Background(), item, fetcher, parser)

	if result.URL != "https://example.com/page" {
		t.Errorf("Result.URL = %q, want %q", result.URL, "https://example.com/page")
	}
	if result.Err != nil {
		t.Errorf("Result.Err = %v, want nil", result.Err)
	}
	if len(result.Links) != 2 {
		t.Errorf("len(Result.Links) = %d, want 2", len(result.Links))
	}
	if result.Links[0] != "/link1" || result.Links[1] != "/link2" {
		t.Errorf("Result.Links = %v, want [/link1 /link2]", result.Links)
	}
}

func TestProcessWorkItem_FetchError(t *testing.T) {
	fetcher := &mockFetcher{
		errors: map[string]error{
			"https://example.com/error": errors.New("connection refused"),
		},
	}
	parser := &mockParser{
		links: []string{"/link1"},
	}

	item := WorkItem{URL: "https://example.com/error"}
	result := processWorkItem(context.Background(), item, fetcher, parser)

	if result.URL != "https://example.com/error" {
		t.Errorf("Result.URL = %q, want %q", result.URL, "https://example.com/error")
	}
	if result.Err == nil {
		t.Errorf("Result.Err = nil, want error")
	}
	if result.Links != nil {
		t.Errorf("Result.Links = %v, want nil", result.Links)
	}
}

func TestProcessWorkItem_ParseError(t *testing.T) {
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/page": []byte("<html>...</html>"),
		},
	}
	parser := &mockParser{
		err: errors.New("parse error"),
	}

	item := WorkItem{URL: "https://example.com/page"}
	result := processWorkItem(context.Background(), item, fetcher, parser)

	if result.URL != "https://example.com/page" {
		t.Errorf("Result.URL = %q, want %q", result.URL, "https://example.com/page")
	}
	if result.Err == nil {
		t.Errorf("Result.Err = nil, want error")
	}
	if result.Links != nil {
		t.Errorf("Result.Links = %v, want nil", result.Links)
	}
}

func TestProcessWorkItem_EmptyLinks(t *testing.T) {
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/page": []byte("<html><body>No links</body></html>"),
		},
	}
	parser := &mockParser{
		links: []string{}, // No links
	}

	item := WorkItem{URL: "https://example.com/page"}
	result := processWorkItem(context.Background(), item, fetcher, parser)

	if result.URL != "https://example.com/page" {
		t.Errorf("Result.URL = %q, want %q", result.URL, "https://example.com/page")
	}
	if result.Err != nil {
		t.Errorf("Result.Err = %v, want nil", result.Err)
	}
	if len(result.Links) != 0 {
		t.Errorf("len(result.Links) = %d, want 0", len(result.Links))
	}
}

func TestWorker_ProcessesMultipleItems(t *testing.T) {
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/page1": []byte("<html>page1</html>"),
			"https://example.com/page2": []byte("<html>page2</html>"),
			"https://example.com/page3": []byte("<html>page3</html>"),
		},
	}
	parser := &mockParser{
		links: []string{"/link"},
	}

	workCh := make(chan WorkItem, 3)
	resultsCh := make(chan Result, 3)

	// Start worker
	go worker(context.Background(), workCh, resultsCh, fetcher, parser)

	// Send work items
	workCh <- WorkItem{URL: "https://example.com/page1"}
	workCh <- WorkItem{URL: "https://example.com/page2"}
	workCh <- WorkItem{URL: "https://example.com/page3"}
	close(workCh)

	// Collect results
	results := make([]Result, 0, 3)
	for i := 0; i < 3; i++ {
		results = append(results, <-resultsCh)
	}

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// Verify all URLs were processed
	urls := make(map[string]bool)
	for _, r := range results {
		urls[r.URL] = true
		if r.Err != nil {
			t.Errorf("Result for %s has error: %v", r.URL, r.Err)
		}
	}

	expectedURLs := []string{
		"https://example.com/page1",
		"https://example.com/page2",
		"https://example.com/page3",
	}
	for _, url := range expectedURLs {
		if !urls[url] {
			t.Errorf("URL %s not found in results", url)
		}
	}
}

func TestWorker_MixedSuccessAndErrors(t *testing.T) {
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/success": []byte("<html>ok</html>"),
		},
		errors: map[string]error{
			"https://example.com/error": errors.New("fetch failed"),
		},
	}
	parser := &mockParser{
		links: []string{"/link"},
	}

	workCh := make(chan WorkItem, 2)
	resultsCh := make(chan Result, 2)

	// Start worker
	go worker(context.Background(), workCh, resultsCh, fetcher, parser)

	// Send work items
	workCh <- WorkItem{URL: "https://example.com/success"}
	workCh <- WorkItem{URL: "https://example.com/error"}
	close(workCh)

	// Collect results
	results := make(map[string]Result)
	for i := 0; i < 2; i++ {
		r := <-resultsCh
		results[r.URL] = r
	}

	// Check success case
	if r, ok := results["https://example.com/success"]; !ok {
		t.Error("success result not found")
	} else if r.Err != nil {
		t.Errorf("success result has error: %v", r.Err)
	}

	// Check error case
	if r, ok := results["https://example.com/error"]; !ok {
		t.Error("error result not found")
	} else if r.Err == nil {
		t.Error("error result should have error")
	}
}

func TestWorker_AlwaysSendsOneResultPerItem(t *testing.T) {
	// Test that worker sends exactly one result per work item, even on error
	fetcher := &mockFetcher{
		errors: map[string]error{
			"https://example.com/error1": errors.New("error 1"),
			"https://example.com/error2": errors.New("error 2"),
		},
	}
	parser := &mockParser{
		links: []string{},
	}

	workCh := make(chan WorkItem, 2)
	resultsCh := make(chan Result, 2)

	// Start worker
	go worker(context.Background(), workCh, resultsCh, fetcher, parser)

	// Send work items that will fail
	workCh <- WorkItem{URL: "https://example.com/error1"}
	workCh <- WorkItem{URL: "https://example.com/error2"}
	close(workCh)

	// Should receive exactly 2 results
	count := 0
	for i := 0; i < 2; i++ {
		r := <-resultsCh
		count++
		if r.Err == nil {
			t.Errorf("expected error in result for %s", r.URL)
		}
	}

	if count != 2 {
		t.Errorf("received %d results, want 2", count)
	}
}

// panicFetcher is a Fetcher that always panics
type panicFetcher struct{}

func (p *panicFetcher) Fetch(ctx context.Context, url string) (*FetchResult, error) {
	panic("fetcher panic!")
}

func TestWorker_RecoverFromFetcherPanic(t *testing.T) {
	// Test that worker recovers from fetcher panic and sends error Result
	fetcher := &panicFetcher{}
	parser := &mockParser{links: []string{}}

	workCh := make(chan WorkItem, 1)
	resultsCh := make(chan Result, 1)

	// Start worker
	go worker(context.Background(), workCh, resultsCh, fetcher, parser)

	// Send work item that will cause panic
	workCh <- WorkItem{URL: "https://example.com/panic"}
	close(workCh)

	// Should receive exactly one Result with error
	result := <-resultsCh

	if result.URL != "https://example.com/panic" {
		t.Errorf("Result.URL = %q, want %q", result.URL, "https://example.com/panic")
	}
	if result.Err == nil {
		t.Error("Result.Err = nil, want error from panic")
	}
	if result.Links != nil {
		t.Errorf("Result.Links = %v, want nil", result.Links)
	}
}

func TestWorker_RecoverFromParserPanic(t *testing.T) {
	// Test that worker recovers from parser panic and sends error Result
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/page": []byte("<html>test</html>"),
		},
	}

	parser := &mockParser{
		fn: func(r io.Reader) ([]string, error) {
			panic("parser panic!")
		},
	}

	workCh := make(chan WorkItem, 1)
	resultsCh := make(chan Result, 1)

	// Start worker
	go worker(context.Background(), workCh, resultsCh, fetcher, parser)

	// Send work item that will cause parser to panic
	workCh <- WorkItem{URL: "https://example.com/page"}
	close(workCh)

	// Should receive exactly one Result with error
	result := <-resultsCh

	if result.URL != "https://example.com/page" {
		t.Errorf("Result.URL = %q, want %q", result.URL, "https://example.com/page")
	}
	if result.Err == nil {
		t.Error("Result.Err = nil, want error from panic")
	}
	if result.Links != nil {
		t.Errorf("Result.Links = %v, want nil", result.Links)
	}
}

func TestWorker_ContinuesAfterPanic(t *testing.T) {
	// Test that worker continues processing after recovering from panic
	callCount := 0
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/page1": []byte("<html>page1</html>"),
			"https://example.com/page2": []byte("<html>page2</html>"),
			"https://example.com/page3": []byte("<html>page3</html>"),
		},
	}

	parser := &mockParser{
		fn: func(r io.Reader) ([]string, error) {
			callCount++
			if callCount == 2 {
				// Second call panics
				panic("parser panic on second call!")
			}
			return []string{"/link"}, nil
		},
	}

	workCh := make(chan WorkItem, 3)
	resultsCh := make(chan Result, 3)

	// Start worker
	go worker(context.Background(), workCh, resultsCh, fetcher, parser)

	// Send 3 work items (second one will panic)
	workCh <- WorkItem{URL: "https://example.com/page1"}
	workCh <- WorkItem{URL: "https://example.com/page2"}
	workCh <- WorkItem{URL: "https://example.com/page3"}
	close(workCh)

	// Should receive exactly 3 results
	results := make([]Result, 0, 3)
	for i := 0; i < 3; i++ {
		results = append(results, <-resultsCh)
	}

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// First result should succeed
	if results[0].Err != nil {
		t.Errorf("result 0 has error: %v", results[0].Err)
	}

	// Second result should have panic error
	if results[1].Err == nil {
		t.Error("result 1 should have panic error")
	}

	// Third result should succeed (worker recovered and continued)
	if results[2].Err != nil {
		t.Errorf("result 2 has error (worker didn't recover): %v", results[2].Err)
	}
}

func TestProcessWorkItem_HandlesRedirect(t *testing.T) {
	// Test that redirected URL is captured as FinalURL
	fetcher := &mockFetcher{
		responses: map[string][]byte{
			"https://example.com/old": []byte("<html><body><a href='/new-link'>Link</a></body></html>"),
		},
		finalURLs: map[string]string{
			"https://example.com/old": "https://example.com/new",
		},
	}
	parser := &mockParser{
		links: []string{"/new-link"},
	}

	item := WorkItem{URL: "https://example.com/old"}
	result := processWorkItem(context.Background(), item, fetcher, parser)

	if result.URL != "https://example.com/old" {
		t.Errorf("Result.URL = %q, want %q", result.URL, "https://example.com/old")
	}
	if result.FinalURL != "https://example.com/new" {
		t.Errorf("Result.FinalURL = %q, want %q", result.FinalURL, "https://example.com/new")
	}
	if result.Err != nil {
		t.Errorf("Result.Err = %v, want nil", result.Err)
	}
	if len(result.Links) != 1 || result.Links[0] != "/new-link" {
		t.Errorf("Result.Links = %v, want [/new-link]", result.Links)
	}
}

func TestProcessWorkItem_NonHTMLContent(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
	}{
		{"PDF", "application/pdf"},
		{"JPEG", "image/jpeg"},
		{"PNG", "image/png"},
		{"JSON", "application/json"},
		{"Plain text", "text/plain"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := &mockFetcher{
				responses: map[string][]byte{
					"https://example.com/file": []byte("binary data or whatever"),
				},
				contentTypes: map[string]string{
					"https://example.com/file": tt.contentType,
				},
			}
			parser := &mockParser{
				links: []string{"/should-not-be-called"},
			}

			item := WorkItem{URL: "https://example.com/file"}
			result := processWorkItem(context.Background(), item, fetcher, parser)

			if result.URL != "https://example.com/file" {
				t.Errorf("Result.URL = %q, want %q", result.URL, "https://example.com/file")
			}
			if result.FinalURL != "https://example.com/file" {
				t.Errorf("Result.FinalURL = %q, want %q", result.FinalURL, "https://example.com/file")
			}
			if result.Err != nil {
				t.Errorf("Result.Err = %v, want nil (non-HTML is not an error)", result.Err)
			}
			if result.Links == nil {
				t.Error("Result.Links = nil, want empty slice")
			}
			if len(result.Links) != 0 {
				t.Errorf("len(Result.Links) = %d, want 0", len(result.Links))
			}
		})
	}
}

func TestProcessWorkItem_HTMLContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
	}{
		{"text/html", "text/html"},
		{"text/html with charset", "text/html; charset=utf-8"},
		{"text/html with uppercase", "TEXT/HTML"},
		{"empty (assume HTML)", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := &mockFetcher{
				responses: map[string][]byte{
					"https://example.com/page": []byte("<html><body><a href='/link'>Link</a></body></html>"),
				},
				contentTypes: map[string]string{
					"https://example.com/page": tt.contentType,
				},
			}
			parser := &mockParser{
				links: []string{"/link"},
			}

			item := WorkItem{URL: "https://example.com/page"}
			result := processWorkItem(context.Background(), item, fetcher, parser)

			if result.Err != nil {
				t.Errorf("Result.Err = %v, want nil", result.Err)
			}
			if len(result.Links) != 1 {
				t.Errorf("len(Result.Links) = %d, want 1", len(result.Links))
			}
		})
	}
}
