package crawler

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

// worker is a stateless goroutine that processes WorkItems from workCh.
// For each WorkItem, it fetches the URL, parses the HTML, and sends exactly one Result.
// Workers never mutate shared state, never print, and never touch the WaitGroup.
// CRITICAL: Even on panic, exactly one Result must be sent to maintain termination invariant.
// Respects context cancellation for graceful shutdown.
func worker(ctx context.Context, workCh <-chan WorkItem, resultsCh chan<- Result, fetcher Fetcher, parser Parser) {
	for {
		select {
		case <-ctx.Done():
			// Context cancelled - stop processing new items
			return
		case item, ok := <-workCh:
			if !ok {
				// Channel closed - exit
				return
			}
			// Use defer/recover to ensure exactly one Result is sent even on panic
			func() {
				var result Result
				sent := false

				defer func() {
					if r := recover(); r != nil {
						// Panic occurred - send error Result if we haven't sent one yet
						if !sent {
							resultsCh <- Result{
								URL:   item.URL,
								Links: nil,
								Err:   fmt.Errorf("worker panic: %v", r),
							}
						}
					}
				}()

				// Normal processing
				result = processWorkItem(ctx, item, fetcher, parser)
				resultsCh <- result
				sent = true
			}()
		}
	}
}

// processWorkItem handles the fetch and parse for a single WorkItem.
// Always returns a Result, even on error.
// Worker is stateless - it does NOT log. Logging is done by the coordinator.
func processWorkItem(ctx context.Context, item WorkItem, fetcher Fetcher, parser Parser) Result {
	// Fetch the URL
	fetchResult, err := fetcher.Fetch(ctx, item.URL)
	if err != nil {
		return Result{
			URL:      item.URL,
			FinalURL: item.URL, // Use original URL as fallback
			Links:    nil,
			Err:      err, // Return raw error - coordinator will wrap/log
		}
	}

	// Check if content is HTML
	if !isHTML(fetchResult.ContentType) {
		// Non-HTML content: return empty links (not an error)
		return Result{
			URL:      item.URL,
			FinalURL: fetchResult.FinalURL,
			Links:    []string{}, // Empty, not nil
			Err:      nil,
		}
	}

	// Parse the HTML to extract links
	links, err := parser.ExtractLinks(bytes.NewReader(fetchResult.Body))
	if err != nil {
		return Result{
			URL:      item.URL,
			FinalURL: fetchResult.FinalURL,
			Links:    nil,
			Err:      err, // Return raw error - coordinator will log
		}
	}

	// Success
	return Result{
		URL:      item.URL,
		FinalURL: fetchResult.FinalURL,
		Links:    links,
		Err:      nil,
	}
}

// isHTML returns true if the Content-Type header indicates HTML content.
func isHTML(contentType string) bool {
	// Content-Type might be "text/html; charset=utf-8" or just "text/html"
	// Also handle empty content type (assume HTML)
	if contentType == "" {
		return true // Assume HTML if no Content-Type
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return ct == "text/html"
}
