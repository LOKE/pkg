package lokerpc_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/LOKE/pkg/lokerpc"
	v2 "github.com/LOKE/pkg/lokerpc/v2"
	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb"
	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb/testpbconnect"
	"github.com/LOKE/pkg/requestid"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fixtureHandler struct {
	testpbconnect.UnimplementedFixtureServiceHandler
	echo func(context.Context, *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error)
}

func (h fixtureHandler) Echo(ctx context.Context, req *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
	if h.echo != nil {
		return h.echo(ctx, req)
	}
	return connect.NewResponse(req.Msg), nil
}

func (fixtureHandler) Ping(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func fixtureService(name string, handler fixtureHandler, opts ...connect.HandlerOption) v2.Service {
	return v2.Service{
		Name: name,
		Handler: func(extra ...connect.HandlerOption) (string, http.Handler) {
			return testpbconnect.NewFixtureServiceHandler(handler, append(append([]connect.HandlerOption{}, extra...), opts...)...)
		},
	}
}

func mountFixture(t *testing.T, service v2.Service, legacy ...*v1.Service) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	if err := v2.MountHandlers(log.NewNopLogger(), mux, []v2.Service{service}, legacy...); err != nil {
		t.Fatalf("MountHandlers() error = %v", err)
	}
	return mux
}

func serve(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(body)))
	return response
}

func decodeObject(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("response JSON = %q: %v", response.Body.String(), err)
	}
	return result
}

func TestBridgeProtoJSON(t *testing.T) {
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{}))
	for _, body := range []string{"", " \n\t", "{}", `{"unknownField":{"ignored":true}}`} {
		t.Run("defaults/"+body, func(t *testing.T) {
			response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", body)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q", got)
			}
			result := decodeObject(t, response)
			for name, want := range map[string]any{
				"doubleValue": float64(0), "floatValue": float64(0), "int32Value": float64(0),
				"int64Value": "0", "uint64Value": "0", "sint64Value": "0", "fixed64Value": "0", "sfixed64Value": "0",
				"boolValue": false, "stringValue": "", "bytesValue": "", "state": "STATE_UNSPECIFIED",
				"tags": []any{}, "entries": map[string]any{}, "children": []any{}, "labels": map[string]any{}, "customName": "",
			} {
				if got, ok := result[name]; !ok || !reflect.DeepEqual(got, want) {
					t.Errorf("response[%q] = %#v (present %v), want %#v", name, got, ok, want)
				}
			}
			for _, name := range []string{
				"optionalValue", "choiceText", "choiceNested", "nested", "left", "right",
				"timestamp", "duration", "fieldMask", "structValue", "value", "listValue", "anyValue", "empty",
				"wrappedDouble", "wrappedFloat", "wrappedInt64", "wrappedUint64", "wrappedInt32", "wrappedUint32",
				"wrappedBool", "wrappedString", "wrappedBytes", "unknownField", "int64_value", "renamed",
			} {
				if _, ok := result[name]; ok {
					t.Errorf("response unexpectedly contains %q", name)
				}
			}
		})
	}

	body := `{"int64Value":"9223372036854775807","uint64Value":"18446744073709551615","state":"STATE_READY","optionalValue":"","choiceText":"","nested":{"name":"nested"},"tags":["one"],"bytesValue":"AAEC","customName":"custom","unknown":true}`
	response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", body)
	if response.Code != http.StatusOK {
		t.Fatalf("populated status = %d, body = %s", response.Code, response.Body.String())
	}
	result := decodeObject(t, response)
	for name, want := range map[string]any{
		"int64Value": "9223372036854775807", "uint64Value": "18446744073709551615", "state": "STATE_READY",
		"optionalValue": "", "choiceText": "", "bytesValue": "AAEC", "customName": "custom", "tags": []any{"one"},
	} {
		if got := result[name]; !reflect.DeepEqual(got, want) {
			t.Errorf("populated response[%q] = %#v, want %#v", name, got, want)
		}
	}
	if nested, ok := result["nested"].(map[string]any); !ok || nested["name"] != "nested" {
		t.Errorf("nested = %#v", result["nested"])
	}
	if _, ok := result["unknown"]; ok {
		t.Error("unknown request field was echoed")
	}
	response = serve(t, mux, http.MethodPost, "/rpc/fixture/ping", "")
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "{}" {
		t.Errorf("Ping = %d %q, want 200 {}", response.Code, response.Body.String())
	}
}

