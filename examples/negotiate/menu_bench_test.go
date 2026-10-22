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

func newBenchmarkMenuService(menu benchmarkMenu, opts ...lokerpc.ServiceOption) *lokerpc.Service {
	return lokerpc.NewService("menu", "benchmark", lokerpc.EndpointCodecMap{
		"encode": lokerpc.MakeStandardEndpointCodec(func(context.Context, struct{}) (benchmarkMenu, error) { return menu, nil }, "encode"),
		"decode": lokerpc.MakeStandardEndpointCodec(func(context.Context, benchmarkMenu) (struct{}, error) { return struct{}{}, nil }, "decode"),
		"small":  lokerpc.MakeStandardEndpointCodec(func(_ context.Context, request benchmarkOption) (benchmarkOption, error) { return request, nil }, "small"),
	}, opts...)
}

// Each round times three adjacent per-path blocks, reversing their order between
// -count rounds. All cases use loopback HTTP. Server-only calls drain responses
// without parsing; round trips use the real clients.
func BenchmarkMenuNegotiationHTTP(b *testing.B) {
	proto := protocodec.New()
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
	lokerpc.MountHandlers(kitlog.NewNopLogger(), legacyMux, newBenchmarkMenuService(typed))
	negotiatedMux := http.NewServeMux()
	lokerpc.MountHandlers(kitlog.NewNopLogger(), negotiatedMux, newBenchmarkMenuService(typed, lokerpc.ServeCodecs(proto)))
	legacyServer := httptest.NewServer(benchmarkLegacyGzip(legacyMux))
	negotiatedServer := httptest.NewServer(benchmarkLegacyGzip(negotiatedMux))
	b.Cleanup(legacyServer.Close)
	b.Cleanup(negotiatedServer.Close)

	var httpClients [2]*http.Client
	for compressed := range 2 {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DisableCompression = compressed == 0
		b.Cleanup(transport.CloseIdleConnections)
		httpClients[compressed] = &http.Client{Transport: transport}
	}

	// The service schemas come from the same OPTIONS probe a client sends.
	probe, err := http.NewRequest(http.MethodOptions, negotiatedServer.URL+"/rpc/menu", nil)
	if err != nil {
		b.Fatal(err)
	}
	probeResponse, err := httpClients[0].Do(probe)
	if err != nil {
		b.Fatal(err)
	}
	var meta lokerpc.Meta
	err = json.NewDecoder(probeResponse.Body).Decode(&meta)
	probeResponse.Body.Close()
	if err != nil || probeResponse.StatusCode != http.StatusOK {
		b.Fatalf("probe: status %d: %v", probeResponse.StatusCode, err)
	}
	etag := probeResponse.Header.Get("ETag")
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

	paths := []struct {
		name        string
		baseURL     string
		contentType string
		accept      string
		ifMatch     string
		binary      bool
	}{
		{name: "legacy", baseURL: legacyServer.URL, contentType: "application/json"},
		{name: "negotiated-json", baseURL: negotiatedServer.URL, contentType: "application/json"},
		{"negotiated-proto", negotiatedServer.URL, proto.ContentType(), proto.ContentType() + ", application/json;q=0.5", etag, true},
	}

	var clients [2][3]lokerpc.Client
	for compressed := range 2 {
		withHTTP := lokerpc.WithHTTPClient(httpClients[compressed])
		clients[compressed][0] = lokerpc.NewClient(legacyServer.URL+"/rpc/menu", withHTTP)
		clients[compressed][1] = lokerpc.NewClient(negotiatedServer.URL+"/rpc/menu", withHTTP)
		clients[compressed][2] = lokerpc.NewClient(negotiatedServer.URL+"/rpc/menu", withHTTP, lokerpc.ClientCodecs(proto))
	}

	post := func(path int, method string, requestBody []byte) (*http.Response, error) {
		p := paths[path]
		request, err := http.NewRequest(http.MethodPost, p.baseURL+"/rpc/menu/"+method, bytes.NewReader(requestBody))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", p.contentType)
		if p.binary {
			request.Header.Set("Accept", p.accept)
			request.Header.Set("If-Match", p.ifMatch)
		}
		return httpClients[0].Do(request)
	}

	// Count response payload bytes before HTTP's automatic decompression,
	// excluding HTTP headers and transport framing.
	for compressed := range 2 {
		var sizes [3]int
		for index, path := range paths {
			requestBody := []byte("{}")
			if path.binary {
				requestBody = emptyProto
			}
			request, err := http.NewRequest(http.MethodPost, path.baseURL+"/rpc/menu/encode", bytes.NewReader(requestBody))
			if err != nil {
				b.Fatal(err)
			}
			request.Header.Set("Content-Type", path.contentType)
			if path.binary {
				request.Header.Set("Accept", path.accept)
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
			if path.binary && response.Header.Get("Content-Type") != proto.ContentType() {
				b.Fatalf("wire %s answered %q, want %q", path.name, response.Header.Get("Content-Type"), proto.ContentType())
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
			var decoded benchmarkMenu
			if path.binary {
				err = proto.Unmarshal(data, &decoded, endpoints["encode"].ResponseTypeDef, meta.Definitions)
			} else {
				err = json.Unmarshal(data, &decoded)
			}
			if err != nil || !reflect.DeepEqual(typed, decoded) {
				b.Fatalf("wire %s changed menu: %v", path.name, err)
			}
		}
		b.Logf("wire gzip=%t legacy=%d negotiated-json=%d negotiated-proto=%d", compressed == 1, sizes[0], sizes[1], sizes[2])
	}

	// Validate each real client outside the measured blocks.
	for compressed := range 2 {
		for index, client := range clients[compressed] {
			var decoded benchmarkMenu
			err := client.DoRequest(context.Background(), "encode", struct{}{}, &decoded)
			if err != nil || !reflect.DeepEqual(typed, decoded) {
				b.Fatalf("client %s gzip=%d changed menu: %v", paths[index].name, compressed, err)
			}
			if compressed == 0 {
				var decodedSmall benchmarkOption
				err := client.DoRequest(context.Background(), "small", small, &decodedSmall)
				if err != nil || decodedSmall != small {
					b.Fatalf("client %s changed small response: %v", paths[index].name, err)
				}
			}
		}
	}
	b.Logf("fixture: products=%d options/product=%d compact-JSON=%d protobuf=%d small-JSON=%d small-protobuf=%d",
		len(typed.Products), len(typed.Products[0].Options), len(body), len(bodyProto), len(smallJSON), len(smallProto))

	cases := []struct {
		name  string
		calls [3]func() error
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
			requestBody := [][]byte{[]byte("{}"), body, smallJSON}[operation]
			if path.binary {
				requestBody = [][]byte{emptyProto, bodyProto, smallProto}[operation]
			}
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
				var response benchmarkMenu
				return client.DoRequest(context.Background(), "encode", struct{}{}, &response)
			}
		}
		client := clients[0][index]
		cases[5].calls[index] = func() error {
			var response benchmarkOption
			return client.DoRequest(context.Background(), "small", small, &response)
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
			for offset := range 3 {
				index := offset
				if round%2 != 0 {
					index = 2 - offset
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
