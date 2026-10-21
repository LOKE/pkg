package lokerpc_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"connectrpc.com/connect"
	v1 "github.com/LOKE/pkg/lokerpc"
	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb"
	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb/testpbconnect"
	"github.com/LOKE/pkg/lokerpc/v2/rpcv1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

type codedFixtureError struct {
	message, code, namespace, kind, instance string
	expose                                   bool
	meta                                     map[string]any
}

func (e codedFixtureError) Error() string             { return e.message }
func (e codedFixtureError) ErrorCode() string         { return e.code }
func (e codedFixtureError) Public() bool              { return e.expose }
func (e codedFixtureError) ErrorID() string           { return e.instance }
func (e codedFixtureError) ErrorType() string         { return e.kind }
func (e codedFixtureError) ErrorNamespace() string    { return e.namespace }
func (e codedFixtureError) ErrorMeta() map[string]any { return e.meta }

type helpfulFixtureError struct {
	codedFixtureError
	help string
}

func (e helpfulFixtureError) ErrorHelp() string { return e.help }

type minimalCodedFixtureError struct{}

func (minimalCodedFixtureError) Error() string     { return "minimal failure" }
func (minimalCodedFixtureError) ErrorCode() string { return "minimal_failure" }

func callErrorFixture(t *testing.T, handlerError error) *connect.Error {
	t.Helper()
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(context.Context, *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
		return nil, handlerError
	}}))
	server := httptest.NewServer(mux)
	defer server.Close()
	client := testpbconnect.NewFixtureServiceClient(server.Client(), server.URL)
	_, err := client.Echo(context.Background(), connect.NewRequest(&testpb.Scalars{}))
	var result *connect.Error
	if !errors.As(err, &result) {
		t.Fatalf("Connect Echo() error = %v, want *connect.Error", err)
	}
	return result
}

func fixtureMeta(t *testing.T, fields map[string]any) *structpb.Struct {
	t.Helper()
	result, err := structpb.NewStruct(fields)
	if err != nil {
		t.Fatalf("NewStruct() error = %v", err)
	}
	return result
}

func TestConnectStandardErrorDetail(t *testing.T) {
	meta := map[string]any{
		"productId": "p1",
		"nested":    map[string]any{"available": false, "count": float64(3), "items": []any{"one", nil, true}},
	}
	failure := helpfulFixtureError{
		codedFixtureError: codedFixtureError{
			message: "product missing", code: "product_not_found", namespace: "menu",
			kind: "https://errors.loke.global/menu/product_not_found", instance: "01JERRORINSTANCE",
			expose: true, meta: meta,
		},
		help: "Choose another product",
	}
	got := callErrorFixture(t, connect.NewError(connect.CodeNotFound, fmt.Errorf("lookup: %w", failure)))
	if got.Code() != connect.CodeNotFound || got.Message() != "lookup: product missing" {
		t.Errorf("Connect error = %s %q, want not_found lookup: product missing", got.Code(), got.Message())
	}
	if len(got.Details()) != 1 {
		t.Fatalf("Connect details count = %d, want 1", len(got.Details()))
	}
	detail, ok := rpcv1.FromError(got)
	want := &rpcv1.Error{
		Code: failure.code, Namespace: failure.namespace, Type: failure.kind, Instance: failure.instance,
		Expose: true, Help: failure.help, Meta: fixtureMeta(t, meta),
	}
	if !ok || !proto.Equal(detail, want) {
		t.Errorf("ErrorDetail() = %v, %v; want %v, true", detail, ok, want)
	}
}

func TestConnectPreservesExistingStandardErrorDetail(t *testing.T) {
	want := &rpcv1.Error{
		Code: "existing_code", Namespace: "original", Type: "existing_type", Instance: "existing_instance",
		Expose: true, Help: "original help", Meta: fixtureMeta(t, map[string]any{"nested": map[string]any{"kept": true}}),
	}
	attached, err := connect.NewErrorDetail(want)
	if err != nil {
		t.Fatal(err)
	}
	failure := connect.NewError(connect.CodeAborted, codedFixtureError{message: "failure", code: "replacement_code"})
	failure.AddDetail(attached)
	got := callErrorFixture(t, failure)
	if got.Code() != connect.CodeAborted {
		t.Errorf("Connect code = %s, want aborted", got.Code())
	}
	if len(got.Details()) != 1 {
		t.Fatalf("Connect details count = %d, want exactly 1", len(got.Details()))
	}
	if detail, ok := rpcv1.FromError(got); !ok || !proto.Equal(detail, want) {
		t.Errorf("ErrorDetail() = %v, %v; want unchanged %v", detail, ok, want)
	}
	if !proto.Equal(want, mustErrorDetailValue(t, attached)) {
		t.Error("original attached detail was mutated")
	}
}