func TestBridgeDispatchAndInvalidJSON(t *testing.T) {
	calls := 0
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(_ context.Context, req *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
		calls++
		return connect.NewResponse(req.Msg), nil
	}}))
	for _, test := range []struct {
		name, method, path, body string
		status                   int
		message                  string
	}{
		{name: "malformed", method: "POST", path: "echo", body: "{", status: 400, message: "Invalid JSON"},
		{name: "trailing JSON", method: "POST", path: "echo", body: "{} {}", status: 400, message: "Invalid JSON"},
		{name: "wrong scalar type", method: "POST", path: "echo", body: `{"int32Value":"no"}`, status: 400, message: "Invalid JSON"},
		{name: "unknown", method: "POST", path: "missing", body: "{}", status: 404, message: "Not Found"},
		{name: "case sensitive", method: "POST", path: "Echo", body: "{}", status: 404, message: "Not Found"},
		{name: "streaming server", method: "POST", path: "expand", body: "{}", status: 404, message: "Not Found"},
		{name: "streaming client", method: "POST", path: "collect", body: "{}", status: 404, message: "Not Found"},
		{name: "streaming bidi", method: "POST", path: "chat", body: "{}", status: 404, message: "Not Found"},
		{name: "get", method: "GET", path: "echo", status: 405, message: "Method Not Allowed"},
		{name: "put", method: "PUT", path: "echo", body: "{}", status: 405, message: "Method Not Allowed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := serve(t, mux, test.method, "/rpc/fixture/"+test.path, test.body)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.status, response.Body.String())
			}
			if test.message != "" {
				if got, want := decodeObject(t, response), (map[string]any{"message": test.message}); !reflect.DeepEqual(got, want) {
					t.Errorf("error = %#v, want %#v", got, want)
				}
			}
		})
	}
	if calls != 0 {
		t.Errorf("invalid requests invoked Echo %d times", calls)
	}
}

func TestBridgeConnectErrorMapping(t *testing.T) {
	codes := []connect.Code{
		connect.CodeCanceled, connect.CodeUnknown, connect.CodeInvalidArgument, connect.CodeDeadlineExceeded,
		connect.CodeNotFound, connect.CodeAlreadyExists, connect.CodePermissionDenied, connect.CodeResourceExhausted,
		connect.CodeFailedPrecondition, connect.CodeAborted, connect.CodeOutOfRange, connect.CodeUnimplemented,
		connect.CodeInternal, connect.CodeUnavailable, connect.CodeDataLoss, connect.CodeUnauthenticated,
	}
	for _, code := range codes {
		t.Run(code.String(), func(t *testing.T) {
			mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(context.Context, *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
				return nil, connect.NewError(code, errors.New("failure message"))
			}}))
			response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", "{}")
			wantStatus := http.StatusBadRequest
			want := map[string]any{"message": "failure message", "code": code.String(), "type": code.String()}
			if code == connect.CodeUnknown || code == connect.CodeInternal {
				wantStatus = http.StatusInternalServerError
				want = map[string]any{"message": "failure message"}
			}
			if response.Code != wantStatus {
				t.Errorf("status = %d, want %d", response.Code, wantStatus)
			}
			if got := decodeObject(t, response); !reflect.DeepEqual(got, want) {
				t.Errorf("error = %#v, want %#v", got, want)
			}
		})
	}
	t.Run("ordinary error", func(t *testing.T) {
		mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(context.Context, *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
			return nil, errors.New("ordinary failure")
		}}))
		response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", "{}")
		if response.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", response.Code)
		}
		if got, want := decodeObject(t, response), (map[string]any{"message": "ordinary failure"}); !reflect.DeepEqual(got, want) {
			t.Errorf("error = %#v, want %#v", got, want)
		}
	})
}

func TestConnectAndBridgeShareInterceptors(t *testing.T) {
	procedures := make(chan string, 2)
	interceptor := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedures <- req.Spec().Procedure
			req.Any().(*testpb.Scalars).StringValue = "intercepted"
			return next(ctx, req)
		}
	})
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{}, connect.WithInterceptors(interceptor)))
	server := httptest.NewServer(mux)
	defer server.Close()
	client := testpbconnect.NewFixtureServiceClient(server.Client(), server.URL)
	want := &testpb.Scalars{StringValue: "intercepted", Int64Value: 123}
	response, err := client.Echo(context.Background(), connect.NewRequest(&testpb.Scalars{Int64Value: 123}))
	if err != nil {
		t.Fatalf("Connect Echo() error = %v", err)
	}
	if !proto.Equal(response.Msg, want) {
		t.Errorf("Connect Echo() = %v, want %v", response.Msg, want)
	}
	bridgeResponse := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", `{"int64Value":"123"}`)
	if bridgeResponse.Code != http.StatusOK {
		t.Fatalf("bridge status = %d, body = %s", bridgeResponse.Code, bridgeResponse.Body.String())
	}
	if got := decodeObject(t, bridgeResponse); got["stringValue"] != "intercepted" || got["int64Value"] != "123" {
		t.Errorf("bridge response = %#v", got)
	}
	for range 2 {
		select {
		case got := <-procedures:
			if got != testpbconnect.FixtureServiceEchoProcedure {
				t.Errorf("interceptor procedure = %q", got)
			}
		default:
			t.Fatal("interceptor did not observe both transports")
		}
	}
}

