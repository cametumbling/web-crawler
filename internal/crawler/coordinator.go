package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"sync"
	"time"
)

// Coordinator is the brain of the crawler.
// It owns the visited map, WaitGroup, and all scheduling decisions.
// It is the only component that prints to stdout.
type Coordinator struct {
	// visited tracks URLs we've already enqueued (dedupe)
	visited map[string]bool
	// wg tracks outstanding work items
	wg sync.WaitGroup
	// workCh sends work to workers
	workCh chan WorkItem
	// resultsCh receives results from workers
	resultsCh chan Result
	// fetcher is the HTTP client
	fetcher Fetcher
	// parser is the HTML parser
	parser Parser
	// startURL is the parsed starting URL
	startURL *url.URL
	// startHost is the hostname we're crawling
	startHost string
	// maxPages is the maximum number of pages to visit (0 = unlimited)
	maxPages int
	// visitCount tracks how many pages we've visited
	visitCount int
	// errorCount tracks how many pages failed to fetch/parse
	errorCount int
	// numWorkers is the number of worker goroutines
	numWorkers int
	// output is where we write results (default: os.Stdout)
	output io.Writer
	// outputFormat is the output format: "text" or "json"
	outputFormat string
}

// Config contains configuration for the Coordinator.
type Config struct {
	// StartURL is the starting URL to crawl
	StartURL string
	// MaxPages is the maximum number of pages to visit (0 = unlimited)
	MaxPages int
	// NumWorkers is the number of concurrent workers
	NumWorkers int
	// Fetcher is the HTTP client interface
	Fetcher Fetcher
	// Parser is the HTML parser interface
	Parser Parser
	// Output is where to write results (default: os.Stdout)
	Output io.Writer
	// OutputFormat is the output format: "text" or "json" (default: "text")
	OutputFormat string
}

// NewCoordinator creates a new Coordinator with the given configuration.
func NewCoordinator(cfg Config) (*Coordinator, error) {
	// Parse and validate start URL
	startURL, err := url.Parse(cfg.StartURL)
	if err != nil {
		return nil, fmt.Errorf("invalid start URL: %w", err)
	}

	if startURL.Scheme != "http" && startURL.Scheme != "https" {
		return nil, fmt.Errorf("start URL must use http or https scheme")
	}

	// Normalize the start URL
	normalizedStart, ok := Sanitize(cfg.StartURL, startURL)
	if !ok {
		return nil, fmt.Errorf("failed to normalize start URL")
	}

	// Re-parse the normalized URL
	startURL, err = url.Parse(normalizedStart)
	if err != nil {
		return nil, fmt.Errorf("failed to parse normalized start URL: %w", err)
	}

	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	outputFormat := cfg.OutputFormat
	if outputFormat == "" {
		outputFormat = "text"
	}

	// Buffer workCh to avoid deadlock when coordinator enqueues multiple URLs
	// before workers can pick them up. Buffer size is generous to handle
	// pages with many links.
	bufferSize := cfg.NumWorkers * 100
	if bufferSize < 100 {
		bufferSize = 100
	}

	return &Coordinator{
		visited:      make(map[string]bool),
		workCh:       make(chan WorkItem, bufferSize),
		resultsCh:    make(chan Result),
		fetcher:      cfg.Fetcher,
		parser:       cfg.Parser,
		startURL:     startURL,
		startHost:    startURL.Hostname(),
		maxPages:     cfg.MaxPages,
		numWorkers:   cfg.NumWorkers,
		output:       output,
		outputFormat: outputFormat,
	}, nil
}

// Crawl starts the crawl and blocks until completion.
// Respects context cancellation for graceful shutdown.
func (c *Coordinator) Crawl(ctx context.Context) error {
	startTime := time.Now()

	// Track when workers exit so we can close resultsCh
	var workerWg sync.WaitGroup

	// Seed the first URL BEFORE starting closer
	// Mark as visited and add to WaitGroup
	startKey := Key(c.startURL.String())
	c.visited[startKey] = true
	c.visitCount++
	c.wg.Add(1) // MUST happen before starting closer goroutine

	// Start workers
	for i := 0; i < c.numWorkers; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			worker(ctx, c.workCh, c.resultsCh, c.fetcher, c.parser)
		}()
	}

	// Start closer goroutine for workCh
	// It waits for all work to complete, then closes workCh
	go func() {
		c.wg.Wait()
		close(c.workCh)
	}()

	// Start closer goroutine for resultsCh
	// It waits for all workers to exit, then closes resultsCh
	go func() {
		workerWg.Wait()
		close(c.resultsCh)
	}()

	// Enqueue the first work item
	// wg.Add(1) was already called above
	select {
	case c.workCh <- WorkItem{URL: c.startURL.String()}:
		// Successfully enqueued
	case <-ctx.Done():
		// Context cancelled before we could start
		c.wg.Done()
		return ctx.Err()
	}

	// Process results until all workers are done
	c.processResults(ctx)

	// Print summary to stderr
	duration := time.Since(startTime)
	log.Printf("\n=== Crawl Summary ===")
	log.Printf("Total pages visited: %d", c.visitCount)
	log.Printf("Total errors: %d", c.errorCount)
	log.Printf("Duration: %v", duration)
	if duration.Seconds() > 0 {
		rate := float64(c.visitCount) / duration.Seconds()
		log.Printf("Rate: %.2f pages/sec", rate)
	}

	return nil
}

