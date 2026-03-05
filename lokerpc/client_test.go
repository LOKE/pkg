package lokerpc

import (
	"net/http"
	"testing"
)

func TestNewClientNormalizesBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		baseURL  string
		expected string
	}{
		{
			name:     "adds slash when missing",
			baseURL:  "http://example.com/rpc/service",
			expected: "http://example.com/rpc/service/",
		},
		{
			name:     "keeps single trailing slash",
			baseURL:  "http://example.com/rpc/service/",
			expected: "http://example.com/rpc/service/",
		},
		{
			name:     "collapses multiple trailing slashes",
			baseURL:  "http://example.com/rpc/service///",
			expected: "http://example.com/rpc/service/",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := newClientWithClient(tc.baseURL, http.DefaultClient)
			if c.bURL != tc.expected {
				t.Fatalf("expected normalized base URL %q, got %q", tc.expected, c.bURL)
			}
		})
	}
}