func TestBridgeRequestContext(t *testing.T) {
	for _, test := range []struct {
		name, id, deadline string
		contextID          bool
	}{
		{name: "headers", id: "client-request", deadline: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)},
		{name: "generated"},
		{name: "invalid deadline", deadline: "not-a-date"},
		{name: "non-RFC3339 deadline", deadline: time.Now().Add(time.Minute).UTC().Format("2006-01-02 15:04:05Z07:00")},
		{name: "existing context ID", id: "conflicting-header", contextID: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var gotID string
			var gotDeadline time.Time
			var hasID, hasDeadline bool
			mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(ctx context.Context, req *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
				id, ok := requestid.FromContext(ctx)
				gotID, hasID = id.String(), ok
				gotDeadline, hasDeadline = ctx.Deadline()
				return connect.NewResponse(req.Msg), nil
			}}))
			request := httptest.NewRequest(http.MethodPost, "/rpc/fixture/echo", strings.NewReader("{}"))
			request.Header.Set("X-Request-ID", test.id)
			request.Header.Set("X-Request-Deadline", test.deadline)
			var existingID requestid.RequestID
			if test.contextID {
				existingID = requestid.NewRequestID()
				request = request.WithContext(requestid.NewContext(request.Context(), existingID))
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if !hasID {
				t.Fatal("request ID missing from service context")
			}
			if test.contextID {
				if gotID != existingID.String() {
					t.Errorf("request ID = %q, want existing context ID %q", gotID, existingID.String())
				}
			} else if test.id != "" {
				if gotID != test.id {
					t.Errorf("request ID = %q, want %q", gotID, test.id)
				}
			} else if decoded, err := base64.RawURLEncoding.DecodeString(gotID); err != nil || len(decoded) != 6 {
				t.Errorf("generated request ID = %q, want six base64url bytes", gotID)
			}
			wantDeadline, err := time.Parse(time.RFC3339Nano, test.deadline)
			if err == nil {
				if !hasDeadline || !gotDeadline.Equal(wantDeadline) {
					t.Errorf("deadline = %v (present %v), want %v", gotDeadline, hasDeadline, wantDeadline)
				}
			} else if hasDeadline {
				t.Errorf("unexpected deadline %v", gotDeadline)
			}
		})
	}
}