func mustErrorDetailValue(t *testing.T, detail *connect.ErrorDetail) proto.Message {
	t.Helper()
	value, err := detail.Value()
	if err != nil {
		t.Fatalf("detail.Value() error = %v", err)
	}
	return value
}

func TestConcurrentRequestsDoNotMutateSharedConnectError(t *testing.T) {
	meta := map[string]any{"nested": map[string]any{"kept": true}}
	shared := connect.NewError(connect.CodeNotFound, codedFixtureError{
		message: "shared failure", code: "shared_code", kind: "shared_type", meta: meta,
	})
	shared.Meta().Add("X-Shared", "first")
	shared.Meta().Add("X-Shared", "second")
	wantHeaders := shared.Meta().Clone()
	unrelated, err := connect.NewErrorDetail(&emptypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	shared.AddDetail(unrelated)
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(context.Context, *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
		return nil, shared
	}}))
	server := httptest.NewServer(mux)
	defer server.Close()
	client := testpbconnect.NewFixtureServiceClient(server.Client(), server.URL)
	want := &rpcv1.Error{Code: "shared_code", Type: "shared_type", Meta: fixtureMeta(t, meta)}
	const requests = 16
	start := make(chan struct{})
	results := make(chan error, requests)
	for range requests {
		go func() {
			<-start
			_, err := client.Echo(context.Background(), connect.NewRequest(&testpb.Scalars{}))
			results <- err
		}()
	}
	close(start)
	for range requests {
		err := <-results
		var got *connect.Error
		if !errors.As(err, &got) {
			t.Errorf("Connect Echo() error = %v, want *connect.Error", err)
			continue
		}
		if got.Code() != connect.CodeNotFound || len(got.Details()) != 2 {
			t.Errorf("Connect error = %s with %d details, want not_found with 2", got.Code(), len(got.Details()))
		}
		if detail, ok := rpcv1.FromError(got); !ok || !proto.Equal(detail, want) {
			t.Errorf("ErrorDetail() = %v, %v; want %v, true", detail, ok, want)
		}
	}
	if got := shared.Details(); len(got) != 1 || got[0] != unrelated {
		t.Errorf("shared error details = %v, want original unrelated detail only", got)
	}
	if !reflect.DeepEqual(shared.Meta(), wantHeaders) {
		t.Errorf("shared error headers = %v, want %v", shared.Meta(), wantHeaders)
	}
	if !proto.Equal(fixtureMeta(t, meta), want.Meta) {
		t.Errorf("shared error metadata = %v, want %v", meta, want.Meta)
	}
}

func TestStandardDetailWrapsCallerInterceptorErrors(t *testing.T) {
	interceptor := connect.UnaryInterceptorFunc(func(connect.UnaryFunc) connect.UnaryFunc {
		return func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
			return nil, minimalCodedFixtureError{}
		}
	})
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{}, connect.WithInterceptors(interceptor)))
	server := httptest.NewServer(mux)
	defer server.Close()
	client := testpbconnect.NewFixtureServiceClient(server.Client(), server.URL)
	_, err := client.Echo(context.Background(), connect.NewRequest(&testpb.Scalars{}))
	want := &rpcv1.Error{Code: "minimal_failure", Type: "minimal_failure"}
	if detail, ok := rpcv1.FromError(err); !ok || !proto.Equal(detail, want) {
		t.Errorf("interceptor ErrorDetail() = %v, %v; want %v, true", detail, ok, want)
	}
	response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", "{}")
	wantBody := `{"message":"minimal failure","instance":"","code":"minimal_failure","type":"minimal_failure"}`
	if response.Code != http.StatusBadRequest || response.Body.String() != wantBody {
		t.Errorf("bridge interceptor error = %d %q, want 400 %q", response.Code, response.Body.String(), wantBody)
	}
}

