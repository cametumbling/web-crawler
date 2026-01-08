package crawler

import (
	"errors"
	"io"
	"testing"
)

// mockFetcher is a mock implementation of the Fetcher interface for testing.
type mockFetcher struct {
	responses map[string][]byte
	errors    map[string]error
}

func (m *mockFetcher) Fetch(url string) ([]byte, error) {
	if err, ok := m.errors[url]; ok {
		return nil, err
	}
	if body, ok := m.responses[url]; ok {
		return body, nil
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
	result := processWorkItem(item, fetcher, parser)

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
	result := processWorkItem(item, fetcher, parser)

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
	result := processWorkItem(item, fetcher, parser)

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
	result := processWorkItem(item, fetcher, parser)

	if result.URL != "https://example.com/page" {
		t.Errorf("Result.URL = %q, want %q", result.URL, "https://example.com/page")
	}
	if result.Err != nil {
		t.Errorf("Result.Err = %v, want nil", result.Err)
	}
	if len(result.Links) != 0 {
		t.Errorf("len(Result.Links) = %d, want 0", len(result.Links))
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
	go worker(workCh, resultsCh, fetcher, parser)

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
	go worker(workCh, resultsCh, fetcher, parser)

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
	go worker(workCh, resultsCh, fetcher, parser)

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