func TestMergedMetadataAndLegacyDispatch(t *testing.T) {
	codec := v1.MakeStandardEndpointCodec(func(_ context.Context, request struct{}) (string, error) {
		return "legacy response", nil
	}, "legacy method")
	legacy := v1.NewService("alpha", "legacy service", v1.EndpointCodecMap{"zulu": codec, "alpha": codec})
	mux := mountFixture(t, fixtureService("zeta", fixtureHandler{}), legacy)
	response := serve(t, mux, http.MethodGet, "/rpc", "")
	if response.Code != http.StatusOK {
		t.Fatalf("root metadata status = %d", response.Code)
	}
	var root struct {
		Services []v2.Meta `json:"services"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &root); err != nil {
		t.Fatal(err)
	}
	if len(root.Services) != 2 {
		t.Fatalf("services = %v, want two merged services", root.Services)
	}
	if root.Services[0].ServiceName != "alpha" || root.Services[1].ServiceName != "zeta" {
		t.Errorf("service order = %q, %q", root.Services[0].ServiceName, root.Services[1].ServiceName)
	}
	for i, wantMethods := range [][]string{{"alpha", "zulu"}, {"echo", "ping"}} {
		meta := root.Services[i]
		var gotMethods []string
		for _, method := range meta.Interfaces {
			gotMethods = append(gotMethods, method.MethodName)
			if method.MethodTimeout != 60000 {
				t.Errorf("%s.%s timeout = %d", meta.ServiceName, method.MethodName, method.MethodTimeout)
			}
		}
		if !reflect.DeepEqual(gotMethods, wantMethods) {
			t.Errorf("%s methods = %v, want %v", meta.ServiceName, gotMethods, wantMethods)
		}
		response := serve(t, mux, http.MethodGet, "/rpc/"+meta.ServiceName, "")
		if response.Code != http.StatusOK {
			t.Fatalf("service metadata status = %d", response.Code)
		}
		var individual v2.Meta
		if err := json.Unmarshal(response.Body.Bytes(), &individual); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(individual, meta) {
			t.Errorf("individual metadata for %q differs from root", meta.ServiceName)
		}
	}
	response = serve(t, mux, http.MethodPost, "/rpc/alpha/alpha", "{}")
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `"legacy response"` {
		t.Errorf("legacy dispatch = %d %s", response.Code, response.Body.String())
	}
	for _, path := range []string{"/rpc", "/rpc/zeta", "/rpc/alpha"} {
		if response := serve(t, mux, http.MethodPost, path, "{}"); response.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", path, response.Code)
		}
	}
}

func TestPairedMetadataAndDispatch(t *testing.T) {
	codec := v1.MakeStandardEndpointCodec(func(_ context.Context, request struct {
		Value string `json:"value"`
	}) (string, error) {
		if request.Value == "fail" {
			return "", errors.New("legacy failure")
		}
		return "legacy " + request.Value, nil
	}, "legacy echo")
	legacy := v1.NewService("fixture", "original help", v1.EndpointCodecMap{"echo": codec})
	service := fixtureService("fixture", fixtureHandler{})
	service.NoBridge = true
	others := []*v1.Service{v1.NewService("zeta", "last", nil), legacy, v1.NewService("alpha", "first", nil)}
	mux := mountFixture(t, service, others...)
	baseline := http.NewServeMux()
	for _, original := range others {
		original.MountHandlers(log.NewNopLogger(), baseline)
	}

	for _, path := range []string{"/rpc", "/rpc/fixture"} {
		t.Run(path, func(t *testing.T) {
			response := serve(t, mux, http.MethodGet, path, "")
			if response.Code != http.StatusOK {
				t.Fatalf("metadata status = %d", response.Code)
			}
			meta := decodeObject(t, response)
			if path == "/rpc" {
				services := meta["services"].([]any)
				if len(services) != 3 {
					t.Fatalf("root contains %d services, want 3", len(services))
				}
				for i, name := range []string{"alpha", "fixture", "zeta"} {
					entry := services[i].(map[string]any)
					if entry["serviceName"] != name {
						t.Fatalf("root service %d = %v, want %q", i, entry["serviceName"], name)
					}
					if name != "fixture" {
						want := decodeObject(t, serve(t, baseline, http.MethodGet, "/rpc/"+name, ""))
						if !reflect.DeepEqual(entry, want) {
							t.Errorf("unpaired legacy metadata changed: got %#v, want %#v", entry, want)
						}
					}
				}
				meta = services[1].(map[string]any)
			}
			if path == "/rpc/fixture" {
				want := serve(t, baseline, http.MethodGet, path, "")
				if !bytes.Equal(response.Body.Bytes(), want.Body.Bytes()) {
					t.Errorf("paired JSON differs from v1: got %s, want %s", response.Body.Bytes(), want.Body.Bytes())
				}
			}
			want := decodeObject(t, serve(t, baseline, http.MethodGet, "/rpc/fixture", ""))
			if !reflect.DeepEqual(meta, want) {
				t.Errorf("legacy metadata changed: got %#v, want %#v", meta, want)
			}
		})
	}
	for _, test := range []struct {
		method, path, body string
	}{
		{http.MethodPost, "/rpc/fixture/echo", `{"value":"ok","unknown":true}`},
		{http.MethodPost, "/rpc/fixture/echo", `{"value":"fail"}`},
		{http.MethodPost, "/rpc/fixture/echo", "{"},
		{http.MethodPost, "/rpc/fixture/ping", "{}"},
		{http.MethodGet, "/rpc/fixture/echo", ""},
	} {
		t.Run(test.method+test.path+test.body, func(t *testing.T) {
			got := serve(t, mux, test.method, test.path, test.body)
			want := serve(t, baseline, test.method, test.path, test.body)
			if got.Code != want.Code || got.Body.String() != want.Body.String() || !reflect.DeepEqual(got.Header(), want.Header()) {
				t.Errorf("paired legacy response = %d %v %q, want %d %v %q", got.Code, got.Header(), got.Body.String(), want.Code, want.Header(), want.Body.String())
			}
		})
	}
	server := httptest.NewServer(mux)
	defer server.Close()
	client := testpbconnect.NewFixtureServiceClient(server.Client(), server.URL)
	response, err := client.Echo(context.Background(), connect.NewRequest(&testpb.Scalars{StringValue: "connect"}))
	if err != nil {
		t.Fatalf("Connect Echo: %v", err)
	}
	if response.Msg.StringValue != "connect" {
		t.Errorf("Connect response = %q", response.Msg.StringValue)
	}
}

func TestNoBridgeWithoutLegacy(t *testing.T) {
	service := fixtureService("fixture", fixtureHandler{})
	service.NoBridge = true
	recorded := &recordingMux{}
	if err := v2.MountHandlers(log.NewNopLogger(), recorded, []v2.Service{service}); err != nil {
		t.Fatal(err)
	}
	for _, path := range recorded.paths {
		if strings.HasPrefix(path, "/rpc/fixture/") {
			t.Errorf("NoBridge mounted legacy method route %q", path)
		}
	}
	mux := mountFixture(t, service)
	bridged := mountFixture(t, fixtureService("fixture", fixtureHandler{}))
	for _, path := range []string{"/rpc", "/rpc/fixture"} {
		got := serve(t, mux, http.MethodGet, path, "")
		want := serve(t, bridged, http.MethodGet, path, "")
		if got.Code != want.Code || got.Body.String() != want.Body.String() {
			t.Errorf("%s metadata differs with NoBridge", path)
		}
	}
	if response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", "{}"); response.Code != http.StatusNotFound {
		t.Errorf("disabled bridge status = %d, want 404", response.Code)
	}
	request := httptest.NewRequest(http.MethodPost, testpbconnect.FixtureServiceEchoProcedure, strings.NewReader(`{"stringValue":"connect"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || decodeObject(t, response)["stringValue"] != "connect" {
		t.Errorf("standalone Connect = %d %s", response.Code, response.Body.String())
	}
}

type recordingMux struct{ paths []string }

func (mux *recordingMux) Handle(path string, _ http.Handler) {
	mux.paths = append(mux.paths, path)
}

func TestMountValidationIsAtomic(t *testing.T) {
	valid := fixtureService("fixture", fixtureHandler{})
	legacy := v1.NewService("legacy", "", nil)
	noBridge := valid
	noBridge.NoBridge = true
	pairedLegacy := v1.NewService("fixture", "", nil)
	validPath, validHandler := valid.Handler()
	withPath := func(path string) v2.Service {
		return v2.Service{Name: "bad", Handler: func(...connect.HandlerOption) (string, http.Handler) {
			return path, validHandler
		}}
	}
	for _, test := range []struct {
		name      string
		services  []v2.Service
		legacy    []*v1.Service
		wantError string
	}{
		{name: "duplicate proto", services: []v2.Service{valid, valid}, wantError: "duplicate legacy service name"},
		{name: "duplicate legacy", services: []v2.Service{valid}, legacy: []*v1.Service{legacy, legacy}, wantError: "duplicate legacy service name"},
		{name: "mixed duplicate", services: []v2.Service{valid}, legacy: []*v1.Service{v1.NewService("fixture", "", nil)}, wantError: "duplicate legacy service name"},
		{name: "duplicate unpaired no bridge", services: []v2.Service{noBridge, noBridge}, wantError: "duplicate legacy service name"},
		{name: "duplicate paired no bridge", services: []v2.Service{noBridge, noBridge}, legacy: []*v1.Service{pairedLegacy}, wantError: "duplicate legacy service name"},
		{name: "duplicate legacy with pair", services: []v2.Service{noBridge}, legacy: []*v1.Service{pairedLegacy, pairedLegacy}, wantError: "duplicate legacy service name"},
		{name: "bridge then no bridge", services: []v2.Service{valid, noBridge}, wantError: "duplicate legacy service name"},
		{name: "no bridge then bridge", services: []v2.Service{noBridge, valid}, wantError: "duplicate legacy service name"},
		{name: "paired bridge then no bridge", services: []v2.Service{valid, noBridge}, legacy: []*v1.Service{pairedLegacy}, wantError: "duplicate legacy service name"},
		{name: "paired no bridge then bridge", services: []v2.Service{noBridge, valid}, legacy: []*v1.Service{pairedLegacy}, wantError: "duplicate legacy service name"},
		{name: "nil legacy", services: []v2.Service{valid}, legacy: []*v1.Service{nil}},
		{name: "unregistered service", services: []v2.Service{withPath("/unregistered.FixtureService/")}},
		{name: "message instead of service", services: []v2.Service{withPath("/loke.rpc.test.v2.Scalars/")}},
		{name: "missing factory", services: []v2.Service{{Name: "bad"}}},
		{name: "missing handler", services: []v2.Service{{Name: "bad", Handler: func(...connect.HandlerOption) (string, http.Handler) {
			return validPath, nil
		}}}},
		{name: "missing path", services: []v2.Service{withPath("")}},
		{name: "missing leading slash", services: []v2.Service{withPath(strings.TrimPrefix(validPath, "/"))}},
		{name: "missing trailing slash", services: []v2.Service{withPath(strings.TrimSuffix(validPath, "/"))}},
		{name: "method path", services: []v2.Service{withPath(validPath + "Echo")}},
		{name: "empty name", services: []v2.Service{fixtureService("", fixtureHandler{})}},
		{name: "slash name", services: []v2.Service{fixtureService("bad/name", fixtureHandler{})}},
		{name: "query name", services: []v2.Service{fixtureService("bad?name", fixtureHandler{})}},
		{name: "fragment name", services: []v2.Service{fixtureService("bad#name", fixtureHandler{})}},
		{name: "brace name", services: []v2.Service{fixtureService("bad{name}", fixtureHandler{})}},
		{name: "space name", services: []v2.Service{fixtureService("bad name", fixtureHandler{})}},
		{name: "dot name", services: []v2.Service{fixtureService(".", fixtureHandler{})}},
		{name: "dot dot name", services: []v2.Service{fixtureService("..", fixtureHandler{})}},
		{name: "duplicate Connect path", services: []v2.Service{valid, fixtureService("another", fixtureHandler{})}},
	} {
		t.Run(test.name, func(t *testing.T) {
			mux := &recordingMux{}
			if err := v2.MountHandlers(log.NewNopLogger(), mux, test.services, test.legacy...); err == nil {
				t.Error("MountHandlers() succeeded, want validation error")
			} else if !strings.Contains(err.Error(), test.wantError) {
				t.Errorf("MountHandlers() error = %v, want %q", err, test.wantError)
			}
			if len(mux.paths) != 0 {
				t.Errorf("failed mount registered routes %v", mux.paths)
			}
		})
	}
}

func metricValue(t *testing.T, family, handler, failure string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, candidate := range families {
		if candidate.GetName() != family {
			continue
		}
		for _, metric := range candidate.Metric {
			labels := map[string]string{}
			for _, pair := range metric.Label {
				labels[pair.GetName()] = pair.GetValue()
			}
			if labels["handler"] != handler || (failure != "" && labels["type"] != failure) {
				continue
			}
			if family == "http_rpc_request_duration_seconds" {
				return float64(metric.GetHistogram().GetSampleCount())
			}
			return metric.GetCounter().GetValue()
		}
	}
	return 0
}

func TestBridgeUsesLegacyMetrics(t *testing.T) {
	mux := mountFixture(t, fixtureService("bridge-metrics", fixtureHandler{echo: func(_ context.Context, req *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
		if req.Msg.StringValue == "fail" {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("missing"))
		}
		return connect.NewResponse(req.Msg), nil
	}}))
	const handler = "bridge-metrics.echo"
	beforeCount := metricValue(t, "http_rpc_requests_total", handler, "")
	beforeDuration := metricValue(t, "http_rpc_request_duration_seconds", handler, "")
	beforeFailure := metricValue(t, "http_rpc_failures_total", handler, "not_found")
	beforeInvalid := metricValue(t, "http_rpc_failures_total", handler, "invalid_argument")
	for _, test := range []struct {
		body   string
		status int
	}{
		{body: "{}", status: 200},
		{body: `{"stringValue":"fail"}`, status: 400},
		{body: "{", status: 400},
	} {
		if response := serve(t, mux, http.MethodPost, "/rpc/bridge-metrics/echo", test.body); response.Code != test.status {
			t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body.String())
		}
	}
	for _, test := range []struct {
		family, failure string
		before, delta   float64
	}{
		{family: "http_rpc_requests_total", before: beforeCount, delta: 3},
		{family: "http_rpc_request_duration_seconds", before: beforeDuration, delta: 3},
		{family: "http_rpc_failures_total", failure: "not_found", before: beforeFailure, delta: 1},
		{family: "http_rpc_failures_total", failure: "invalid_argument", before: beforeInvalid, delta: 1},
	} {
		if got := metricValue(t, test.family, handler, test.failure) - test.before; got != test.delta {
			t.Errorf("%s{%s,%s} delta = %v, want %v", test.family, handler, test.failure, got, test.delta)
		}
	}
}