func TestConnectErrorsWithoutCodeHaveNoStandardDetail(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		code connect.Code
	}{
		{name: "ordinary", err: errors.New("failure"), code: connect.CodeUnknown},
		{name: "Connect", err: connect.NewError(connect.CodeInvalidArgument, errors.New("failure")), code: connect.CodeInvalidArgument},
		{name: "empty code", err: codedFixtureError{message: "failure"}, code: connect.CodeUnknown},
		{name: "Connect with empty code", err: connect.NewError(connect.CodeNotFound, codedFixtureError{message: "failure"}), code: connect.CodeNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := callErrorFixture(t, test.err)
			if got.Code() != test.code {
				t.Errorf("Connect code = %s, want %s", got.Code(), test.code)
			}
			if len(got.Details()) != 0 {
				t.Errorf("Connect details = %v, want none", got.Details())
			}
			if detail, ok := rpcv1.FromError(got); ok || detail != nil {
				t.Errorf("ErrorDetail() = %v, %v; want nil, false", detail, ok)
			}
		})
	}
}

func TestPlainCodedErrorUsesUnknownConnectCode(t *testing.T) {
	got := callErrorFixture(t, fmt.Errorf("outer: %w", minimalCodedFixtureError{}))
	if got.Code() != connect.CodeUnknown || got.Message() != "outer: minimal failure" {
		t.Errorf("Connect error = %s %q, want unknown outer: minimal failure", got.Code(), got.Message())
	}
	want := &rpcv1.Error{Code: "minimal_failure", Type: "minimal_failure"}
	if detail, ok := rpcv1.FromError(got); !ok || !proto.Equal(detail, want) {
		t.Errorf("ErrorDetail() = %v, %v; want %v, true", detail, ok, want)
	}
}

