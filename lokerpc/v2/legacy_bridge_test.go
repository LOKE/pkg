package lokerpc_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	v2 "github.com/LOKE/pkg/lokerpc/v2"
	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb/testpbconnect"
)

func TestLegacyBridgeCodecIsNotExposedOnNativeRoutes(t *testing.T) {
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{}))
	request := httptest.NewRequest(http.MethodPost, testpbconnect.FixtureServiceEchoProcedure, strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/loke-legacy-json")
	request.Header.Set("Connect-Protocol-Version", "1")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("native custom codec status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestLegacyBridgeSendLimitAppliesToDefaultFilledJSON(t *testing.T) {
	// Native JSON is {}, but the bridge emits the scalar/list/map defaults.
	mux := mountFixture(t, fixtureService("fixture", fixtureHandler{}, connect.WithSendMaxBytes(64)))
	request := httptest.NewRequest(http.MethodPost, testpbconnect.FixtureServiceEchoProcedure, strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	native := httptest.NewRecorder()
	mux.ServeHTTP(native, request)
	if native.Code != http.StatusOK || strings.TrimSpace(native.Body.String()) != "{}" {
		t.Fatalf("native compact JSON status = %d, body = %s", native.Code, native.Body.String())
	}
	legacy := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", "{}")
	if legacy.Code != http.StatusBadRequest {
		t.Fatalf("bridge default-filled JSON status = %d, body = %s", legacy.Code, legacy.Body.String())
	}
	if code := decodeObject(t, legacy)["code"]; code != connect.CodeResourceExhausted.String() {
		t.Fatalf("bridge error code = %v, want resource_exhausted", code)
	}
}

func TestLegacyBridgeEmptyHandlerResponseIsInvalidJSON(t *testing.T) {
	service := v2.Service{
		Name: "fixture",
		Handler: func(...connect.HandlerOption) (string, http.Handler) {
			return "/loke.rpc.test.v2.FixtureService/", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
		},
	}
	mux := mountFixture(t, service)
	response := serve(t, mux, http.MethodPost, "/rpc/fixture/echo", "{}")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("empty response status = %d, body = %s", response.Code, response.Body.String())
	}
	if message, ok := decodeObject(t, response)["message"].(string); !ok || message == "" {
		t.Fatalf("empty response error = %s", response.Body.String())
	}
}