func TestBridgePreservesReadLimitOnRawBody(t *testing.T) {
	calls := 0
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(_ context.Context, req *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
		calls++
		return connect.NewResponse(req.Msg), nil
	}}, connect.WithReadMaxBytes(64)))
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "unknown fields", body: `{"ignored":"` + strings.Repeat("x", 128) + `"}`},
		{name: "leading whitespace", body: strings.Repeat(" ", 128) + "{}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			nativeRequest := httptest.NewRequest(http.MethodPost, testpbconnect.FixtureServiceEchoProcedure, strings.NewReader(test.body))
			nativeRequest.Header.Set("Content-Type", "application/json")
			nativeRequest.Header.Set("Connect-Protocol-Version", "1")
			nativeResponse := httptest.NewRecorder()
			mux.ServeHTTP(nativeResponse, nativeRequest)
			nativeError := decodeObject(t, nativeResponse)
			if nativeError["code"] != "resource_exhausted" {
				t.Fatalf("Connect error = %#v, want resource_exhausted", nativeError)
			}
			bridgeResponse := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", test.body)
			if bridgeResponse.Code != http.StatusBadRequest {
				t.Errorf("bridge status = %d, want 400", bridgeResponse.Code)
			}
			want := map[string]any{
				"message": nativeError["message"],
				"code":    "resource_exhausted",
				"type":    "resource_exhausted",
			}
			if got := decodeObject(t, bridgeResponse); !reflect.DeepEqual(got, want) {
				t.Errorf("bridge error = %#v, want %#v", got, want)
			}
		})
	}
	if calls != 0 {
		t.Errorf("oversized bodies invoked Echo %d times", calls)
	}
}

