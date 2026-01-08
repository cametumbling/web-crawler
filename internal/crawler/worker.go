package crawler

import (
	"bytes"
	"fmt"
)

// worker is a stateless goroutine that processes WorkItems from workCh.
// For each WorkItem, it fetches the URL, parses the HTML, and sends exactly one Result.
// Workers never mutate shared state, never print, and never touch the WaitGroup.
func worker(workCh <-chan WorkItem, resultsCh chan<- Result, fetcher Fetcher, parser Parser) {
	for item := range workCh {
		result := processWorkItem(item, fetcher, parser)
		resultsCh <- result
	}
}

// processWorkItem handles the fetch and parse for a single WorkItem.
// Always returns a Result, even on error.
func processWorkItem(item WorkItem, fetcher Fetcher, parser Parser) Result {
	// Fetch the URL
	body, err := fetcher.Fetch(item.URL)
	if err != nil {
		return Result{
			URL:   item.URL,
			Links: nil,
			Err:   fmt.Errorf("fetch failed: %w", err),
		}
	}

	// Parse the HTML to extract links
	links, err := parser.ExtractLinks(bytes.NewReader(body))
	if err != nil {
		return Result{
			URL:   item.URL,
			Links: nil,
			Err:   fmt.Errorf("parse failed: %w", err),
		}
	}

	// Success
	return Result{
		URL:   item.URL,
		Links: links,
		Err:   nil,
	}
}
