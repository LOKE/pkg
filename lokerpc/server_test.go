package lokerpc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LOKE/pkg/errors"
	"github.com/go-kit/log"
)

func TestMountHandlers(t *testing.T) {
	// A basic type for requests and responses
	type Foo struct {
		Foo string `json:"foo"`
	}

	tests := []struct {
		name          string
		req           *http.Request
		endpointCoded EndpointCodec
		wantBody      string
		wantStatus    int
	}{
		{
			name: "basic request",
			req:  httptest.NewRequest("POST", "/rpc/service1/basic", strings.NewReader(`{"foo":"bar"}`)),
			endpointCoded: MakeStandardEndpointCodec(func(ctx context.Context, req Foo) (Foo, error) {
				return Foo{"baz"}, nil
			}, ""),
			wantBody:   `{"foo":"baz"}`,
			wantStatus: 200,
		},
		{
			name: "public error",
			req:  httptest.NewRequest("POST", "/rpc/service1/basic", strings.NewReader(`{}`)),
			endpointCoded: MakeStandardEndpointCodec(func(ctx context.Context, req struct{}) (*struct{}, error) {
				return nil, errors.NewPublicError("public_error_code", "Some public error")
			}, ""),
			wantBody:   `{"message":"Some public error","expose":true,"code":"public_error_code","type":"public_error_code"}`,
			wantStatus: 400,
		},
		{
			name: "generic error",
			req:  httptest.NewRequest("POST", "/rpc/service1/basic", strings.NewReader(`{}`)),
			endpointCoded: MakeStandardEndpointCodec(func(ctx context.Context, req struct{}) (*struct{}, error) {
				return nil, fmt.Errorf("some error")
			}, ""),
			wantBody:   `{"message":"some error"}`,
			wantStatus: 400,
		},
		{
			name: "upstream rpc error",
			req:  httptest.NewRequest("POST", "/rpc/service1/basic", strings.NewReader(`{}`)),
			endpointCoded: MakeStandardEndpointCodec(func(ctx context.Context, req struct{}) (*struct{}, error) {
				return nil, &rpcClientError{
					Message:   "some error",
					Instance:  "123",
					Expose:    true,
					Code:      "bar",
					Type:      "https://example.com/errors/foo/bar",
					Namespace: "foo",
				}
			}, ""),
			wantBody:   `{"message":"some error","instance":"123","expose":true,"code":"bar","namespace":"foo","type":"https://example.com/errors/foo/bar"}`,
			wantStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			MountHandlers(
				log.NewNopLogger(),
				mux,
				NewService("service1", "", EndpointCodecMap{
					"basic": tt.endpointCoded,
				}),
			)

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, tt.req)

			if rr.Code != tt.wantStatus {
				t.Errorf("want status %d, got %d", tt.wantStatus, rr.Code)
			}

			gotBody := strings.Trim(rr.Body.String(), "\n")
			if gotBody != tt.wantBody {
				t.Errorf("want body %q, got %q", tt.wantBody, gotBody)
			}
		})
	}
}
