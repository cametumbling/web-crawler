package crawler

import (
	"net/url"
	"testing"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name    string
		href    string
		baseURL string
		want    string
		wantOk  bool
	}{
		// Relative URL resolution
		{
			name:    "relative path from root",
			href:    "/about",
			baseURL: "https://example.com/page",
			want:    "https://example.com/about",
			wantOk:  true,
		},
		{
			name:    "relative file",
			href:    "contact.html",
			baseURL: "https://example.com/",
			want:    "https://example.com/contact.html",
			wantOk:  true,
		},
		{
			name:    "relative file from subdirectory",
			href:    "page2.html",
			baseURL: "https://example.com/dir/page1.html",
			want:    "https://example.com/dir/page2.html",
			wantOk:  true,
		},
		{
			name:    "parent directory reference",
			href:    "../parent",
			baseURL: "https://example.com/dir/subdir/page",
			want:    "https://example.com/dir/parent",
			wantOk:  true,
		},
		{
			name:    "current directory reference",
			href:    "./page",
			baseURL: "https://example.com/dir/",
			want:    "https://example.com/dir/page",
			wantOk:  true,
		},
		// Fragment stripping
		{
			name:    "strip fragment from absolute URL",
			href:    "https://example.com/page#section",
			baseURL: "https://example.com/",
			want:    "https://example.com/page",
			wantOk:  true,
		},
		{
			name:    "strip fragment from relative URL",
			href:    "/page#section",
			baseURL: "https://example.com/",
			want:    "https://example.com/page",
			wantOk:  true,
		},
		{
			name:    "fragment only becomes base URL without fragment",
			href:    "#section",
			baseURL: "https://example.com/page",
			want:    "https://example.com/page",
			wantOk:  true,
		},
		// Lowercase hostname
		{
			name:    "lowercase hostname in href",
			href:    "https://EXAMPLE.COM/page",
			baseURL: "https://example.com/",
			want:    "https://example.com/page",
			wantOk:  true,
		},
		{
			name:    "lowercase hostname from base",
			href:    "/page",
			baseURL: "https://EXAMPLE.COM/",
			want:    "https://example.com/page",
			wantOk:  true,
		},
		{
			name:    "mixed case hostname",
			href:    "https://Example.Com/page",
			baseURL: "https://example.com/",
			want:    "https://example.com/page",
			wantOk:  true,
		},
		// Default port stripping
		{
			name:    "strip default http port 80",
			href:    "http://example.com:80/page",
			baseURL: "http://example.com/",
			want:    "http://example.com/page",
			wantOk:  true,
		},
		{
			name:    "strip default https port 443",
			href:    "https://example.com:443/page",
			baseURL: "https://example.com/",
			want:    "https://example.com/page",
			wantOk:  true,
		},
		{
			name:    "keep non-default http port",
			href:    "http://example.com:8080/page",
			baseURL: "http://example.com/",
			want:    "http://example.com:8080/page",
			wantOk:  true,
		},
		{
			name:    "keep non-default https port",
			href:    "https://example.com:8443/page",
			baseURL: "https://example.com/",
			want:    "https://example.com:8443/page",
			wantOk:  true,
		},
		// Path normalization
		{
			name:    "empty path becomes /",
			href:    "https://example.com",
			baseURL: "https://example.com/",
			want:    "https://example.com/",
			wantOk:  true,
		},
		{
			name:    "preserve trailing slash",
			href:    "/page/",
			baseURL: "https://example.com/",
			want:    "https://example.com/page/",
			wantOk:  true,
		},
		{
			name:    "preserve no trailing slash",
			href:    "/page",
			baseURL: "https://example.com/",
			want:    "https://example.com/page",
			wantOk:  true,
		},
		// Query string preservation
		{
			name:    "keep query string",
			href:    "/search?q=test&page=2",
			baseURL: "https://example.com/",
			want:    "https://example.com/search?q=test&page=2",
			wantOk:  true,
		},
		{
			name:    "keep query string with fragment stripped",
			href:    "/search?q=test#results",
			baseURL: "https://example.com/",
			want:    "https://example.com/search?q=test",
			wantOk:  true,
		},
		// Scheme validation
		{
			name:    "reject ftp scheme",
			href:    "ftp://example.com/file",
			baseURL: "https://example.com/",
			want:    "",
			wantOk:  false,
		},
		{
			name:    "reject mailto scheme",
			href:    "mailto:test@example.com",
			baseURL: "https://example.com/",
			want:    "",
			wantOk:  false,
		},
		{
			name:    "reject javascript scheme",
			href:    "javascript:void(0)",
			baseURL: "https://example.com/",
			want:    "",
			wantOk:  false,
		},
		{
			name:    "accept http scheme",
			href:    "http://example.com/page",
			baseURL: "https://example.com/",
			want:    "http://example.com/page",
			wantOk:  true,
		},
		{
			name:    "accept https scheme",
			href:    "https://example.com/page",
			baseURL: "http://example.com/",
			want:    "https://example.com/page",
			wantOk:  true,
		},
		// Complex cases
		{
			name:    "all normalizations combined",
			href:    "HTTPS://EXAMPLE.COM:443/Page/../About?foo=bar#section",
			baseURL: "https://example.com/",
			want:    "https://example.com/About?foo=bar",
			wantOk:  true,
		},
		// Edge cases
		{
			name:    "empty href",
			href:    "",
			baseURL: "https://example.com/page",
			want:    "https://example.com/page",
			wantOk:  true,
		},
		{
			name:    "query only",
			href:    "?query=value",
			baseURL: "https://example.com/page",
			want:    "https://example.com/page?query=value",
			wantOk:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, err := url.Parse(tt.baseURL)
			if err != nil {
				t.Fatalf("Failed to parse base URL: %v", err)
			}

			got, ok := Sanitize(tt.href, base)
			if ok != tt.wantOk {
				t.Errorf("Sanitize() ok = %v, want %v", ok, tt.wantOk)
			}
			if got != tt.want {
				t.Errorf("Sanitize() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInScope(t *testing.T) {
	tests := []struct {
		name      string
		urlStr    string
		startHost string
		want      bool
	}{
		// Exact matches
		{
			name:      "exact match same case",
			urlStr:    "https://example.com/page",
			startHost: "example.com",
			want:      true,
		},
		{
			name:      "exact match case insensitive - uppercase URL",
			urlStr:    "https://EXAMPLE.COM/page",
			startHost: "example.com",
			want:      true,
		},
		{
			name:      "exact match case insensitive - uppercase startHost",
			urlStr:    "https://example.com/page",
			startHost: "EXAMPLE.COM",
			want:      true,
		},
		{
			name:      "exact match case insensitive - mixed case",
			urlStr:    "https://Example.Com/page",
			startHost: "EXAMPLE.com",
			want:      true,
		},
		// Different hosts
		{
			name:      "different host",
			urlStr:    "https://other.com/page",
			startHost: "example.com",
			want:      false,
		},
		{
			name:      "subdomain is different",
			urlStr:    "https://sub.example.com/page",
			startHost: "example.com",
			want:      false,
		},
		{
			name:      "parent domain is different",
			urlStr:    "https://example.com/page",
			startHost: "sub.example.com",
			want:      false,
		},
		{
			name:      "different subdomain",
			urlStr:    "https://sub1.example.com/page",
			startHost: "sub2.example.com",
			want:      false,
		},
		// Port handling
		{
			name:      "ignore port in URL",
			urlStr:    "https://example.com:8080/page",
			startHost: "example.com",
			want:      true,
		},
		{
			name:      "ignore default https port",
			urlStr:    "https://example.com:443/page",
			startHost: "example.com",
			want:      true,
		},
		{
			name:      "ignore default http port",
			urlStr:    "http://example.com:80/page",
			startHost: "example.com",
			want:      true,
		},
		// Invalid URLs
		{
			name:      "invalid URL returns false",
			urlStr:    "://invalid",
			startHost: "example.com",
			want:      false,
		},
		// Scheme variations
		{
			name:      "http scheme matches",
			urlStr:    "http://example.com/page",
			startHost: "example.com",
			want:      true,
		},
		{
			name:      "https scheme matches",
			urlStr:    "https://example.com/page",
			startHost: "example.com",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InScope(tt.urlStr, tt.startHost)
			if got != tt.want {
				t.Errorf("InScope(%q, %q) = %v, want %v", tt.urlStr, tt.startHost, got, tt.want)
			}
		})
	}
}

func TestKey(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		// Fragment stripping
		{
			name: "strip fragment",
			url:  "https://example.com/page#section",
			want: "https://example.com/page",
		},
		// Lowercase hostname
		{
			name: "lowercase hostname",
			url:  "https://EXAMPLE.COM/page",
			want: "https://example.com/page",
		},
		{
			name: "mixed case hostname",
			url:  "https://Example.Com/PAGE",
			want: "https://example.com/PAGE",
		},
		// Default port stripping
		{
			name: "strip default http port",
			url:  "http://example.com:80/page",
			want: "http://example.com/page",
		},
		{
			name: "strip default https port",
			url:  "https://example.com:443/page",
			want: "https://example.com/page",
		},
		{
			name: "keep non-default port",
			url:  "https://example.com:8443/page",
			want: "https://example.com:8443/page",
		},
		// Path normalization
		{
			name: "empty path becomes /",
			url:  "https://example.com",
			want: "https://example.com/",
		},
		{
			name: "preserve trailing slash",
			url:  "https://example.com/page/",
			want: "https://example.com/page/",
		},
		{
			name: "preserve no trailing slash",
			url:  "https://example.com/page",
			want: "https://example.com/page",
		},
		// Query string preservation
		{
			name: "keep query string",
			url:  "https://example.com/search?q=test",
			want: "https://example.com/search?q=test",
		},
		// Path case sensitivity
		{
			name: "preserve path case",
			url:  "https://example.com/Page",
			want: "https://example.com/Page",
		},
		// Consistency test: same logical URL should produce same key
		{
			name: "normalized URL",
			url:  "HTTPS://EXAMPLE.COM:443/page#fragment",
			want: "https://example.com/page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Key(tt.url)
			if got != tt.want {
				t.Errorf("Key(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestKey_Consistency(t *testing.T) {
	// Test that different representations of the same URL produce the same key
	tests := []struct {
		name string
		urls []string
		want string
	}{
		{
			name: "all variations produce same key",
			urls: []string{
				"https://example.com/page",
				"HTTPS://EXAMPLE.COM/page",
				"https://example.com:443/page",
				"https://example.com/page#fragment",
				"https://EXAMPLE.com:443/page#section",
			},
			want: "https://example.com/page",
		},
		{
			name: "http variations",
			urls: []string{
				"http://example.com/page",
				"HTTP://EXAMPLE.COM/page",
				"http://example.com:80/page",
				"http://example.com/page#frag",
			},
			want: "http://example.com/page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, url := range tt.urls {
				got := Key(url)
				if got != tt.want {
					t.Errorf("Key(%q) = %q, want %q", url, got, tt.want)
				}
			}
		})
	}
}

func TestKey_DifferentURLs(t *testing.T) {
	// Test that different URLs produce different keys
	tests := []struct {
		url1 string
		url2 string
	}{
		{
			url1: "https://example.com/page",
			url2: "https://example.com/other",
		},
		{
			url1: "https://example.com/page/",
			url2: "https://example.com/page",
		},
		{
			url1: "https://example.com/Page",
			url2: "https://example.com/page",
		},
		{
			url1: "http://example.com/page",
			url2: "https://example.com/page",
		},
		{
			url1: "https://example.com:8080/page",
			url2: "https://example.com/page",
		},
		{
			url1: "https://example.com/page?foo=bar",
			url2: "https://example.com/page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url1+" vs "+tt.url2, func(t *testing.T) {
			key1 := Key(tt.url1)
			key2 := Key(tt.url2)
			if key1 == key2 {
				t.Errorf("Key(%q) == Key(%q) = %q, want different keys", tt.url1, tt.url2, key1)
			}
		})
	}
}