func TestBridgeStandardErrorLegacyJSON(t *testing.T) {
	for _, test := range []struct {
		name    string
		failure error
		want    string
	}{
		{
			name: "public namespaced error",
			failure: connect.NewError(connect.CodeInternal, helpfulFixtureError{
				codedFixtureError: codedFixtureError{
					message: "product missing", instance: "01JERRORINSTANCE", expose: true,
					code: "product_not_found", namespace: "menu", kind: "menu/product_not_found",
					meta: map[string]any{"productId": "p1", "nested": map[string]any{"count": float64(2), "values": []any{true, nil}}},
				},
				help: "must not appear in legacy JSON",
			}),
			want: `{"message":"product missing","instance":"01JERRORINSTANCE","expose":true,"code":"product_not_found","namespace":"menu","type":"menu/product_not_found","nested":{"count":2,"values":[true,null]},"productId":"p1"}`,
		},
		{
			name:    "private unnamespaced error",
			failure: codedFixtureError{message: "private failure", instance: "private_instance", code: "private_code", kind: "private_type"},
			want:    `{"message":"private failure","instance":"private_instance","code":"private_code","type":"private_type"}`,
		},
		{
			name:    "optional fields absent",
			failure: minimalCodedFixtureError{},
			want:    `{"message":"minimal failure","instance":"","code":"minimal_failure","type":"minimal_failure"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(context.Context, *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
				return nil, test.failure
			}}))
			got := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", "{}")
			if got.Code != http.StatusBadRequest {
				t.Errorf("bridge status = %d, want 400", got.Code)
			}
			if got.Body.String() != test.want {
				t.Errorf("bridge body = %q, want %q", got.Body.String(), test.want)
			}
		})
	}
}

func TestErrorDetail(t *testing.T) {
	want := &rpcv1.Error{Code: "kept", Type: "kept", Instance: "instance"}
	standard, err := connect.NewErrorDetail(want)
	if err != nil {
		t.Fatal(err)
	}
	other, err := connect.NewErrorDetail(&emptypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	without := connect.NewError(connect.CodeUnknown, errors.New("failure"))
	without.AddDetail(other)
	with := connect.NewError(connect.CodeNotFound, errors.New("failure"))
	with.AddDetail(other)
	with.AddDetail(standard)
	for _, test := range []struct {
		name string
		err  error
		want *rpcv1.Error
	}{
		{name: "nil"},
		{name: "ordinary error", err: errors.New("failure")},
		{name: "coded but not Connect", err: minimalCodedFixtureError{}},
		{name: "Connect without details", err: connect.NewError(connect.CodeUnknown, errors.New("failure"))},
		{name: "nonstandard detail", err: without},
		{name: "standard after unrelated detail", err: with, want: want},
		{name: "wrapped Connect error", err: fmt.Errorf("outer: %w", with), want: want},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := rpcv1.FromError(test.err)
			if ok != (test.want != nil) || !proto.Equal(got, test.want) {
				t.Errorf("ErrorDetail() = %v, %v; want %v, %v", got, ok, test.want, test.want != nil)
			}
		})
	}
}

type wrappingCodedFixtureError struct {
	message, code string
	cause         error
}

func (e wrappingCodedFixtureError) Error() string     { return e.message }
func (e wrappingCodedFixtureError) ErrorCode() string { return e.code }
func (e wrappingCodedFixtureError) Unwrap() error     { return e.cause }

func TestConnectPreservesNestedStandardDetail(t *testing.T) {
	want := &rpcv1.Error{
		Code: "downstream_code", Type: "downstream_type", Namespace: "downstream",
		Instance: "downstream_instance", Expose: true, Help: "downstream help",
		Meta: fixtureMeta(t, map[string]any{"nested": map[string]any{"kept": true}}),
	}
	detail, err := connect.NewErrorDetail(want)
	if err != nil {
		t.Fatal(err)
	}
	downstream := connect.NewError(connect.CodeNotFound, minimalCodedFixtureError{})
	downstream.AddDetail(detail)
	got := callErrorFixture(t, connect.NewError(connect.CodeFailedPrecondition, downstream))
	if got.Code() != connect.CodeFailedPrecondition {
		t.Errorf("Connect code = %s, want failed_precondition", got.Code())
	}
	if len(got.Details()) != 1 {
		t.Errorf("Connect details count = %d, want 1", len(got.Details()))
	}
	if detail, ok := rpcv1.FromError(got); !ok || !proto.Equal(detail, want) {
		t.Errorf("ErrorDetail() = %v, %v; want unchanged %v, true", detail, ok, want)
	}
}

func TestEmptyCodedWrapperUsesCodedCause(t *testing.T) {
	failure := wrappingCodedFixtureError{
		message: "outer message",
		cause: helpfulFixtureError{
			codedFixtureError: codedFixtureError{
				message: "inner message", code: "inner_code", kind: "inner_type", namespace: "inner",
				instance: "inner_instance", expose: true, meta: map[string]any{"kept": true},
			},
			help: "inner help",
		},
	}
	got := callErrorFixture(t, failure)
	want := &rpcv1.Error{
		Code: "inner_code", Type: "inner_type", Namespace: "inner", Instance: "inner_instance",
		Expose: true, Help: "inner help", Meta: fixtureMeta(t, map[string]any{"kept": true}),
	}
	if got.Code() != connect.CodeUnknown || got.Message() != "outer message" {
		t.Errorf("Connect error = %s %q, want unknown outer message", got.Code(), got.Message())
	}
	if detail, ok := rpcv1.FromError(got); !ok || !proto.Equal(detail, want) {
		t.Errorf("ErrorDetail() = %v, %v; want %v, true", detail, ok, want)
	}
}

func TestOuterCodedErrorDoesNotInheritCauseFields(t *testing.T) {
	inner := connect.NewError(connect.CodeNotFound, helpfulFixtureError{
		codedFixtureError: codedFixtureError{
			message: "inner message", code: "inner_code", kind: "inner_type", namespace: "inner",
			instance: "inner_instance", expose: true, meta: map[string]any{"private": true},
		},
		help: "inner help",
	})
	outer := wrappingCodedFixtureError{message: "outer message", code: "outer_code", cause: inner}
	got := callErrorFixture(t, outer)
	want := &rpcv1.Error{Code: "outer_code", Type: "outer_code"}
	if got.Code() != connect.CodeNotFound || got.Message() != "outer message" {
		t.Errorf("Connect error = %s %q, want not_found outer message", got.Code(), got.Message())
	}
	if detail, ok := rpcv1.FromError(got); !ok || !proto.Equal(detail, want) {
		t.Errorf("ErrorDetail() = %v, %v; want outer-only %v, true", detail, ok, want)
	}
}

func TestWireErrorDetailDoesNotLeakDownstreamHeaders(t *testing.T) {
	downstream := connect.NewWireError(connect.CodeNotFound, minimalCodedFixtureError{})
	downstream.Meta().Set("X-Downstream-Secret", "must-not-leak")
	got := callErrorFixture(t, downstream)
	if got.Meta().Get("X-Downstream-Secret") != "" {
		t.Errorf("Connect response leaked downstream headers: %v", got.Meta())
	}
	if !connect.IsWireError(got) || !connect.IsWireError(downstream) {
		t.Error("Connect errors lost their wire-error identity")
	}
	if len(downstream.Details()) != 0 || downstream.Meta().Get("X-Downstream-Secret") != "must-not-leak" {
		t.Errorf("original wire error mutated: details %v, headers %v", downstream.Details(), downstream.Meta())
	}
	want := &rpcv1.Error{Code: "minimal_failure", Type: "minimal_failure"}
	if detail, ok := rpcv1.FromError(got); !ok || !proto.Equal(detail, want) {
		t.Errorf("ErrorDetail() = %v, %v; want %v, true", detail, ok, want)
	}
}

func TestGenericWrapperPreservesConnectMessage(t *testing.T) {
	inner := connect.NewError(connect.CodeNotFound, codedFixtureError{
		message: "original message", code: "original_code", kind: "original_type",
	})
	got := callErrorFixture(t, fmt.Errorf("outer context: %w", inner))
	if got.Code() != connect.CodeNotFound || got.Message() != "original message" {
		t.Errorf("Connect error = %s %q, want not_found original message", got.Code(), got.Message())
	}
	want := &rpcv1.Error{Code: "original_code", Type: "original_type"}
	if detail, ok := rpcv1.FromError(got); !ok || !proto.Equal(detail, want) {
		t.Errorf("ErrorDetail() = %v, %v; want %v, true", detail, ok, want)
	}
}

func TestLegacyClientErrorRetainsStandardIdentity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"upstream failure","instance":"upstream_instance","expose":true,"code":"upstream_code","namespace":"upstream_namespace","type":"upstream_type"}`))
	}))
	defer upstream.Close()
	legacy := v1.NewClient(upstream.URL)
	failure := legacy.DoRequest(context.Background(), "lookup", struct{}{}, nil)
	if failure == nil {
		t.Fatal("legacy DoRequest() succeeded, want upstream error")
	}
	got := callErrorFixture(t, failure)
	want := &rpcv1.Error{
		Code: "upstream_code", Namespace: "upstream_namespace", Type: "upstream_type",
		Instance: "upstream_instance", Expose: true,
	}
	if got.Code() != connect.CodeUnknown || got.Message() != "upstream failure" {
		t.Errorf("Connect error = %s %q, want unknown upstream failure", got.Code(), got.Message())
	}
	if detail, ok := rpcv1.FromError(got); !ok || !proto.Equal(detail, want) {
		t.Errorf("ErrorDetail() = %v, %v; want %v, true", detail, ok, want)
	}
}

