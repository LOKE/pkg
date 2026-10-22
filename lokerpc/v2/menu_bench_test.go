package lokerpc_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/LOKE/pkg/lokerpc"
	v2 "github.com/LOKE/pkg/lokerpc/v2"
	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb"
	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb/testpbconnect"
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

type benchmarkMenuService struct {
	menu *testpb.BenchmarkMenu
}

func (s benchmarkMenuService) Encode(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[testpb.BenchmarkMenu], error) {
	return connect.NewResponse(s.menu), nil
}

func (benchmarkMenuService) Decode(context.Context, *connect.Request[testpb.BenchmarkMenu]) (*connect.Response[emptypb.Empty], error) {
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (benchmarkMenuService) Small(_ context.Context, request *connect.Request[testpb.BenchmarkOption]) (*connect.Response[testpb.BenchmarkOption], error) {
	return connect.NewResponse(request.Msg), nil
}

type benchmarkGzipResponse struct {
	http.ResponseWriter
	writer *gzip.Writer
}

func (w benchmarkGzipResponse) WriteHeader(status int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
}

func (w benchmarkGzipResponse) Write(data []byte) (int, error) {
	w.Header().Del("Content-Length")
	return w.writer.Write(data)
}

// Legacy routes rely on outer HTTP compression; native Connect negotiates its
// own gzip. Both paths use pooled standard-library default-level compressors.
func benchmarkLegacyGzip(handler http.Handler) http.Handler {
	writers := sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rpc/") || !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			handler.ServeHTTP(w, r)
			return
		}
		writer := writers.Get().(*gzip.Writer)
		writer.Reset(w)
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		handler.ServeHTTP(benchmarkGzipResponse{ResponseWriter: w, writer: writer}, r)
		_ = writer.Close()
		writers.Put(writer)
	})
}