func TestBridgePreservesResponseMetadata(t *testing.T) {
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(_ context.Context, req *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
		if req.Msg.StringValue == "fail" {
			err := connect.NewError(connect.CodeNotFound, errors.New("missing"))
			err.Meta().Set("X-Fixture-Meta", "error metadata")
			err.Meta().Add("X-Fixture-Multi", "first")
			err.Meta().Add("X-Fixture-Multi", "second")
			err.Meta().Set("Accept-Encoding", "gzip")
			err.Meta().Set("Trailer-X-Fixture-Internal", "transport metadata")
			return nil, err
		}
		response := connect.NewResponse(req.Msg)
		response.Header().Set("X-Fixture-Meta", "success metadata")
		response.Header().Add("X-Fixture-Multi", "first")
		response.Header().Add("X-Fixture-Multi", "second")
		response.Header().Set("Accept-Encoding", "gzip")
		response.Header().Set("Trailer-X-Fixture-Internal", "transport metadata")
		response.Trailer().Set("X-Fixture-Trailer", "transport trailer")
		return response, nil
	}}))
	for _, test := range []struct {
		name     string
		body     string
		status   int
		metadata string
	}{
		{name: "success", body: "{}", status: 200, metadata: "success metadata"},
		{name: "error", body: `{"stringValue":"fail"}`, status: 400, metadata: "error metadata"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", test.body)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
			if got := response.Header().Get("X-Fixture-Meta"); got != test.metadata {
				t.Errorf("metadata header = %q, want %q", got, test.metadata)
			}
			if got, want := response.Header().Values("X-Fixture-Multi"), []string{"first", "second"}; !reflect.DeepEqual(got, want) {
				t.Errorf("repeated metadata = %v, want %v", got, want)
			}
			if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q, want legacy JSON content type", got)
			}
			for name := range response.Header() {
				if strings.EqualFold(name, "Accept-Encoding") || strings.HasPrefix(strings.ToLower(name), "trailer-") {
					t.Errorf("transport response header %q leaked to legacy response", name)
				}
			}
		})
	}
}

