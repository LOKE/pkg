package lokerpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/log"
)

type voidTestReq struct {
	Name string `json:"name"`
}

type voidTestRes struct {
	ID string `json:"id"`
}

func voidTestMethod(_ context.Context, _ voidTestReq) (voidTestRes, error) {
	return voidTestRes{ID: "x"}, nil
}

func TestMountHandlersVoidResponse(t *testing.T) {
	svc := NewService("svc", "help", EndpointCodecMap{
		"m": MakeStandardEndpointCodec(voidTestMethod, "method", VoidResponse()),
	})

	mux := http.NewServeMux()
	MountHandlers(log.NewNopLogger(), mux, svc)

	req := httptest.NewRequest(http.MethodGet, "/rpc/svc", nil)
	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", rw.Code, http.StatusOK)
	}

	var meta Meta
	if err := json.Unmarshal(rw.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decode meta: %v", err)
	}

	if len(meta.Interfaces) != 1 {
		t.Fatalf("unexpected interface count: got %d want 1", len(meta.Interfaces))
	}

	got := meta.Interfaces[0].ResponseTypeDef
	if got == nil {
		t.Fatal("responseTypeDef is nil")
	}
	if got.Metadata["void"] != true {
		t.Fatalf("responseTypeDef.metadata.void: got %#v want true", got.Metadata["void"])
	}
}
