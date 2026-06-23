package lokerpc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/LOKE/pkg/errors"
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
		t.Run(tc.name, func(t *testing.T) {
			c := newClientWithClient(tc.baseURL, http.DefaultClient)

			if c.bURL != tc.expected {
				t.Fatalf("expected normalized base URL %q, got %q", tc.expected, c.bURL)
			}
		})
	}
}

func TestClient_DoRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/notFound", http.NotFound)
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	mux.HandleFunc("/echo_error", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write(body)
	})
	mux.HandleFunc("/drop", func(w http.ResponseWriter, r *http.Request) {
		conn, _, _ := http.NewResponseController(w).Hijack()
		conn.Close()
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		method      string
		args        any
		result      any
		want        any
		wantErr     bool
		wantErrCode string
	}{
		{
			name:   "basic echo",
			method: "echo",
			args:   struct{ Foo string }{Foo: "bar"},
			result: &struct{ Foo string }{},
			want:   &struct{ Foo string }{Foo: "bar"},
		},
		{
			name:   "basic error",
			method: "echo_error",
			args: struct {
				Code string `json:"code"`
			}{Code: "foo_bar"},
			wantErr:     true,
			wantErrCode: "foo_bar",
		},
		{
			name:    "drop",
			method:  "drop",
			wantErr: true,
		},
		{
			name:    "not found",
			method:  "notFound",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			c := NewClient(server.URL)
			gotErr := c.DoRequest(context.Background(), tt.method, tt.args, tt.result)

			if tt.wantErr == (gotErr == nil) {
				t.Errorf("DoRequest() failed: got error %v, want %v", gotErr, tt.wantErr)
			}

			if tt.wantErrCode != "" {
				if gotErr == nil || errors.ErrorCode(gotErr) != tt.wantErrCode {
					t.Errorf("DoRequest() error code mismatch: got %v, want %v",
						errors.ErrorCode(gotErr), tt.wantErrCode)
				}
			}

			if tt.want != nil {
				if !reflect.DeepEqual(tt.result, tt.want) {
					t.Errorf("DoRequest() result mismatch: got %v, want %v", tt.result, tt.want)
				}
			}
		})
	}
}
