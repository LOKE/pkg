package lokerpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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
			name: "upstream rpc error, expose true",
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
		{
			name: "upstream rpc error, expose false",
			req:  httptest.NewRequest("POST", "/rpc/service1/basic", strings.NewReader(`{}`)),
			endpointCoded: MakeStandardEndpointCodec(func(ctx context.Context, req struct{}) (*struct{}, error) {
				return nil, &rpcClientError{
					Message:   "some error",
					Instance:  "123",
					Expose:    false,
					Code:      "bar",
					Type:      "https://example.com/errors/foo/bar",
					Namespace: "foo",
				}
			}, ""),
			wantBody:   `{"message":"some error","instance":"123","code":"bar","namespace":"foo","type":"https://example.com/errors/foo/bar"}`,
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

func TestServiceMeta(t *testing.T) {
	type request struct {
		Value string `json:"value"`
	}
	codec := MakeStandardEndpointCodec(func(_ context.Context, req request) (request, error) {
		return req, nil
	}, "Echo the value")
	service := NewService("metadata", "Metadata test service", EndpointCodecMap{
		"zebra": codec,
		"alpha": codec,
	})

	meta := service.Meta()
	if meta.ServiceName != service.Name || meta.Help != service.Help || meta.MultiArg {
		t.Fatalf("Meta() service fields = %+v", meta)
	}
	if len(meta.Interfaces) != 2 {
		t.Fatalf("Meta() interfaces = %v, want two interfaces", meta.Interfaces)
	}
	for i, method := range []string{"alpha", "zebra"} {
		endpoint := meta.Interfaces[i]
		if endpoint.MethodName != method {
			t.Errorf("interface %d method = %q, want %q", i, endpoint.MethodName, method)
		}
		if endpoint.Help != codec.Help || endpoint.MethodTimeout != 60000 ||
			!reflect.DeepEqual(endpoint.ParamNames, []string{"value"}) {
			t.Errorf("interface %d metadata = %+v", i, endpoint)
		}
		if endpoint.RequestTypeDef == nil || endpoint.ResponseTypeDef == nil {
			t.Errorf("interface %d is missing request or response schema", i)
		}
	}
	if len(meta.Definitions) == 0 {
		t.Error("Meta() is missing schema definitions")
	}
}

func TestServiceMountHandlers(t *testing.T) {
	service := NewService("standalone", "Standalone service", EndpointCodecMap{
		"echo": MakeStandardEndpointCodec(func(_ context.Context, req struct{}) (string, error) {
			return "echoed", nil
		}, ""),
	})
	mux := http.NewServeMux()
	service.MountHandlers(log.NewNopLogger(), mux)

	root := httptest.NewRecorder()
	mux.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/rpc", nil))
	if root.Code != http.StatusNotFound {
		t.Errorf("root status = %d, want 404", root.Code)
	}

	metadata := httptest.NewRecorder()
	mux.ServeHTTP(metadata, httptest.NewRequest(http.MethodGet, "/rpc/standalone", nil))
	if metadata.Code != http.StatusOK {
		t.Fatalf("metadata status = %d, want 200", metadata.Code)
	}
	if got := metadata.Header().Get("Content-Type"); got != ContentType {
		t.Errorf("metadata content type = %q, want %q", got, ContentType)
	}
	wantMeta, err := json.Marshal(service.Meta())
	if err != nil {
		t.Fatalf("encode metadata: %v", err)
	}
	if got := strings.TrimSpace(metadata.Body.String()); got != string(wantMeta) {
		t.Errorf("served metadata = %s, want %s", got, wantMeta)
	}

	rejected := httptest.NewRecorder()
	mux.ServeHTTP(rejected, httptest.NewRequest(http.MethodPost, "/rpc/standalone", nil))
	if rejected.Code != http.StatusMethodNotAllowed {
		t.Errorf("metadata POST status = %d, want 405", rejected.Code)
	}

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/rpc/standalone/echo", strings.NewReader(`{}`)))
	if response.Code != http.StatusOK || response.Body.String() != "\"echoed\"\n" {
		t.Errorf("method response = %d %q, want 200 echoed", response.Code, response.Body.String())
	}
}

func TestFieldNames(t *testing.T) {
	type Pagination struct {
		Limit          *int    `json:"limit,omitempty"`
		NextPageCursor *string `json:"nextPageCursor,omitempty"`
	}

	got := FieldNames(struct {
		OrganizationUID string `json:"organizationUid"`
		Secret          string `json:"-"`
		Pagination
	}{})

	want := []string{"organizationUid", "limit", "nextPageCursor"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FieldNames() = %v, want %v", got, want)
	}
}
