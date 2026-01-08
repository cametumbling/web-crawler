# SPEC.md — Monzo Take-Home Crawler (Go)

## Goal

Given a starting URL, crawl every page on the same hostname (single subdomain). For each visited page, print the page URL and the list of links found on that page. Do not follow links to other hosts.

## Non-goals

No UI, no sitemap format requirements, no robots.txt support required, no JS rendering, no retries/backoff unless trivial.

## CLI

Command:

- `crawler -url <starting_url> [-workers N] [-max-pages M] [-rate-ms R]`

Flags:

- `-url` (required): starting absolute URL, e.g. `https://crawlme.monzo.com/`
- `-workers` (optional, default 8): number of concurrent workers
- `-max-pages` (optional, default 0 meaning unlimited): stop after visiting N pages
- `-rate-ms` (optional, default 0 meaning no rate limit): minimum ms between requests globally (politeness)

Exit codes:

- non-zero on invalid input or fatal internal error

## Scope rule (single subdomain)

Let `startHost = startURL.Hostname()`.
A URL is in-scope iff `candidate.Hostname()` equals `startHost` (case-insensitive).
Do not follow external hosts (including parent domain and other subdomains).

## Output contract

Stdout:
For each visited page, output exactly:

Visited: <normalized_page_url>
Links found:
<normalized_link_url_1>
<normalized_link_url_2>
...

Notes:

- Links printed are the sanitized/normalized absolute URLs extracted from that page.
- Duplicates are allowed in the printed link list.
- Printing is performed only by the coordinator.

Stderr:
All logs/errors/progress only (never stdout).

## Concurrency architecture

Pattern: Coordinator + worker pool.

Coordinator responsibilities (single goroutine):

- Owns `visited` set (dedupe)
- Owns stdout printing (single writer)
- Owns all scheduling decisions (scope, normalization, dedupe, caps)
- Owns WaitGroup entirely (`Add` and `Done`)
- Owns lifecycle: start workers, stop workers, shutdown

Workers are stateless:

- Receive a WorkItem (URL)
- Fetch HTTP
- Parse HTML
- Extract raw href strings
- Send exactly one Result per WorkItem (success or error)
- Never print, never mutate shared state, never touch WaitGroup

## Termination invariant (must hold)

Unit of work = “fetch + parse + return Result for a single URL”.

Rules:

1. Coordinator calls `wg.Add(1)` exactly once for each URL it enqueues to `workCh`.
2. For each enqueued URL, exactly one Result is sent to `resultsCh` (even on error/panic).
3. Coordinator calls `wg.Done()` exactly once per Result, after it has:
   - printed the page output
   - sanitized links
   - enqueued any new in-scope URLs derived from that page (each with `wg.Add(1)` before enqueue)

Shutdown:

- A closer goroutine waits `wg.Wait()` then closes `workCh`.
- Workers exit when `workCh` is closed.
- Coordinator exits after all workers have exited and results channel is drained (implementation may use a second WaitGroup for workers or close `resultsCh` from a fan-in closer).

## URL processing pipeline

Workers return:

- the page URL that was requested (as provided) and base URL for resolution
- `[]string` raw hrefs (as extracted from HTML)

Coordinator sanitizes each raw href:

Sanitize(href, baseURL) -> (absURL, ok):

- Parse href as URL reference
- Resolve against base URL (`base.ResolveReference(ref)`)
- Require scheme is http or https
- Lowercase host
- Strip fragment (`#...`)
- Normalize path minimally:
  - if path empty -> `/`
- Keep query string
- Keep trailing slashes (do not normalize `/about` to `/about/` or vice versa)
- Optionally strip default port (80 for http, 443 for https) if present
  Return absolute URL.

Printing:

- Print sanitized URLs (not raw hrefs).
- Print all sanitized http(s) links found, regardless of scope.
- Duplicates allowed in print.

Dedupe key:

- `Key(u)` is the canonical string used for `visited` comparisons.
- Key must reflect the same normalization rules used in Sanitize.

## Scheduling policy

For each sanitized link:

- If `InScope(link)` AND `!visited[Key(link)]`:
  - mark visited
  - enforce `max-pages` cap (if enabled): if reached, do not enqueue more
  - `wg.Add(1)` then enqueue link to `workCh`

Start:

- Coordinator sanitizes and normalizes the starting URL and enqueues it as the first WorkItem.

## HTTP fetching

Implementation uses `net/http` with:

- a single shared `http.Client` with timeouts (e.g., 10s total request timeout)
- User-Agent set (simple string)
- Optional max response body size cap (e.g., 2MB) to avoid pathological pages

If `-rate-ms > 0`:

- A global rate limiter is applied before each request (shared ticker/channel).

Errors:

- On non-2xx, still return Result with Err and empty extracted links.
- Still print the visited URL with an empty “Links found:” list.

## HTML parsing / link extraction

Use `golang.org/x/net/html` (or equivalent).
Extract `href` attributes from `<a>` tags only.
Ignore `<link>`, `<script>`, images, etc.

## Tests (required)

Unit tests (table-driven):

- Sanitize/Resolve relative links
- Lowercase host + fragment stripping
- InScope host matching behavior
- Key normalization consistency

Integration test:
Use `httptest.Server` hosting a small graph:

- root links to 2 pages
- a cycle between pages
- a fragment link
- an external host link
  Assertions:
- terminates
- does not visit out-of-scope
- does not visit duplicates
- prints sanitized links (no fragments, lowercase host)

Run:

- `go test ./...`
- `go test -race ./...` (recommended)

## Repository structure (fixed)

Use this structure:

web-crawler/
├── cmd/
│ └── crawler/
│ └── main.go
├── internal/
│ ├── crawler/
│ │ ├── coordinator.go
│ │ ├── interfaces.go
│ │ ├── util.go
│ │ └── worker.go
│ └── platform/
│ ├── httpclient/
│ │ └── client.go
│ └── htmlparser/
│ └── parser.go
├── go.mod
├── go.sum
├── README.md
└── (optional) Makefile

README.md must include:

- How to run
- Design summary (5–10 bullets matching invariants/trade-offs)
- How to test
