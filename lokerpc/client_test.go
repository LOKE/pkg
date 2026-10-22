package lokerpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

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

func TestClient_DoRequestSendsDeadline(t *testing.T) {
	var gotDeadline string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDeadline = r.Header.Get("X-Request-Deadline")
	}))
	defer server.Close()

	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	if err := NewClient(server.URL).DoRequest(ctx, "method", nil, nil); err != nil {
		t.Fatalf("DoRequest() error = %v", err)
	}

	got, err := time.Parse(time.RFC3339Nano, gotDeadline)
	if err != nil {
		t.Fatalf("X-Request-Deadline = %q, not RFC3339: %v", gotDeadline, err)
	}
	if !got.Equal(deadline) {
		t.Errorf("X-Request-Deadline = %v, want %v", got, deadline)
	}
}

func TestClient_ErrorDetail(t *testing.T) {
	const cardDeclined = `{"message":"declined","instance":"abc","expose":true,"code":"card_declined","type":"https://example.com/errors/card_declined","paymentIntentId":"pi_123","declineCode":"insufficient_funds","attempts":3}`

	const capitalised = `{"Message":"declined","Code":"card_declined","paymentIntentId":"pi_1"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := cardDeclined
		switch r.URL.Path {
		case "/plain":
			body = `{"message":"plain"}`
		case "/capitalised":
			body = capitalised
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	var intentID, declineCode, wrongType string
	var attempts int

	tests := []struct {
		name      string
		method    string
		wrap      bool
		key       string
		into      any
		wantFound bool
		wantErr   bool
		want      any
	}{
		{name: "string field", method: "declined", key: "paymentIntentId", into: &intentID, wantFound: true, want: "pi_123"},
		{name: "second string field", method: "declined", key: "declineCode", into: &declineCode, wantFound: true, want: "insufficient_funds"},
		{name: "number field", method: "declined", key: "attempts", into: &attempts, wantFound: true, want: 3},
		{name: "wrapped error", method: "declined", wrap: true, key: "paymentIntentId", into: &intentID, wantFound: true, want: "pi_123"},
		{name: "unknown key", method: "declined", key: "nope", into: &wrongType},
		{name: "known field is not a detail", method: "declined", key: "message", into: &wrongType},
		{name: "capitalised known field is not a detail", method: "capitalised", key: "Message", into: &wrongType},
		{name: "capitalised body keeps details", method: "capitalised", key: "paymentIntentId", into: &intentID, wantFound: true, want: "pi_1"},
		{name: "no extra fields", method: "plain", key: "paymentIntentId", into: &wrongType},
		{name: "type mismatch", method: "declined", key: "attempts", into: &wrongType, wantFound: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewClient(server.URL).DoRequest(context.Background(), tt.method, nil, nil)
			if tt.wrap {
				err = fmt.Errorf("calling upstream: %w", err)
			}

			found, detailErr := ErrorDetail(err, tt.key, tt.into)
			if found != tt.wantFound || (detailErr != nil) != tt.wantErr {
				t.Fatalf("ErrorDetail() = (%v, %v), want found=%v err=%v", found, detailErr, tt.wantFound, tt.wantErr)
			}
			if tt.want != nil && !reflect.DeepEqual(reflect.ValueOf(tt.into).Elem().Interface(), tt.want) {
				t.Errorf("ErrorDetail() decoded %v, want %v", tt.into, tt.want)
			}
		})
	}

	t.Run("non rpc errors", func(t *testing.T) {
		for _, err := range []error{nil, fmt.Errorf("boom"), errors.NewPublicError("code", "msg")} {
			found, detailErr := ErrorDetail(err, "paymentIntentId", &intentID)
			if found || detailErr != nil {
				t.Errorf("ErrorDetail(%v) = (%v, %v), want (false, nil)", err, found, detailErr)
			}
		}
	})

	t.Run("known fields still decoded", func(t *testing.T) {
		tests := []struct {
			name   string
			method string
			want   rpcClientError
		}{
			{"lowercase", "declined", rpcClientError{Message: "declined", Instance: "abc", Expose: true, Code: "card_declined", Type: "https://example.com/errors/card_declined"}},
			{"capitalised", "capitalised", rpcClientError{Message: "declined", Code: "card_declined"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := NewClient(server.URL).DoRequest(context.Background(), tt.method, nil, nil)

				rpcErr, ok := errors.AsType[*rpcClientError](err)
				if !ok {
					t.Fatalf("expected *rpcClientError, got %T", err)
				}
				got := *rpcErr
				got.Rest = nil
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("decoded %+v, want %+v", got, tt.want)
				}
			})
		}
	})

	t.Run("plain error has nil rest", func(t *testing.T) {
		err := NewClient(server.URL).DoRequest(context.Background(), "plain", nil, nil)

		rpcErr, _ := errors.AsType[*rpcClientError](err)
		if rpcErr == nil || rpcErr.Rest != nil {
			t.Errorf("expected nil Rest, got %v", rpcErr)
		}
	})
}
