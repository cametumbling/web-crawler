# Monorepo/Project: Monzo Crawler (Go)

## Overview

A high-performance concurrent web crawler built in Go.
Follow the full technical architecture and invariants in: @SPEC.md

## Build & Test Commands

... (existing content) ...

## Tech Stack & Style

... (existing content) ...

## Critical Guardrails

- IMPORTANT: The Coordinator MUST own the `visited` map and the `sync.WaitGroup`. Workers must be stateless.
- IMPORTANT: Never ignore errors. Handle them explicitly as per Go idioms.
- IMPORTANT: Follow the repository structure defined in SPEC.md exactly.

## Scope & Non-Goals (from ADR)

- **Do not** implement `robots.txt` support (site returns 403 anyway).
- **Do not** implement JavaScript rendering (site is static HTML).
- **Do not** implement complex retry/backoff logic (skip and log on failure).
- Ensure correct **relative URL resolution** is implemented (e.g., `about.html` not `/about.html`).
- **Termination Invariant:** The coordinator calls `wg.Add(1)` _before_ sending work to the channel, and `wg.Done()` only _after_ processing results and queuing all subsequent work.
