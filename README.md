# Web Crawler

A high-performance concurrent web crawler built in Go that crawls a single subdomain and reports all discovered links.

## How to Run

```bash
# Build the crawler
go build -o crawler ./cmd/crawler

# Run with a starting URL (required)
./crawler -url https://crawlme.monzo.com/

# Run with optional flags
./crawler -url https://crawlme.monzo.com/ -workers 16 -max-pages 100 -rate-ms 50
```

### CLI Flags

- `-url` (required): Starting absolute URL to begin crawling
- `-workers` (optional, default 8): Number of concurrent workers
- `-max-pages` (optional, default 0 = unlimited): Maximum pages to visit before stopping
- `-rate-ms` (optional, default 0 = no limit): Minimum milliseconds between requests (politeness)

## Design Summary

- **Coordinator + Worker Pool Pattern**: Single coordinator goroutine manages state while stateless workers perform fetch/parse operations
- **Coordinator Owns All State**: The `visited` map and `sync.WaitGroup` are owned exclusively by the coordinator; workers never mutate shared state
- **Strict Termination Invariant**: `wg.Add(1)` called before enqueuing work, `wg.Done()` called after processing results and enqueuing derived work
- **Single-Writer Output**: Only the coordinator prints to stdout, ensuring clean output without mutex contention
- **URL Normalization**: Lowercase hostname, fragment stripping, relative URL resolution, default port removal
- **Scope Enforcement**: Only follows links matching the exact hostname (case-insensitive) of the starting URL
- **No Retry Logic**: Failed requests are logged to stderr and skipped; keeps complexity low
- **Bounded Resources**: Configurable worker pool size, optional request rate limiting, response body size cap

## How to Test

```bash
# Run all tests
go test ./...

# Run with race detector (recommended)
go test -race ./...

# Run with verbose output
go test -v ./...
```

### Test Coverage

- **Unit tests**: URL sanitization, relative link resolution, scope checking, normalization consistency
- **Integration tests**: End-to-end crawl with httptest server validating termination, deduplication, and scope enforcement
