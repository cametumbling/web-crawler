package htmlparser

import (
	"strings"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected []string
	}{
		{
			name: "absolute URLs",
			html: `<html><body>
				<a href="https://example.com/page1">Link 1</a>
				<a href="http://example.com/page2">Link 2</a>
			</body></html>`,
			expected: []string{"https://example.com/page1", "http://example.com/page2"},
		},
		{
			name: "relative URLs",
			html: `<html><body>
				<a href="/about">About</a>
				<a href="contact.html">Contact</a>
				<a href="../parent">Parent</a>
			</body></html>`,
			expected: []string{"/about", "contact.html", "../parent"},
		},
		{
			name: "fragment URLs",
			html: `<html><body>
				<a href="#section1">Section 1</a>
				<a href="/page#section2">Page Section 2</a>
			</body></html>`,
			expected: []string{"#section1", "/page#section2"},
		},
		{
			name: "mixed content",
			html: `<html><body>
				<a href="https://example.com/absolute">Absolute</a>
				<a href="/relative">Relative</a>
				<a href="#fragment">Fragment</a>
				<a href="page.html">File</a>
			</body></html>`,
			expected: []string{"https://example.com/absolute", "/relative", "#fragment", "page.html"},
		},
		{
			name:     "empty href",
			html:     `<html><body><a href="">Empty</a></body></html>`,
			expected: []string{""},
		},
		{
			name:     "no href attribute",
			html:     `<html><body><a>No href</a></body></html>`,
			expected: []string{},
		},
		{
			name:     "no links",
			html:     `<html><body><p>No links here</p></body></html>`,
			expected: []string{},
		},
		{
			name: "ignores non-anchor tags",
			html: `<html><head>
				<link rel="stylesheet" href="style.css">
			</head><body>
				<script src="script.js"></script>
				<img src="image.jpg">
				<a href="/valid">Valid</a>
			</body></html>`,
			expected: []string{"/valid"},
		},
		{
			name: "multiple attributes",
			html: `<html><body>
				<a id="link1" class="nav" href="/page1" target="_blank">Link</a>
				<a href="/page2" title="Page 2">Link 2</a>
			</body></html>`,
			expected: []string{"/page1", "/page2"},
		},
		{
			name: "nested links (malformed but parseable)",
			html: `<html><body>
				<div><a href="/outer"><span><a href="/inner">Inner</a></span></a></div>
			</body></html>`,
			expected: []string{"/outer", "/inner"},
		},
		{
			name: "duplicate hrefs",
			html: `<html><body>
				<a href="/page">Link 1</a>
				<a href="/page">Link 2</a>
			</body></html>`,
			expected: []string{"/page", "/page"},
		},
		{
			name: "query strings and ports",
			html: `<html><body>
				<a href="http://example.com:8080/page?foo=bar&baz=qux">Query</a>
				<a href="/search?q=test">Search</a>
			</body></html>`,
			expected: []string{"http://example.com:8080/page?foo=bar&baz=qux", "/search?q=test"},
		},
		{
			name: "trailing slashes preserved",
			html: `<html><body>
				<a href="/page/">With slash</a>
				<a href="/page">Without slash</a>
			</body></html>`,
			expected: []string{"/page/", "/page"},
		},
		{
			name: "special characters in URLs",
			html: `<html><body>
				<a href="/path%20with%20spaces">Encoded</a>
				<a href="/path/to/file.html?query=value&other=value">Complex</a>
			</body></html>`,
			expected: []string{"/path%20with%20spaces", "/path/to/file.html?query=value&other=value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.html)
			got, err := ExtractLinks(r)
			if err != nil {
				t.Fatalf("ExtractLinks() error = %v", err)
			}

			if len(got) != len(tt.expected) {
				t.Fatalf("ExtractLinks() got %d links, want %d\nGot: %v\nWant: %v",
					len(got), len(tt.expected), got, tt.expected)
			}

			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("ExtractLinks()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestExtractLinks_InvalidHTML(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		wantErr bool
	}{
		{
			name:    "valid but minimal HTML",
			html:    `<a href="/test">Link</a>`,
			wantErr: false,
		},
		{
			name:    "unclosed tags",
			html:    `<html><body><a href="/test">Link</body></html>`,
			wantErr: false,
		},
		{
			name:    "completely empty",
			html:    ``,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.html)
			_, err := ExtractLinks(r)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractLinks() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
