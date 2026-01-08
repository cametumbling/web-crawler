package crawler

import (
	"net/url"
	"strings"
)

// Sanitize normalizes a raw href string against a base URL.
// Returns the absolute, normalized URL string and true if valid, or "", false if invalid.
//
// Normalization rules:
// - Parse href as URL reference and resolve against base URL
// - Require scheme is http or https
// - Lowercase hostname
// - Strip fragment (#...)
// - Normalize path: if empty -> "/"
// - Keep query string
// - Keep trailing slashes
// - Strip default port (80 for http, 443 for https)
func Sanitize(href string, baseURL *url.URL) (string, bool) {
	// Parse href as a URL reference
	ref, err := url.Parse(href)
	if err != nil {
		return "", false
	}

	// Resolve against base URL
	absURL := baseURL.ResolveReference(ref)

	// Require http or https scheme
	if absURL.Scheme != "http" && absURL.Scheme != "https" {
		return "", false
	}

	// Lowercase hostname
	absURL.Host = strings.ToLower(absURL.Host)

	// Strip default port
	if (absURL.Scheme == "http" && strings.HasSuffix(absURL.Host, ":80")) {
		absURL.Host = strings.TrimSuffix(absURL.Host, ":80")
	}
	if (absURL.Scheme == "https" && strings.HasSuffix(absURL.Host, ":443")) {
		absURL.Host = strings.TrimSuffix(absURL.Host, ":443")
	}

	// Normalize path: if empty -> "/"
	if absURL.Path == "" {
		absURL.Path = "/"
	}

	// Strip fragment
	absURL.Fragment = ""

	return absURL.String(), true
}

// InScope returns true if the given URL's hostname matches the startHost (case-insensitive).
// Only URLs with matching hostnames are considered in-scope.
func InScope(urlStr string, startHost string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Extract hostname and compare case-insensitively
	candidateHost := strings.ToLower(u.Hostname())
	normalizedStartHost := strings.ToLower(startHost)

	return candidateHost == normalizedStartHost
}

// Key returns the canonical string representation of a URL for deduplication.
// The key reflects the same normalization rules as Sanitize.
func Key(urlStr string) string {
	// Parse the URL
	u, err := url.Parse(urlStr)
	if err != nil {
		// If invalid, return as-is (will fail scope checks anyway)
		return urlStr
	}

	// Apply same normalization as Sanitize
	u.Host = strings.ToLower(u.Host)

	// Strip default port
	if (u.Scheme == "http" && strings.HasSuffix(u.Host, ":80")) {
		u.Host = strings.TrimSuffix(u.Host, ":80")
	}
	if (u.Scheme == "https" && strings.HasSuffix(u.Host, ":443")) {
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}

	// Normalize path
	if u.Path == "" {
		u.Path = "/"
	}

	// Strip fragment
	u.Fragment = ""

	return u.String()
}
