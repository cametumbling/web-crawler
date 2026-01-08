package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cametumbling/web-crawler/internal/crawler"
	"github.com/cametumbling/web-crawler/internal/platform/htmlparser"
	"github.com/cametumbling/web-crawler/internal/platform/httpclient"
)

func main() {
	// Parse command line flags
	url := flag.String("url", "", "Starting URL (required)")
	workers := flag.Int("workers", 8, "Number of concurrent workers")
	maxPages := flag.Int("max-pages", 0, "Maximum pages to visit (0 = unlimited)")
	rateMs := flag.Int("rate-ms", 0, "Minimum milliseconds between requests (0 = no limit)")

	flag.Parse()

	// Validate required flags
	if *url == "" {
		fmt.Fprintf(os.Stderr, "Error: -url flag is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Validate flag values
	if *workers <= 0 {
		fmt.Fprintf(os.Stderr, "Error: -workers must be greater than 0\n")
		os.Exit(1)
	}
	if *maxPages < 0 {
		fmt.Fprintf(os.Stderr, "Error: -max-pages cannot be negative\n")
		os.Exit(1)
	}
	if *rateMs < 0 {
		fmt.Fprintf(os.Stderr, "Error: -rate-ms cannot be negative\n")
		os.Exit(1)
	}

	// Create HTTP client with optional rate limiting
	var rateLimit time.Duration
	if *rateMs > 0 {
		rateLimit = time.Duration(*rateMs) * time.Millisecond
	}

	httpClient := httpclient.New(httpclient.Config{
		Timeout:     10 * time.Second,
		UserAgent:   "MonzoCrawler/1.0",
		MaxBodySize: 2 * 1024 * 1024, // 2MB
		RateLimit:   rateLimit,
	})

	// Create coordinator
	coord, err := crawler.NewCoordinator(crawler.Config{
		StartURL:   *url,
		MaxPages:   *maxPages,
		NumWorkers: *workers,
		Fetcher:    httpClient,
		Parser:     &parserAdapter{},
		Output:     os.Stdout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating coordinator: %v\n", err)
		os.Exit(1)
	}

	// Run the crawl
	if err := coord.Crawl(); err != nil {
		fmt.Fprintf(os.Stderr, "Error during crawl: %v\n", err)
		os.Exit(1)
	}
}

// parserAdapter adapts the htmlparser package to the Parser interface.
type parserAdapter struct{}

func (p *parserAdapter) ExtractLinks(r io.Reader) ([]string, error) {
	return htmlparser.ExtractLinks(r)
}
