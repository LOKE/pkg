package main

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

	"github.com/LOKE/pkg/examples/negotiate/fixtures"
	"github.com/LOKE/pkg/examples/negotiate/fixtures/menuclient"
	"github.com/LOKE/pkg/examples/negotiate/jsonengines"
	"github.com/LOKE/pkg/examples/negotiate/msgpackcodec"
	"github.com/LOKE/pkg/examples/negotiate/protocodec"
	"github.com/LOKE/pkg/lokerpc"
	kitlog "github.com/go-kit/log"
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

func largeBenchmarkMenu() benchmarkMenu {
	menu := benchmarkMenu{}
	for i := range 200 {
		product := benchmarkProduct{
			ID: fmt.Sprintf("product-%03d", i), Name: fmt.Sprintf("Roasted seasonal bowl %d", i),
			Description: strings.Repeat("Freshly prepared seasonal vegetables, grains and house dressing. ", 23),
			Price:       int32(1200 + i), Available: true, Tags: []string{"seasonal", "vegetarian", "popular"},
		}
		for j := range 8 {
			product.Options = append(product.Options, benchmarkOption{ID: fmt.Sprintf("option-%d", j), Name: fmt.Sprintf("Extra topping %d", j), Price: int32(j * 75)})
		}
		menu.Products = append(menu.Products, product)
	}
	return menu
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

func newPointer[T any]() any {
	return new(T)
}

func newBenchmarkMenuService[Menu, Option any](menu Menu, opts ...lokerpc.ServiceOption) *lokerpc.Service {
	return lokerpc.NewService("menu", "benchmark", lokerpc.EndpointCodecMap{
		"encode": lokerpc.MakeStandardEndpointCodec(func(context.Context, struct{}) (Menu, error) { return menu, nil }, "encode"),
		"decode": lokerpc.MakeStandardEndpointCodec(func(context.Context, Menu) (struct{}, error) { return struct{}{}, nil }, "decode"),
		"small":  lokerpc.MakeStandardEndpointCodec(func(_ context.Context, request Option) (Option, error) { return request, nil }, "small"),
	}, opts...)
}

// Each round times four adjacent per-path blocks, reversing their order between
// -count rounds. All cases use loopback HTTP. Server-only calls drain responses
// without parsing; round trips use the real clients.
func BenchmarkMenuNegotiationHTTP(b *testing.B) {
	proto := protocodec.New()
	msgpackCodec := msgpackcodec.New()
	typed := largeBenchmarkMenu()
	body, err := json.Marshal(typed)
	if err != nil {
		b.Fatal(err)
	}
	small := benchmarkOption{ID: "small", Name: "tiny", Price: 42}
	smallJSON, err := json.Marshal(small)
	if err != nil {
		b.Fatal(err)
	}

	legacyMux := http.NewServeMux()
	lokerpc.MountHandlers(kitlog.NewNopLogger(), legacyMux, newBenchmarkMenuService[benchmarkMenu, benchmarkOption](typed))
	negotiatedMux := http.NewServeMux()
	lokerpc.MountHandlers(kitlog.NewNopLogger(), negotiatedMux, newBenchmarkMenuService[benchmarkMenu, benchmarkOption](typed, lokerpc.ServeCodecs(proto)))
	// The fixtures types carry generated protobuf code, so this server and its
	// client skip reflection when the negotiated schema matches.
	generatedMenu := fixtures.LargeMenu()
	generatedMux := http.NewServeMux()
	lokerpc.MountHandlers(kitlog.NewNopLogger(), generatedMux, newBenchmarkMenuService[fixtures.BenchMenu, fixtures.BenchOption](generatedMenu, lokerpc.ServeCodecs(proto)))
	// The JSON engines keep the wire format; only the library behind it changes.
	jsonV2Mux := http.NewServeMux()
	lokerpc.MountHandlers(kitlog.NewNopLogger(), jsonV2Mux, newBenchmarkMenuService[benchmarkMenu, benchmarkOption](typed, lokerpc.ServeJSON(jsonengines.V2{})))
	goJSONMux := http.NewServeMux()
	lokerpc.MountHandlers(kitlog.NewNopLogger(), goJSONMux, newBenchmarkMenuService[benchmarkMenu, benchmarkOption](typed, lokerpc.ServeJSON(jsonengines.GoJSON{})))
	goJSONOrderedMux := http.NewServeMux()
	lokerpc.MountHandlers(kitlog.NewNopLogger(), goJSONOrderedMux, newBenchmarkMenuService[benchmarkMenu, benchmarkOption](typed, lokerpc.ServeJSON(jsonengines.GoJSON{OptimizeFieldOrder: true})))
	// The fixtures types also carry tinylib/msgp generated methods.
	msgpackMux := http.NewServeMux()
	lokerpc.MountHandlers(kitlog.NewNopLogger(), msgpackMux, newBenchmarkMenuService[fixtures.BenchMenu, fixtures.BenchOption](generatedMenu, lokerpc.ServeCodecs(msgpackCodec)))

	newServer := func(mux http.Handler) *httptest.Server {
		server := httptest.NewServer(benchmarkLegacyGzip(mux))
		b.Cleanup(server.Close)
		return server
	}
	legacyServer, negotiatedServer, generatedServer := newServer(legacyMux), newServer(negotiatedMux), newServer(generatedMux)
	jsonV2Server, goJSONServer, msgpackServer := newServer(jsonV2Mux), newServer(goJSONMux), newServer(msgpackMux)
	goJSONOrderedServer := newServer(goJSONOrderedMux)

	var httpClients [2]*http.Client
	for compressed := range 2 {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DisableCompression = compressed == 0
		b.Cleanup(transport.CloseIdleConnections)
		httpClients[compressed] = &http.Client{Transport: transport}
	}

	// The service schemas come from the same OPTIONS probe a client sends.
	probe := func(url string) (string, lokerpc.Meta) {
		request, err := http.NewRequest(http.MethodOptions, url+"/rpc/menu", nil)
		if err != nil {
			b.Fatal(err)
		}
		response, err := httpClients[0].Do(request)
		if err != nil {
			b.Fatal(err)
		}
		var meta lokerpc.Meta
		err = json.NewDecoder(response.Body).Decode(&meta)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK {
			b.Fatalf("probe: status %d: %v", response.StatusCode, err)
		}
		return response.Header.Get("ETag"), meta
	}
	etag, meta := probe(negotiatedServer.URL)
	generatedETag, _ := probe(generatedServer.URL)
	endpoints := map[string]lokerpc.EndpointMeta{}
	for _, endpoint := range meta.Interfaces {
		endpoints[endpoint.MethodName] = endpoint
	}
	encodeProto := func(method string, value any) []byte {
		data, err := proto.Marshal(value, endpoints[method].RequestTypeDef, meta.Definitions)
		if err != nil {
			b.Fatal(err)
		}
		return data
	}
	emptyProto, bodyProto, smallProto := encodeProto("encode", struct{}{}), encodeProto("decode", typed), encodeProto("small", small)

	msgpackBody := func(value any) []byte {
		data, err := msgpackCodec.Marshal(value, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		return data
	}
	emptyMsgpack, bodyMsgpack, smallMsgpack := msgpackBody(struct{}{}), msgpackBody(generatedMenu), msgpackBody(fixtures.BenchOption{ID: small.ID, Name: small.Name, Price: small.Price})

	var generatedClientMenu menuclient.Menu
	err = json.Unmarshal(body, &generatedClientMenu)
	if err != nil {
		b.Fatal(err)
	}
	generatedClientSmall := menuclient.Option{ID: small.ID, Name: small.Name, Price: small.Price}

	jsonBodies := [3][]byte{[]byte("{}"), body, smallJSON}
	protoBodies := [3][]byte{emptyProto, bodyProto, smallProto}
	paths := []struct {
		name        string
		baseURL     string
		contentType string
		accept      string
		ifMatch     string
		// codec is nil for JSON paths.
		codec         lokerpc.Codec
		requestBodies [3][]byte
		// The Go types the path's client decodes into, and what they must hold.
		newMenu    func() any
		newOption  func() any
		wantMenu   any
		wantOption any
		optionArg  any
	}{
		{"legacy", legacyServer.URL, "application/json", "", "", nil, jsonBodies, newPointer[benchmarkMenu], newPointer[benchmarkOption], typed, small, small},
		{"negotiated-json", negotiatedServer.URL, "application/json", "", "", nil, jsonBodies, newPointer[benchmarkMenu], newPointer[benchmarkOption], typed, small, small},
		{"negotiated-proto", negotiatedServer.URL, proto.ContentType(), proto.ContentType() + ", application/json;q=0.5", etag, proto, protoBodies, newPointer[benchmarkMenu], newPointer[benchmarkOption], typed, small, small},
		{"generated-proto", generatedServer.URL, proto.ContentType(), proto.ContentType() + ", application/json;q=0.5", generatedETag, proto, protoBodies, newPointer[menuclient.Menu], newPointer[menuclient.Option], generatedClientMenu, generatedClientSmall, generatedClientSmall},
		{"jsonv2", jsonV2Server.URL, "application/json", "", "", nil, jsonBodies, newPointer[benchmarkMenu], newPointer[benchmarkOption], typed, small, small},
		{"gojson", goJSONServer.URL, "application/json", "", "", nil, jsonBodies, newPointer[benchmarkMenu], newPointer[benchmarkOption], typed, small, small},
		{"msgpack", msgpackServer.URL, msgpackCodec.ContentType(), msgpackCodec.ContentType() + ", application/json;q=0.5", "", msgpackCodec, [3][]byte{emptyMsgpack, bodyMsgpack, smallMsgpack}, newPointer[menuclient.Menu], newPointer[menuclient.Option], generatedClientMenu, generatedClientSmall, generatedClientSmall},
		{"gojson-ordered", goJSONOrderedServer.URL, "application/json", "", "", nil, jsonBodies, newPointer[benchmarkMenu], newPointer[benchmarkOption], typed, small, small},
	}

	const pathCount = 8
	var clients [2][pathCount]lokerpc.Client
	for compressed := range 2 {
		withHTTP := lokerpc.WithHTTPClient(httpClients[compressed])
		clients[compressed][0] = lokerpc.NewClient(legacyServer.URL+"/rpc/menu", withHTTP)
		clients[compressed][1] = lokerpc.NewClient(negotiatedServer.URL+"/rpc/menu", withHTTP)
		clients[compressed][2] = lokerpc.NewClient(negotiatedServer.URL+"/rpc/menu", withHTTP, lokerpc.ClientCodecs(proto))
		clients[compressed][3] = lokerpc.NewClient(generatedServer.URL+"/rpc/menu", withHTTP, lokerpc.ClientCodecs(proto))
		clients[compressed][4] = lokerpc.NewClient(jsonV2Server.URL+"/rpc/menu", withHTTP, lokerpc.ClientJSON(jsonengines.V2{}))
		clients[compressed][5] = lokerpc.NewClient(goJSONServer.URL+"/rpc/menu", withHTTP, lokerpc.ClientJSON(jsonengines.GoJSON{}))
		clients[compressed][6] = lokerpc.NewClient(msgpackServer.URL+"/rpc/menu", withHTTP, lokerpc.ClientCodecs(msgpackCodec))
		clients[compressed][7] = lokerpc.NewClient(goJSONOrderedServer.URL+"/rpc/menu", withHTTP, lokerpc.ClientJSON(jsonengines.GoJSON{OptimizeFieldOrder: true}))
	}

	post := func(path int, method string, requestBody []byte) (*http.Response, error) {
		p := paths[path]
		request, err := http.NewRequest(http.MethodPost, p.baseURL+"/rpc/menu/"+method, bytes.NewReader(requestBody))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", p.contentType)
		if p.codec != nil {
			request.Header.Set("Accept", p.accept)
		}
		if p.ifMatch != "" {
			request.Header.Set("If-Match", p.ifMatch)
		}
		return httpClients[0].Do(request)
	}

	// Count response payload bytes before HTTP's automatic decompression,
	// excluding HTTP headers and transport framing.
	for compressed := range 2 {
		var sizes [pathCount]int
		for index, path := range paths {
			request, err := http.NewRequest(http.MethodPost, path.baseURL+"/rpc/menu/encode", bytes.NewReader(path.requestBodies[0]))
			if err != nil {
				b.Fatal(err)
			}
			request.Header.Set("Content-Type", path.contentType)
			if path.codec != nil {
				request.Header.Set("Accept", path.accept)
			}
			if path.ifMatch != "" {
				request.Header.Set("If-Match", path.ifMatch)
			}
			request.Header.Set("Accept-Encoding", "identity")
			if compressed == 1 {
				request.Header.Set("Accept-Encoding", "gzip")
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
			if path.codec != nil && response.Header.Get("Content-Type") != path.codec.ContentType() {
				b.Fatalf("wire %s answered %q, want %q", path.name, response.Header.Get("Content-Type"), path.codec.ContentType())
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
			decoded := path.newMenu()
			if path.codec != nil {
				err = path.codec.Unmarshal(data, decoded, endpoints["encode"].ResponseTypeDef, meta.Definitions)
			} else {
				err = json.Unmarshal(data, decoded)
			}
			if err != nil || !reflect.DeepEqual(path.wantMenu, reflect.ValueOf(decoded).Elem().Interface()) {
				b.Fatalf("wire %s changed menu: %v", path.name, err)
			}
		}
		b.Logf("wire gzip=%t legacy=%d negotiated-json=%d negotiated-proto=%d generated-proto=%d jsonv2=%d gojson=%d msgpack=%d gojson-ordered=%d", compressed == 1, sizes[0], sizes[1], sizes[2], sizes[3], sizes[4], sizes[5], sizes[6], sizes[7])
	}

	// Validate each real client outside the measured blocks.
	for compressed := range 2 {
		for index, client := range clients[compressed] {
			decoded := paths[index].newMenu()
			err := client.DoRequest(context.Background(), "encode", struct{}{}, decoded)
			if err != nil || !reflect.DeepEqual(paths[index].wantMenu, reflect.ValueOf(decoded).Elem().Interface()) {
				b.Fatalf("client %s gzip=%d changed menu: %v", paths[index].name, compressed, err)
			}
			if compressed == 0 {
				decodedSmall := paths[index].newOption()
				err := client.DoRequest(context.Background(), "small", paths[index].optionArg, decodedSmall)
				if err != nil || reflect.ValueOf(decodedSmall).Elem().Interface() != paths[index].wantOption {
					b.Fatalf("client %s changed small response: %v", paths[index].name, err)
				}
			}
		}
	}
	b.Logf("fixture: products=%d options/product=%d compact-JSON=%d protobuf=%d msgpack=%d small-JSON=%d small-protobuf=%d small-msgpack=%d",
		len(typed.Products), len(typed.Products[0].Options), len(body), len(bodyProto), len(bodyMsgpack), len(smallJSON), len(smallProto), len(smallMsgpack))

	cases := []struct {
		name  string
		calls [pathCount]func() error
	}{
		{name: "server-large-response"},
		{name: "server-large-request"},
		{name: "server-small"},
		{name: "roundtrip-large-gzip-off"},
		{name: "roundtrip-large-gzip-on"},
		{name: "roundtrip-small"},
	}
	for index, path := range paths {
		for operation, method := range []string{"encode", "decode", "small"} {
			requestBody := path.requestBodies[operation]
			cases[operation].calls[index] = func() error {
				response, err := post(index, method, requestBody)
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
			client := clients[compressed][index]
			cases[3+compressed].calls[index] = func() error {
				return client.DoRequest(context.Background(), "encode", struct{}{}, path.newMenu())
			}
		}
		client := clients[0][index]
		cases[5].calls[index] = func() error {
			return client.DoRequest(context.Background(), "small", path.optionArg, path.newOption())
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
			for offset := range pathCount {
				index := offset
				if round%2 != 0 {
					index = pathCount - 1 - offset
				}
				start := time.Now()
				for range b.N {
					err := test.calls[index]()
					if err != nil {
						b.Fatalf("%s: %v", paths[index].name, err)
					}
				}
				elapsed := time.Since(start)
				b.ReportMetric(float64(elapsed)/float64(b.N)/1e6, paths[index].name+"-ms/op")
			}
		})
	}
}