// Each round times four adjacent per-path blocks, reversing their order between
// -count rounds. All cases use loopback HTTP. Server-only calls drain responses
// without parsing; round trips use the real legacy and generated Connect clients.
func BenchmarkMenuBridgeHTTP(b *testing.B) {
	menu := largeBenchmarkMenu()
	body, err := (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(menu)
	if err != nil {
		b.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, body); err != nil {
		b.Fatal(err)
	}
	body = compact.Bytes()
	protoBody, err := proto.Marshal(menu)
	if err != nil {
		b.Fatal(err)
	}
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
	small := &testpb.BenchmarkOption{Id: "small", Name: "tiny", Price: 42}
	smallJSON, err := protojson.Marshal(small)
	if err != nil {
		b.Fatal(err)
	}
	var compactSmall bytes.Buffer
	if err := json.Compact(&compactSmall, smallJSON); err != nil {
		b.Fatal(err)
	}
	smallJSON = compactSmall.Bytes()
	smallProto, err := proto.Marshal(small)
	if err != nil {
		b.Fatal(err)
	}
	smallTyped := benchmarkOption{ID: small.Id, Name: small.Name, Price: small.Price}
	legacy := v1.NewService("menu", "benchmark", v1.EndpointCodecMap{
		"encode": v1.MakeStandardEndpointCodec(func(context.Context, struct{}) (benchmarkMenu, error) { return typed, nil }, "encode"),
		"decode": v1.MakeStandardEndpointCodec(func(context.Context, benchmarkMenu) (struct{}, error) { return struct{}{}, nil }, "decode"),
		"small":  v1.MakeStandardEndpointCodec(func(_ context.Context, request benchmarkOption) (benchmarkOption, error) { return request, nil }, "small"),
	})
	v1mux := http.NewServeMux()
	legacy.MountHandlers(log.NewNopLogger(), v1mux)
	v2mux := http.NewServeMux()
	service := v2.Service{Name: "menu", Handler: func(opts ...connect.HandlerOption) (string, http.Handler) {
		return testpbconnect.NewBenchmarkMenuServiceHandler(benchmarkMenuService{menu: menu}, opts...)
	}}
	if err := v2.MountHandlers(log.NewNopLogger(), v2mux, []v2.Service{service}); err != nil {
		b.Fatal(err)
	}
	legacyServer := httptest.NewServer(benchmarkLegacyGzip(v1mux))
	bridgeServer := httptest.NewServer(benchmarkLegacyGzip(v2mux))
	b.Cleanup(legacyServer.Close)
	b.Cleanup(bridgeServer.Close)
	paths := []struct {
		name        string
		baseURL     string
		contentType string
		native      bool
	}{
		{"legacy", legacyServer.URL, "application/json", false},
		{"bridge-json", bridgeServer.URL, "application/json", false},
		{"connect-proto", bridgeServer.URL, "application/proto", true},
		{"connect-json", bridgeServer.URL, "application/json", true},
	}
	var legacyClients [2][2]v1.Client
	var connectClients [2][2]testpbconnect.BenchmarkMenuServiceClient
	var httpClients [2]*http.Client
	for compressed := range 2 {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DisableCompression = compressed == 0
		b.Cleanup(transport.CloseIdleConnections)
		httpClients[compressed] = &http.Client{Transport: transport}
		// v1 captures the default client and has no exported transport injection.
		// Restore the global immediately, before making any requests.
		previous := http.DefaultClient
		http.DefaultClient = httpClients[compressed]
		legacyClients[compressed][0] = v1.NewClient(legacyServer.URL + "/rpc/menu")
		legacyClients[compressed][1] = v1.NewClient(bridgeServer.URL + "/rpc/menu")
		http.DefaultClient = previous
		for format := range 2 {
			var options []connect.ClientOption
			if compressed == 0 {
				options = append(options, connect.WithAcceptCompression("gzip", nil, nil))
			}
			if format == 1 {
				options = append(options, connect.WithProtoJSON())
			}
			connectClients[compressed][format] = testpbconnect.NewBenchmarkMenuServiceClient(httpClients[compressed], bridgeServer.URL, options...)
		}
	}
	// Count response payload bytes before HTTP's automatic decompression,
	// excluding HTTP headers and transport framing.
	for compressed := range 2 {
		var sizes [4]int
		for index, path := range paths {
			target := "/rpc/menu/encode"
			requestBody := []byte("{}")
			if path.native {
				target = testpbconnect.BenchmarkMenuServiceEncodeProcedure
			}
			if path.contentType == "application/proto" {
				requestBody = nil
			}
			request, err := http.NewRequest(http.MethodPost, path.baseURL+target, bytes.NewReader(requestBody))
			if err != nil {
				b.Fatal(err)
			}
			request.Header.Set("Content-Type", path.contentType)
			request.Header.Set("Accept-Encoding", "identity")
			if compressed == 1 {
				request.Header.Set("Accept-Encoding", "gzip")
			}
			if path.native {
				request.Header.Set("Connect-Protocol-Version", "1")
			}
			response, err := httpClients[0].Do(request)
			if err != nil {
				b.Fatal(err)
			}
			data, readErr := io.ReadAll(response.Body)
			closeErr := response.Body.Close()
			if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK {
				b.Fatalf("wire %s: status %d, read %v, close %v", path.name, response.StatusCode, readErr, closeErr)
			}
			if compressed == 1 && response.Header.Get("Content-Encoding") != "gzip" {
				b.Fatalf("wire %s did not negotiate gzip", path.name)
			}
			sizes[index] = len(data)
			if compressed == 1 {
				reader, err := gzip.NewReader(bytes.NewReader(data))
				if err != nil {
					b.Fatal(err)
				}
				data, err = io.ReadAll(reader)
				closeErr := reader.Close()
				if err != nil || closeErr != nil {
					b.Fatalf("gzip %s: read %v, close %v", path.name, err, closeErr)
				}
			}
			decoded := &testpb.BenchmarkMenu{}
			if path.contentType == "application/proto" {
				err = proto.Unmarshal(data, decoded)
			} else {
				err = protojson.Unmarshal(data, decoded)
			}
			if err != nil || !proto.Equal(menu, decoded) {
				b.Fatalf("wire %s changed menu: %v", path.name, err)
			}
		}
		b.Logf("wire gzip=%t legacy=%d bridge-json=%d connect-proto=%d connect-json=%d", compressed == 1, sizes[0], sizes[1], sizes[2], sizes[3])
	}
	// Validate each real client outside the measured blocks.
	for compressed := range 2 {
		for index := range 2 {
			client := legacyClients[compressed][index]
			var decoded benchmarkMenu
			if err := client.DoRequest(context.Background(), "encode", struct{}{}, &decoded); err != nil || !reflect.DeepEqual(typed, decoded) {
				b.Fatalf("legacy client %d gzip=%d changed menu: %v", index, compressed, err)
			}
			native := connectClients[compressed][index]
			response, err := native.Encode(context.Background(), connect.NewRequest(&emptypb.Empty{}))
			if err != nil || !proto.Equal(menu, response.Msg) {
				b.Fatalf("Connect client %d gzip=%d changed menu: %v", index, compressed, err)
			}
			if compressed == 0 {
				var decodedSmall benchmarkOption
				if err := client.DoRequest(context.Background(), "small", smallTyped, &decodedSmall); err != nil || decodedSmall != smallTyped {
					b.Fatalf("legacy client %d changed small response: %v", index, err)
				}
				response, err := native.Small(context.Background(), connect.NewRequest(small))
				if err != nil || !proto.Equal(small, response.Msg) {
					b.Fatalf("Connect client %d changed small response: %v", index, err)
				}
			}
		}
	}
	b.Logf("fixture: products=%d options/product=%d compact-JSON=%d protobuf=%d small-JSON=%d small-protobuf=%d",
		len(menu.Products), len(menu.Products[0].Options), len(body), len(protoBody), len(smallJSON), len(smallProto))
	cases := []struct {
		name  string
		calls [4]func() error
	}{
		{name: "server-large-response"},
		{name: "server-large-request"},
		{name: "server-small"},
		{name: "roundtrip-large-gzip-off"},
		{name: "roundtrip-large-gzip-on"},
		{name: "roundtrip-small"},
	}
	for index, path := range paths {
		for operation, method := range []string{"Encode", "Decode", "Small"} {
			target := "/rpc/menu/" + strings.ToLower(method)
			requestBody := []byte("{}")
			if operation == 1 {
				requestBody = body
			} else if operation == 2 {
				requestBody = smallJSON
			}
			if path.native {
				target = "/loke.rpc.test.v2.BenchmarkMenuService/" + method
			}
			if path.contentType == "application/proto" {
				requestBody = nil
				if operation == 1 {
					requestBody = protoBody
				} else if operation == 2 {
					requestBody = smallProto
				}
			}
			cases[operation].calls[index] = func() error {
				request, err := http.NewRequest(http.MethodPost, path.baseURL+target, bytes.NewReader(requestBody))
				if err != nil {
					return err
				}
				request.Header.Set("Content-Type", path.contentType)
				if path.native {
					request.Header.Set("Connect-Protocol-Version", "1")
				}
				response, err := httpClients[0].Do(request)
				if err != nil {
					return err
				}
				_, readErr := io.Copy(io.Discard, response.Body)
				closeErr := response.Body.Close()
				if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK {
					return fmt.Errorf("HTTP %d: read %v, close %v", response.StatusCode, readErr, closeErr)
				}
				return nil
			}
		}
		for compressed := range 2 {
			if index < 2 {
				client := legacyClients[compressed][index]
				cases[3+compressed].calls[index] = func() error {
					var response benchmarkMenu
					return client.DoRequest(context.Background(), "encode", struct{}{}, &response)
				}
			} else {
				client := connectClients[compressed][index-2]
				cases[3+compressed].calls[index] = func() error {
					_, err := client.Encode(context.Background(), connect.NewRequest(&emptypb.Empty{}))
					return err
				}
			}
		}
		if index < 2 {
			client := legacyClients[0][index]
			cases[5].calls[index] = func() error {
				var response benchmarkOption
				return client.DoRequest(context.Background(), "small", smallTyped, &response)
			}
		} else {
			client := connectClients[0][index-2]
			cases[5].calls[index] = func() error {
				_, err := client.Small(context.Background(), connect.NewRequest(small))
				return err
			}
		}
	}
	for _, test := range cases {
		round := -1
		b.Run(test.name, func(b *testing.B) {
			// Classic b.N calibration re-enters this callback. Every -count
			// round starts at N=1; calibration retains that round's order.
			if b.N == 1 {
				round++
			}
			for offset := range 4 {
				index := offset
				if round%2 != 0 {
					index = 3 - offset
				}
				start := time.Now()
				for iteration := 0; iteration < b.N; iteration++ {
					if err := test.calls[index](); err != nil {
						b.Fatalf("%s: %v", paths[index].name, err)
					}
				}
				elapsed := time.Since(start)
				b.ReportMetric(float64(elapsed)/float64(b.N)/1e6, paths[index].name+"-ms/op")
			}
		})
	}
}