func TestStandardDetailNormalizesTypedMetadataAndDropsReservedFields(t *testing.T) {
	typed := struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}{Name: "nested", Count: 2}
	failure := codedFixtureError{
		message: "original message", code: "original_code", namespace: "original_namespace",
		kind: "original_type", instance: "original_instance",
		meta: map[string]any{
			"labels": []string{"one", "two"}, "nested": typed,
			"message": "wrong message", "instance": "wrong instance", "expose": true,
			"code": "wrong code", "namespace": "wrong namespace", "type": "wrong type", "help": "wrong help",
		},
	}
	got := callErrorFixture(t, failure)
	want := &rpcv1.Error{
		Code: "original_code", Namespace: "original_namespace", Type: "original_type", Instance: "original_instance",
		Meta: fixtureMeta(t, map[string]any{
			"labels": []any{"one", "two"}, "nested": map[string]any{"name": "nested", "count": float64(2)},
		}),
	}
	if detail, ok := rpcv1.FromError(got); !ok || !proto.Equal(detail, want) {
		t.Errorf("ErrorDetail() = %v, %v; want normalized %v, true", detail, ok, want)
	}
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{echo: func(context.Context, *connect.Request[testpb.Scalars]) (*connect.Response[testpb.Scalars], error) {
		return nil, failure
	}}))
	response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", "{}")
	wantBody := `{"message":"original message","instance":"original_instance","code":"original_code","namespace":"original_namespace","type":"original_type","labels":["one","two"],"nested":{"count":2,"name":"nested"}}`
	if response.Code != http.StatusBadRequest || response.Body.String() != wantBody {
		t.Errorf("bridge error = %d %q, want 400 %q", response.Code, response.Body.String(), wantBody)
	}
}
