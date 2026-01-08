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
- `-format` (optional, default "text"): Output format - "text" for human-readable or "json" for machine-parseable

## Design Summary

- **Coordinator + Worker Pool Pattern**: Single coordinator goroutine manages state while stateless workers perform fetch/parse operations
- **Coordinator Owns All State**: The `visited` map and `sync.WaitGroup` are owned exclusively by the coordinator; workers never mutate shared state
- **Strict Termination Invariant**: `wg.Add(1)` called before enqueuing work, `wg.Done()` called after processing results and enqueuing derived work
- **Single-Writer Output**: Only the coordinator prints to stdout, ensuring clean output without mutex contention
- **URL Normalization**: Lowercase hostname, fragment stripping, relative URL resolution, default port removal
- **Scope Enforcement**: Only follows links matching the exact hostname (case-insensitive) of the starting URL
- **No Retry Logic**: Failed requests are logged to stderr and skipped; keeps complexity low
- **Bounded Resources**: Configurable worker pool size, optional request rate limiting, response body size cap
- **Graceful Shutdown**: SIGINT/SIGTERM handlers stop scheduling new work while completing in-flight requests
- **Structured Error Categorization**: HTTP errors categorized as dead links (404), retry-able server errors (5xx), or network errors

## Design Decisions

### Output Format: Text vs JSON

**Decision**: Default to human-readable text, with optional JSON via `-format` flag.

**Rationale**:
- The spec states "print the page URL and the list of links" which suggests human-readable output
- Text is easier to scan visually during development and debugging
- JSON enables programmatic consumption for log aggregation, data pipelines, or integration with other tools
- Both formats use the same underlying data, maintaining consistency

**Text format** (default):
```
Visited: https://example.com/
Links found:
https://example.com/about
https://example.com/contact
```

**JSON format** (`-format json`):
```json
{"url":"https://example.com/","links":["https://example.com/about","https://example.com/contact"]}
{"url":"https://example.com/about","links":["https://example.com/"]}
{"url":"https://example.com/error","links":[],"error":"fetch failed: server error (500)"}
```

### Error Categorization

**Decision**: Categorize HTTP errors into actionable types with structured logging.

**Categories**:
- **Dead link (404)**: Permanent failure, page doesn't exist
- **Server error (5xx)**: Transient failure, potentially retry-able
- **Timeout (408, 504)**: Slow server, may succeed on retry
- **Network error**: DNS, connection refused, context cancelled

**Rationale**:
- Different error types require different operational responses
- Dead links (404) are worth reporting but don't warrant retries
- Server errors (5xx) are transient and could succeed if retried
- Structured logs enable filtering and alerting in production monitoring systems

**Example stderr output**:
```
2026/01/07 15:30:01 Failed to fetch https://example.com/missing: not found (404) [dead link]
2026/01/07 15:30:02 Failed to fetch https://example.com/broken: server error (500) [server error (retry-able)]
```

### Stdout/Stderr Separation

**Decision**: Crawl results to stdout, telemetry/errors to stderr.

**Rationale**:
- Follows Unix philosophy: separate data from metadata
- Enables piping results to a file while monitoring progress on terminal: `./crawler -url URL > results.txt`
- Stderr shows configuration, progress logs, errors, and final summary
- Stdout contains only crawl data for clean programmatic consumption

### Non-HTML Content

**Decision**: Return empty links (not an error) for non-HTML content types.

**Rationale**:
- PDFs, images, JSON files are valid pages but have no extractable `<a href>` links
- Marking them as visited prevents re-crawling
- Not treating as error keeps error counts meaningful (actual failures vs. expected behavior)

### Architecture Trade-offs

**Chosen**: Coordinator + worker pool with channels
- ✅ Clean separation of concerns (state vs. work)
- ✅ Easy to reason about termination
- ✅ No shared mutable state between goroutines
- ❌ Slightly more complex than a simple recursive crawler
- ❌ Coordinator is a single point of coordination (but not a bottleneck for I/O-bound work)

**Not chosen**: Shared map with mutex
- ✅ Simpler code
- ❌ Lock contention on visited map
- ❌ Harder to enforce termination invariant
- ❌ Risk of data races

**Not chosen**: Actor model / message passing for visited tracking
- ✅ More scalable for distributed systems
- ❌ Over-engineering for single-machine crawler
- ❌ Added complexity without clear benefit

### HTML Parsing

**Chosen**: `html.Parse()` (DOM parser)
- ✅ Simple, readable code with recursive tree walk
- ✅ Handles malformed HTML gracefully
- ✅ Easier to test and maintain
- ❌ Uses more memory (builds full DOM tree)
- ❌ Slightly slower than tokenizer for huge documents

**Not chosen**: `html.NewTokenizer()` (streaming)
- ✅ Lower memory footprint (streaming)
- ✅ Faster for very large documents
- ❌ More complex state management
- ❌ Manual EOF and error handling
- ❌ Not a bottleneck for this use case (network I/O dominates)

### Rate Limiting

**Chosen**: `time.Tick()` for global rate limiter
- ✅ Simple implementation for single Client instance
- ✅ Shared across all workers naturally
- ❌ Cannot be stopped (minor resource leak)
- ❌ Not appropriate if creating/destroying many Clients

**Note**: For production with dynamic Client lifecycle, would use `time.NewTicker()` with `defer ticker.Stop()`

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