// processResults is the main loop that processes results from workers.
// For each result, it:
// 1. Prints the page and links
// 2. Sanitizes and filters links
// 3. Enqueues new in-scope, unvisited URLs
// 4. Calls wg.Done()
//
// This blocks until resultsCh is closed (which happens after all workers exit).
// Respects context cancellation and stops scheduling new work when cancelled.
func (c *Coordinator) processResults(ctx context.Context) {
	for result := range c.resultsCh {
		c.processResult(ctx, result)
	}
}

// processResult handles a single result from a worker.
// This is where the termination invariant is enforced.
// Stops scheduling new work if context is cancelled.
func (c *Coordinator) processResult(ctx context.Context, result Result) {
	// Handle redirects: if FinalURL differs from URL and FinalURL was already
	// visited (via a direct link), skip printing to avoid duplicates.
	// We still process the result and call wg.Done() to maintain invariant.
	finalKey := Key(result.FinalURL)
	alreadyPrinted := result.URL != result.FinalURL && c.visited[finalKey]

	// Print the page (even on error), unless it's a redirect to an already-visited page
	if !alreadyPrinted {
		c.printResult(result)
	}

	// If there was an error, log it and don't enqueue new work
	if result.Err != nil {
		c.logError(result.URL, result.Err)
		c.errorCount++
		c.wg.Done()
		return
	}

	// Check if context is cancelled - don't schedule new work
	select {
	case <-ctx.Done():
		// Context cancelled - stop scheduling new work
		c.wg.Done()
		return
	default:
		// Continue processing
	}

	// Sanitize all links (use FinalURL for base URL resolution after redirects)
	sanitized := c.sanitizeLinks(result.Links, result.FinalURL)

	// For each sanitized link, check scope and visited
	for _, link := range sanitized {
		// Check if context is cancelled before enqueueing each link
		select {
		case <-ctx.Done():
			// Context cancelled - stop scheduling new work
			c.wg.Done()
			return
		default:
			// Continue
		}

		// Check if in scope
		if !InScope(link, c.startHost) {
			continue
		}

		// Check if already visited
		linkKey := Key(link)
		if c.visited[linkKey] {
			continue
		}

		// Check max pages cap
		if c.maxPages > 0 && c.visitCount >= c.maxPages {
			continue
		}

		// Mark as visited and enqueue
		c.visited[linkKey] = true
		c.visitCount++

		// CRITICAL: wg.Add(1) BEFORE enqueuing
		c.wg.Add(1)
		c.workCh <- WorkItem{URL: link}
	}

	// CRITICAL: wg.Done() AFTER processing result and enqueuing all derived work
	c.wg.Done()
}

// sanitizeLinks sanitizes raw hrefs against the page URL.
// Returns only valid http(s) URLs.
func (c *Coordinator) sanitizeLinks(rawHrefs []string, pageURL string) []string {
	// Parse the page URL to use as base
	base, err := url.Parse(pageURL)
	if err != nil {
		// This shouldn't happen since we successfully fetched this URL
		return nil
	}

	var sanitized []string
	for _, href := range rawHrefs {
		if abs, ok := Sanitize(href, base); ok {
			sanitized = append(sanitized, abs)
		}
	}
	return sanitized
}

// PageResult represents the JSON output for a single page.
type PageResult struct {
	URL   string   `json:"url"`
	Links []string `json:"links"`
	Error string   `json:"error,omitempty"`
}

// printResult prints the result to stdout in the configured format (text or json).
func (c *Coordinator) printResult(result Result) {
	// Sanitize all links (not just in-scope ones)
	var sanitized []string
	if result.Err == nil {
		sanitized = c.sanitizeLinks(result.Links, result.FinalURL)
	}

	if c.outputFormat == "json" {
		// JSON output
		pageResult := PageResult{
			URL:   result.FinalURL,
			Links: sanitized,
		}
		if result.Err != nil {
			pageResult.Error = result.Err.Error()
		}
		if sanitized == nil {
			pageResult.Links = []string{} // Ensure empty array, not null
		}

		jsonBytes, err := json.Marshal(pageResult)
		if err != nil {
			log.Printf("Error marshaling JSON: %v", err)
			return
		}
		fmt.Fprintf(c.output, "%s\n", jsonBytes)
	} else {
		// Text output (default)
		fmt.Fprintf(c.output, "Visited: %s\n", result.FinalURL)
		fmt.Fprintf(c.output, "Links found:\n")

		if result.Err != nil {
			// On error, print empty links list
			return
		}

		for _, link := range sanitized {
			fmt.Fprintf(c.output, "%s\n", link)
		}
	}
}

// logError logs an error to stderr with appropriate categorization.
// All logging is done by the coordinator, not by workers.
func (c *Coordinator) logError(url string, err error) {
	if httpErr, ok := err.(*HTTPError); ok {
		log.Printf("Failed to fetch %s: %s [%s]", url, httpErr.Error(), httpErr.Category())
	} else {
		log.Printf("Failed to fetch %s: %v", url, err)
	}
}