func TestBridgePreservesApplicationUnmarshalError(t *testing.T) {
	const message = "unmarshal message: downstream payload was rejected"
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(context.Context, *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New(message))
	}}))
	response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", "{}")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	want := map[string]any{"message": message, "code": "invalid_argument", "type": "invalid_argument"}
	if got := decodeObject(t, response); !reflect.DeepEqual(got, want) {
		t.Errorf("error = %#v, want %#v", got, want)
	}
}

func TestBridgeDecodesGzipWithHandlerOptions(t *testing.T) {
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{}, connect.WithReadMaxBytes(1024)))
	var body bytes.Buffer
	compressor := gzip.NewWriter(&body)
	if _, err := compressor.Write([]byte(`{"stringValue":"compressed","int64Value":"42","ignored":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/rpc/fixture/echo", &body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Encoding", "gzip")
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if got := decodeObject(t, response); got["stringValue"] != "compressed" || got["int64Value"] != "42" {
		t.Errorf("response = %#v, want decoded gzip message", got)
	}
	if encoding := response.Header().Get("Content-Encoding"); encoding != "" {
		t.Errorf("Content-Encoding = %q, want uncompressed legacy response", encoding)
	}
}

func TestGeneratedFixtureMetadataHelpFallback(t *testing.T) {
	descriptor := testpb.File_lokerpc_v2_internal_testpb_fixture_proto
	if descriptor.SourceLocations().Len() != 0 {
		t.Fatal("generated fixture unexpectedly retains source locations")
	}
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{}))
	response := serve(t, mux, http.MethodGet, "/rpc/fixture", "")
	if response.Code != http.StatusOK {
		t.Fatalf("metadata status = %d, want 200", response.Code)
	}
	var meta v2.Meta
	if err := json.Unmarshal(response.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Help != "fixture service" {
		t.Errorf("service help = %q, want fallback %q", meta.Help, "fixture service")
	}
	if len(meta.Interfaces) != 2 {
		t.Fatalf("interfaces = %d, want 2", len(meta.Interfaces))
	}
	for _, method := range meta.Interfaces {
		if want := method.MethodName + " method"; method.Help != want {
			t.Errorf("%s help = %q, want fallback %q", method.MethodName, method.Help, want)
		}
	}
}

func TestBridgeLogsFailureLabels(t *testing.T) {
	var records [][]any
	logger := log.LoggerFunc(func(keyvals ...any) error {
		records = append(records, append([]any(nil), keyvals...))
		return nil
	})
	service := fixtureService("fixture", fixtureHandler{echo: func(context.Context, *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("fixture missing"))
	}})
	mux := http.NewServeMux()
	if err := v2.MountHandlers(logger, mux, []v2.Service{service}); err != nil {
		t.Fatal(err)
	}
	response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", "{}")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	if len(records) != 1 {
		t.Fatalf("log records = %d, want one failure record", len(records))
	}
	fields := make(map[string]any)
	for i := 0; i+1 < len(records[0]); i += 2 {
		key, ok := records[0][i].(string)
		if !ok {
			t.Fatalf("log key = %v, want string", records[0][i])
		}
		fields[key] = records[0][i+1]
	}
	for key, want := range map[string]string{
		"level": "error", "rpc_service": "fixture", "method": "echo", "err": "fixture missing",
	} {
		if got := fmt.Sprint(fields[key]); got != want {
			t.Errorf("log %s = %q, want %q", key, got, want)
		}
	}
}

func TestMountLegacyNameGrammar(t *testing.T) {
	for _, test := range []struct {
		name  string
		valid bool
	}{
		{name: "fixture", valid: true},
		{name: "7fixture", valid: true},
		{name: "Fixture_7.v2-test", valid: true},
		{name: "-fixture"},
		{name: "_fixture"},
		{name: ".fixture"},
		{name: "fixturé"},
		{name: "fixture!"},
		{name: "fixture:other"},
		{name: "fixture@other"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, transport := range []string{"proto", "legacy"} {
				t.Run(transport, func(t *testing.T) {
					mux := &recordingMux{}
					var services []v2.Service
					var legacy []*v1.Service
					if transport == "proto" {
						services = []v2.Service{fixtureService(test.name, fixtureHandler{})}
					} else {
						legacy = []*v1.Service{v1.NewService(test.name, "", nil)}
					}
					err := v2.MountHandlers(log.NewNopLogger(), mux, services, legacy...)
					if test.valid {
						if err != nil {
							t.Fatalf("MountHandlers() error = %v, want valid name", err)
						}
						if len(mux.paths) == 0 {
							t.Error("valid name registered no routes")
						}
					} else {
						if err == nil {
							t.Error("MountHandlers() succeeded, want invalid name error")
						}
						if len(mux.paths) != 0 {
							t.Errorf("invalid name registered routes %v", mux.paths)
						}
					}
				})
			}
		})
	}
}

func TestBridgeGzipRespectsInflatedReadLimit(t *testing.T) {
	calls := 0
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(_ context.Context, req *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
		calls++
		return connect.NewResponse(req.Msg), nil
	}}, connect.WithReadMaxBytes(128)))
	var body bytes.Buffer
	compressor := gzip.NewWriter(&body)
	if _, err := compressor.Write([]byte(`{"stringValue":"` + strings.Repeat("x", 1024) + `"}`)); err != nil {
		t.Fatal(err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatal(err)
	}
	if body.Len() >= 128 {
		t.Fatalf("compressed fixture size = %d, want below read limit 128", body.Len())
	}
	request := httptest.NewRequest(http.MethodPost, "/rpc/fixture/echo", &body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Encoding", "gzip")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
	}
	if got := decodeObject(t, response); got["code"] != "resource_exhausted" || got["type"] != "resource_exhausted" {
		t.Errorf("error = %#v, want resource_exhausted", got)
	}
	if calls != 0 {
		t.Errorf("oversized inflated body invoked Echo %d times", calls)
	}
}

func TestBridgeRejectsUnsupportedContentEncoding(t *testing.T) {
	calls := 0
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(_ context.Context, req *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
		calls++
		return connect.NewResponse(req.Msg), nil
	}}, connect.WithCompression(
		"customgzip",
		func() connect.Decompressor { return &gzip.Reader{} },
		func() connect.Compressor { return gzip.NewWriter(nil) },
	)))
	for _, encoding := range []string{"br", "customgzip"} {
		t.Run(encoding, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/rpc/fixture/echo", strings.NewReader("{}"))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Content-Encoding", encoding)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != http.StatusUnsupportedMediaType {
				t.Fatalf("status = %d, want 415: %s", response.Code, response.Body.String())
			}
			want := map[string]any{"message": "Unsupported Content-Encoding"}
			if got := decodeObject(t, response); !reflect.DeepEqual(got, want) {
				t.Errorf("error = %#v, want %#v", got, want)
			}
		})
	}
	if calls != 0 {
		t.Errorf("unsupported encodings invoked Echo %d times", calls)
	}
}

type failedBridgeBody struct{}

func (failedBridgeBody) Read(body []byte) (int, error) {
	return copy(body, "partial stale request"), errors.New("request body failed")
}

func TestBridgeRequestBufferReuse(t *testing.T) {
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{}))
	for _, text := range []string{strings.Repeat("x", 10000), "", "short"} {
		response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", `{"stringValue":"`+text+`"}`)
		if response.Code != http.StatusOK || decodeObject(t, response)["stringValue"] != text {
			t.Fatalf("echo = %d %s", response.Code, response.Body.String())
		}
		request := httptest.NewRequest(http.MethodPost, "/rpc/fixture/echo", failedBridgeBody{})
		failure := httptest.NewRecorder()
		mux.ServeHTTP(failure, request)
		if failure.Code != http.StatusBadRequest || decodeObject(t, failure)["message"] != "Invalid JSON" {
			t.Fatalf("read failure = %d %s", failure.Code, failure.Body.String())
		}
	}
}
