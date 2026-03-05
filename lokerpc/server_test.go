package lokerpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/log"
	jtd "github.com/jsontypedef/json-typedef-go"
)

type sendRequest struct {
	Value string `json:"value"`
}

type sendResponse struct {
	OK bool `json:"ok"`
}

func TestMountHandlers_UsesRequestTypeDefOverrideInMetadata(t *testing.T) {
	schema := &jtd.Schema{
		Discriminator: "template",
		Mapping: map[string]jtd.Schema{
			"loke_manager_magic_link": {
				Properties: map[string]jtd.Schema{
					"template": {Enum: []string{"loke_manager_magic_link"}},
					"to":       {Type: jtd.TypeString},
				},
			},
			"loke_manager_otp": {
				Properties: map[string]jtd.Schema{
					"template": {Enum: []string{"loke_manager_otp"}},
					"to":       {Type: jtd.TypeString},
				},
			},
		},
	}

	ecm := EndpointCodecMap{
		"send": MakeStandardEndpointCodec(
			func(ctx context.Context, req sendRequest) (sendResponse, error) {
				return sendResponse{OK: true}, nil
			},
			"send email",
			WithRequestTypeDef(schema),
		),
	}

	svc := NewService("email", "email service", ecm)
	mux := http.NewServeMux()
	MountHandlers(log.NewNopLogger(), mux, svc)

	req := httptest.NewRequest(http.MethodGet, "/rpc/email", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d, want %d", rec.Code, http.StatusOK)
	}

	var meta Meta
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}

	if len(meta.Interfaces) != 1 {
		t.Fatalf("unexpected number of interfaces: got %d, want 1", len(meta.Interfaces))
	}

	got := meta.Interfaces[0].RequestTypeDef
	if got == nil {
		t.Fatal("requestTypeDef missing")
	}
	if got.Form() != jtd.FormDiscriminator {
		t.Fatalf("unexpected requestTypeDef form: got %v, want %v", got.Form(), jtd.FormDiscriminator)
	}
	if got.Discriminator != schema.Discriminator {
		t.Fatalf("unexpected discriminator: got %q, want %q", got.Discriminator, schema.Discriminator)
	}
	if len(got.Mapping) != len(schema.Mapping) {
		t.Fatalf("unexpected mapping size: got %d, want %d", len(got.Mapping), len(schema.Mapping))
	}
	if _, ok := got.Mapping["loke_manager_magic_link"]; !ok {
		t.Fatal("missing mapping for loke_manager_magic_link")
	}
	if _, ok := got.Mapping["loke_manager_otp"]; !ok {
		t.Fatal("missing mapping for loke_manager_otp")
	}
}
