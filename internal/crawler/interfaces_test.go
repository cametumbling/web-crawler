package crawler

import (
	"testing"
)

func TestHTTPError_Error(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       string
	}{
		{"404 Not Found", 404, "not found (404)"},
		{"500 Internal Server Error", 500, "server error (500)"},
		{"503 Service Unavailable", 503, "server error (503)"},
		{"403 Forbidden", 403, "client error (403)"},
		{"400 Bad Request", 400, "client error (400)"},
		{"301 Moved Permanently", 301, "redirect not followed (301)"},
		{"302 Found", 302, "redirect not followed (302)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &HTTPError{
				StatusCode: tt.statusCode,
				URL:        "https://example.com/test",
			}
			got := err.Error()
			if got != tt.want {
				t.Errorf("HTTPError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHTTPError_Category(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       string
	}{
		{"404 is dead link", 404, "dead link"},
		{"500 is retry-able", 500, "server error (retry-able)"},
		{"502 is retry-able", 502, "server error (retry-able)"},
		{"503 is retry-able", 503, "server error (retry-able)"},
		{"408 is timeout", 408, "timeout"},
		{"504 is timeout", 504, "timeout"},
		{"403 is http error", 403, "http error"},
		{"400 is http error", 400, "http error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &HTTPError{
				StatusCode: tt.statusCode,
				URL:        "https://example.com/test",
			}
			got := err.Category()
			if got != tt.want {
				t.Errorf("HTTPError.Category() = %q, want %q", got, tt.want)
			}
		})
	}
}
