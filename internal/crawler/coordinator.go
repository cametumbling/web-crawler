package crawler

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"sync"
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
	// numWorkers is the number of worker goroutines
	numWorkers int
	// output is where we write results (default: os.Stdout)
	output io.Writer
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

	return &Coordinator{
		visited:    make(map[string]bool),
		workCh:     make(chan WorkItem),
		resultsCh:  make(chan Result),
		fetcher:    cfg.Fetcher,
		parser:     cfg.Parser,
		startURL:   startURL,
		startHost:  startURL.Hostname(),
		maxPages:   cfg.MaxPages,
		numWorkers: cfg.NumWorkers,
		output:     output,
	}, nil
}

// Crawl starts the crawl and blocks until completion.
func (c *Coordinator) Crawl() error {
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
			worker(c.workCh, c.resultsCh, c.fetcher, c.parser)
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
	c.workCh <- WorkItem{URL: c.startURL.String()}

	// Process results until all workers are done
	c.processResults()

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
func (c *Coordinator) processResults() {
	for result := range c.resultsCh {
		c.processResult(result)
	}
}

// processResult handles a single result from a worker.
// This is where the termination invariant is enforced.
func (c *Coordinator) processResult(result Result) {
	// Print the page (even on error)
	c.printResult(result)

	// If there was an error, we still call wg.Done() but don't enqueue new work
	if result.Err != nil {
		c.wg.Done()
		return
	}

	// Sanitize all links
	sanitized := c.sanitizeLinks(result.Links, result.URL)

	// For each sanitized link, check scope and visited
	for _, link := range sanitized {
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

// printResult prints the result to stdout in the required format.
func (c *Coordinator) printResult(result Result) {
	fmt.Fprintf(c.output, "Visited: %s\n", result.URL)
	fmt.Fprintf(c.output, "Links found:\n")

	if result.Err != nil {
		// On error, print empty links list
		return
	}

	// Sanitize and print all links found (not just in-scope ones)
	sanitized := c.sanitizeLinks(result.Links, result.URL)
	for _, link := range sanitized {
		fmt.Fprintf(c.output, "%s\n", link)
	}
}
