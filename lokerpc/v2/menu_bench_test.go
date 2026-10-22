package lokerpc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	v1 "github.com/LOKE/pkg/lokerpc"
	v2 "github.com/LOKE/pkg/lokerpc/v2"
	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb"
	"github.com/go-kit/log"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type benchmarkMenu struct {
	Products []benchmarkProduct `json:"products"`
}
type benchmarkProduct struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Price       int32             `json:"price"`
	Available   bool              `json:"available"`
	Tags        []string          `json:"tags"`
	Options     []benchmarkOption `json:"options"`
}
type benchmarkOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int32  `json:"price"`
}

func largeBenchmarkMenu() *testpb.BenchmarkMenu {
	menu := &testpb.BenchmarkMenu{}
	for i := range 200 {
		product := &testpb.BenchmarkProduct{
			Id: fmt.Sprintf("product-%03d", i), Name: fmt.Sprintf("Roasted seasonal bowl %d", i),
			Description: strings.Repeat("Freshly prepared seasonal vegetables, grains and house dressing. ", 23),
			Price:       int32(1200 + i), Available: true, Tags: []string{"seasonal", "vegetarian", "popular"},
		}
		for j := range 8 {
			product.Options = append(product.Options, &testpb.BenchmarkOption{Id: fmt.Sprintf("option-%d", j), Name: fmt.Sprintf("Extra topping %d", j), Price: int32(j * 75)})
		}
		menu.Products = append(menu.Products, product)
	}
	return menu
}

// Both cases exercise the real server routes, codecs, request-context setup and
// metrics over httptest. Encode sends an empty request and returns the entire
// menu; Decode sends the menu and returns an empty object. Client JSON parsing
// is intentionally excluded to isolate server-side bridge overhead.
func BenchmarkMenuBridgeHTTP(b *testing.B) {
	menu := largeBenchmarkMenu()
	body, err := (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(menu)
	if err != nil {
		b.Fatal(err)
	}
	// Protojson varies comma whitespace per binary. Keep the request fixture
	// deterministic so before/after decoding benchmarks process the same bytes.
	var compact bytes.Buffer
	if err := json.Compact(&compact, body); err != nil {
		b.Fatal(err)
	}
	body = compact.Bytes()
	var typed benchmarkMenu
	if err := json.Unmarshal(body, &typed); err != nil {
		b.Fatal(err)
	}
	encoded, err := json.Marshal(typed)
	if err != nil {
		b.Fatal(err)
	}
	roundtrip := &testpb.BenchmarkMenu{}
	if err := protojson.Unmarshal(encoded, roundtrip); err != nil || !proto.Equal(menu, roundtrip) {
		b.Fatalf("typed fixture differs: %v", err)
	}
	b.Logf("menu JSON bytes: %d", len(body))
	legacy := v1.NewService("menu", "benchmark", v1.EndpointCodecMap{
		"encode": v1.MakeStandardEndpointCodec(func(context.Context, struct{}) (benchmarkMenu, error) { return typed, nil }, "encode"),
		"decode": v1.MakeStandardEndpointCodec(func(context.Context, benchmarkMenu) (struct{}, error) { return struct{}{}, nil }, "decode"),
	})
	v1mux := http.NewServeMux()
	legacy.MountHandlers(log.NewNopLogger(), v1mux)
	v2mux := http.NewServeMux()
	service := v2.Service{Name: "menu", Handler: func(opts ...connect.HandlerOption) (string, http.Handler) {
		mux := http.NewServeMux()
		mux.Handle("/loke.rpc.test.v2.BenchmarkMenuService/Encode", connect.NewUnaryHandler("/loke.rpc.test.v2.BenchmarkMenuService/Encode", func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[testpb.BenchmarkMenu], error) {
			return connect.NewResponse(menu), nil
		}, opts...))
		mux.Handle("/loke.rpc.test.v2.BenchmarkMenuService/Decode", connect.NewUnaryHandler("/loke.rpc.test.v2.BenchmarkMenuService/Decode", func(context.Context, *connect.Request[testpb.BenchmarkMenu]) (*connect.Response[emptypb.Empty], error) {
			return connect.NewResponse(&emptypb.Empty{}), nil
		}, opts...))
		return "/loke.rpc.test.v2.BenchmarkMenuService/", mux
	}}
	if err := v2.MountHandlers(log.NewNopLogger(), v2mux, []v2.Service{service}); err != nil {
		b.Fatal(err)
	}
	for _, server := range []struct {
		name    string
		handler http.Handler
	}{{"v1", v1mux}, {"bridge", v2mux}} {
		for _, operation := range []string{"encode", "decode"} {
			requestBody := []byte("{}")
			if operation == "decode" {
				requestBody = body
			}
			b.Run(server.name+"/"+operation, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				for b.Loop() {
					req := httptest.NewRequest(http.MethodPost, "/rpc/menu/"+operation, bytes.NewReader(requestBody))
					req.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					server.handler.ServeHTTP(response, req)
					if response.Code != http.StatusOK {
						b.Fatalf("HTTP %d: %s", response.Code, response.Body.String())
					}
				}
			})
		}
	}
}
