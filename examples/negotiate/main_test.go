package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LOKE/pkg/errors"
	"github.com/LOKE/pkg/examples/negotiate/protocodec"
	"github.com/LOKE/pkg/lokerpc"
	kitlog "github.com/go-kit/log"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type Node struct {
	Name     string `json:"name"`
	Children []Node `json:"children"`
}

type Edge struct {
	OptInt     *int             `json:"optInt,omitempty"`
	Negative   int              `json:"negative"`
	Small      int8             `json:"small"`
	Unsigned   uint32           `json:"unsigned"`
	Float32    float32          `json:"float32"`
	Float64    float64          `json:"float64"`
	Names      []string         `json:"names"`
	Pointers   []*string        `json:"pointers"`
	Matrix     [][]int          `json:"matrix"`
	Groups     map[string][]int `json:"groups"`
	Tree       *Node            `json:"tree,omitempty"`
	When       time.Time        `json:"when"`
	Anything   any              `json:"anything"`
	Hyphenated string           `json:"hy-phen"`
	Addr       netip.Addr       `json:"addr"`
}

type etagStripper struct {
	http.ResponseWriter
}

func (w *etagStripper) WriteHeader(status int) {
	w.Header().Del("ETag")
	w.ResponseWriter.WriteHeader(status)
}

type testCodec struct {
	name  string
	codec lokerpc.Codec
}

var testCodecs = []testCodec{
	{"protobuf", protocodec.New()},
}

var compareOpts = cmp.Options{
	cmpopts.EquateEmpty(),
	cmpopts.EquateComparable(netip.Addr{}),
}

func serve(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func serveService(t *testing.T, service *lokerpc.Service) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	lokerpc.MountHandlers(kitlog.NewNopLogger(), mux, service)
	return serve(t, mux)
}

func newRecordedClient(url string, codecs ...lokerpc.Codec) (lokerpc.Client, *Recorder) {
	recorder := &Recorder{}
	opts := []lokerpc.ClientOption{lokerpc.WithHTTPClient(&http.Client{Transport: recorder})}
	if len(codecs) > 0 {
		opts = append(opts, lokerpc.ClientCodecs(codecs...))
	}
	return lokerpc.NewClient(url, opts...), recorder
}

func allCodecs() []lokerpc.Codec {
	codecs := make([]lokerpc.Codec, len(testCodecs))
	for i, tc := range testCodecs {
		codecs[i] = tc.codec
	}
	return codecs
}

func countMethod(exchanges []Exchange, method string) int {
	count := 0
	for _, exchange := range exchanges {
		if exchange.Method == method {
			count++
		}
	}
	return count
}

func mustDo(t *testing.T, client lokerpc.Client, method string, args, result any) {
	t.Helper()
	err := client.DoRequest(context.Background(), method, args, result)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
}

func rawRequest(t *testing.T, method, url string, headers map[string]string, body []byte) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res, resBody
}

type thingV1 struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type thingV2 struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Extra string `json:"extra"`
}

type itemServer struct {
	ID    int      `json:"id"`
	Name  string   `json:"name"`
	Extra string   `json:"extra"`
	Tags  []string `json:"tags"`
}

type itemRequestServer struct {
	ID   int    `json:"id"`
	Note string `json:"note"`
}

func TestNegotiation(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, tc testCodec)
	}{
		{"binary in both directions", func(t *testing.T, tc testCodec) {
			server := serveService(t, newMenuService(allCodecs()...))
			client, recorder := newRecordedClient(server.URL+"/rpc/menu", tc.codec)
			menu := MenuClient{client}

			want := seedMenu(5)
			_, err := menu.SaveMenu(context.Background(), want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := menu.GetMenu(context.Background(), GetMenuRequest{MenuID: want.ID})
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(want, got, compareOpts); diff != "" {
				t.Errorf("menu mismatch (-want +got):\n%s", diff)
			}

			err = menu.DeleteMenu(context.Background(), DeleteMenuRequest{MenuID: want.ID})
			if err != nil {
				t.Fatal(err)
			}
			_, err = menu.GetMenu(context.Background(), GetMenuRequest{MenuID: want.ID})
			if !errors.IsPublic(err) {
				t.Errorf("get after delete: want public error, got %v", err)
			}

			posts := 0
			for _, exchange := range recorder.Take() {
				if exchange.Method != http.MethodPost || exchange.Status != http.StatusOK {
					continue
				}
				posts++
				if exchange.RequestType != tc.codec.ContentType() || exchange.ResponseType != tc.codec.ContentType() {
					t.Errorf("%s: wire types request=%q response=%q, want %q", exchange.Path, exchange.RequestType, exchange.ResponseType, tc.codec.ContentType())
				}
				if json.Valid(exchange.RequestBody) || json.Valid(exchange.ResponseBody) {
					t.Errorf("%s: body looks like JSON", exchange.Path)
				}
			}
			if posts != 3 {
				t.Errorf("successful POSTs = %d, want 3", posts)
			}
		}},
		{"plain JSON client gets today's JSON", func(t *testing.T, tc testCodec) {
			withCodecs := serveService(t, newMenuService(allCodecs()...))
			withoutCodecs := serveService(t, newMenuService())

			var bodies [2][]byte
			for i, server := range []*httptest.Server{withCodecs, withoutCodecs} {
				client, recorder := newRecordedClient(server.URL + "/rpc/menu")
				menu := MenuClient{client}

				_, err := menu.SaveMenu(context.Background(), seedMenu(3))
				if err != nil {
					t.Fatal(err)
				}
				_, err = menu.GetMenu(context.Background(), GetMenuRequest{MenuID: "lunch"})
				if err != nil {
					t.Fatal(err)
				}

				exchanges := recorder.Take()
				if len(exchanges) != 2 {
					t.Fatalf("exchanges = %d, want 2 POSTs and no probe", len(exchanges))
				}
				for _, exchange := range exchanges {
					if exchange.Method != http.MethodPost || exchange.IfMatch != "" || !strings.HasPrefix(exchange.ResponseType, "application/json") {
						t.Errorf("unexpected exchange %+v", exchange)
					}
				}
				bodies[i] = exchanges[1].ResponseBody
			}

			if !bytes.Equal(bodies[0], bodies[1]) {
				t.Errorf("JSON response differs with codecs enabled:\n%s\n%s", bodies[0], bodies[1])
			}
		}},
		{"old server falls back to JSON and is remembered", func(t *testing.T, tc testCodec) {
			server := serveService(t, newMenuService())
			client, recorder := newRecordedClient(server.URL+"/rpc/menu", tc.codec)
			menu := MenuClient{client}

			want := seedMenu(2)
			_, err := menu.SaveMenu(context.Background(), want)
			if err != nil {
				t.Fatal(err)
			}

			exchanges := recorder.Take()
			if len(exchanges) != 2 || exchanges[0].Method != http.MethodOptions || exchanges[0].Status != http.StatusMethodNotAllowed {
				t.Fatalf("first call exchanges = %+v, want OPTIONS 405 then POST", exchanges)
			}
			if !strings.HasPrefix(exchanges[1].RequestType, "application/json") || !strings.HasPrefix(exchanges[1].ResponseType, "application/json") {
				t.Errorf("fallback POST was not JSON: %+v", exchanges[1])
			}

			got, err := menu.GetMenu(context.Background(), GetMenuRequest{MenuID: want.ID})
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(want, got, compareOpts); diff != "" {
				t.Errorf("menu mismatch (-want +got):\n%s", diff)
			}
			if probes := countMethod(recorder.Take(), http.MethodOptions); probes != 0 {
				t.Errorf("second call probed %d times, want 0", probes)
			}
		}},
		{"negotiation is cached across calls", func(t *testing.T, tc testCodec) {
			server := serveService(t, newMenuService(allCodecs()...))
			client, recorder := newRecordedClient(server.URL+"/rpc/menu", tc.codec)
			menu := MenuClient{client}

			_, err := menu.GetMenu(context.Background(), GetMenuRequest{MenuID: "nope"})
			if !errors.IsPublic(err) {
				t.Fatalf("want public error, got %v", err)
			}
			if probes := countMethod(recorder.Take(), http.MethodOptions); probes != 1 {
				t.Errorf("first call probes = %d, want 1", probes)
			}

			_, err = menu.GetMenu(context.Background(), GetMenuRequest{MenuID: "nope"})
			if !errors.IsPublic(err) {
				t.Fatalf("want public error, got %v", err)
			}
			if probes := countMethod(recorder.Take(), http.MethodOptions); probes != 0 {
				t.Errorf("second call probes = %d, want 0", probes)
			}
		}},
		{"public errors match JSON behaviour", func(t *testing.T, tc testCodec) {
			server := serveService(t, newMenuService(allCodecs()...))
			jsonClient, _ := newRecordedClient(server.URL + "/rpc/menu")
			binaryClient, recorder := newRecordedClient(server.URL+"/rpc/menu", tc.codec)

			request := GetMenuRequest{MenuID: "missing"}
			_, jsonErr := MenuClient{jsonClient}.GetMenu(context.Background(), request)
			_, binaryErr := MenuClient{binaryClient}.GetMenu(context.Background(), request)

			if jsonErr == nil || binaryErr == nil {
				t.Fatalf("want errors, got json=%v binary=%v", jsonErr, binaryErr)
			}
			if jsonErr.Error() != binaryErr.Error() ||
				errors.ErrorCode(jsonErr) != errors.ErrorCode(binaryErr) ||
				errors.IsPublic(jsonErr) != errors.IsPublic(binaryErr) {
				t.Errorf("errors differ: json=%v (%s, public=%v) binary=%v (%s, public=%v)",
					jsonErr, errors.ErrorCode(jsonErr), errors.IsPublic(jsonErr),
					binaryErr, errors.ErrorCode(binaryErr), errors.IsPublic(binaryErr))
			}
			if errors.ErrorCode(binaryErr) != "menu_not_found" || !errors.IsPublic(binaryErr) {
				t.Errorf("binary error = %v code=%q public=%v", binaryErr, errors.ErrorCode(binaryErr), errors.IsPublic(binaryErr))
			}

			for _, exchange := range recorder.Take() {
				if exchange.Method == http.MethodPost && (exchange.Status != http.StatusBadRequest || !strings.HasPrefix(exchange.ResponseType, "application/json")) {
					t.Errorf("error response should be JSON 400, got %+v", exchange)
				}
			}
		}},
		{"schema change triggers 412, JSON retry and renegotiation", func(t *testing.T, tc testCodec) {
			v1 := lokerpc.NewService("thing", "", lokerpc.EndpointCodecMap{
				"echo": lokerpc.MakeStandardEndpointCodec(func(_ context.Context, in thingV1) (thingV1, error) {
					return in, nil
				}, ""),
			}, lokerpc.ServeCodecs(tc.codec))
			v2 := lokerpc.NewService("thing", "", lokerpc.EndpointCodecMap{
				"echo": lokerpc.MakeStandardEndpointCodec(func(_ context.Context, in thingV2) (thingV2, error) {
					in.Extra = "added by v2"
					return in, nil
				}, ""),
			}, lokerpc.ServeCodecs(tc.codec))

			mount := func(service *lokerpc.Service) http.Handler {
				mux := http.NewServeMux()
				lokerpc.MountHandlers(kitlog.NewNopLogger(), mux, service)
				return mux
			}
			var current atomic.Pointer[http.Handler]
			first := mount(v1)
			current.Store(&first)
			server := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				(*current.Load()).ServeHTTP(w, r)
			}))

			client, recorder := newRecordedClient(server.URL+"/rpc/thing", tc.codec)
			call := func() thingV1 {
				var res thingV1
				mustDo(t, client, "echo", thingV1{ID: 7, Name: "seven"}, &res)
				return res
			}
			want := thingV1{ID: 7, Name: "seven"}

			if diff := cmp.Diff(want, call()); diff != "" {
				t.Fatalf("v1 call mismatch (-want +got):\n%s", diff)
			}
			oldETag := recorder.Take()[0].ETag

			second := mount(v2)
			current.Store(&second)

			if diff := cmp.Diff(want, call()); diff != "" {
				t.Fatalf("call after schema change mismatch (-want +got):\n%s", diff)
			}
			exchanges := recorder.Take()
			if len(exchanges) != 2 ||
				exchanges[0].Status != http.StatusPreconditionFailed ||
				exchanges[0].RequestType != tc.codec.ContentType() ||
				exchanges[1].Status != http.StatusOK ||
				!strings.HasPrefix(exchanges[1].RequestType, "application/json") {
				t.Fatalf("want binary 412 then JSON 200, got %+v", exchanges)
			}

			if diff := cmp.Diff(want, call()); diff != "" {
				t.Fatalf("renegotiated call mismatch (-want +got):\n%s", diff)
			}
			exchanges = recorder.Take()
			if len(exchanges) != 2 || exchanges[0].Method != http.MethodOptions || exchanges[0].ETag == oldETag || exchanges[0].ETag == "" {
				t.Fatalf("want a fresh probe with a new ETag, got %+v", exchanges)
			}
			if exchanges[1].IfMatch != exchanges[0].ETag || exchanges[1].ResponseType != tc.codec.ContentType() {
				t.Errorf("renegotiated POST should be binary with the new ETag, got %+v", exchanges[1])
			}
		}},
		{"binary POST landing on a replica without codecs falls back to JSON", func(t *testing.T, tc testCodec) {
			endpoints := lokerpc.EndpointCodecMap{
				"echo": lokerpc.MakeStandardEndpointCodec(func(_ context.Context, in thingV1) (thingV1, error) {
					return in, nil
				}, ""),
			}
			mount := func(service *lokerpc.Service) http.Handler {
				mux := http.NewServeMux()
				lokerpc.MountHandlers(kitlog.NewNopLogger(), mux, service)
				return mux
			}
			var current atomic.Pointer[http.Handler]
			withCodecs := mount(lokerpc.NewService("thing", "", endpoints, lokerpc.ServeCodecs(tc.codec)))
			current.Store(&withCodecs)
			server := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				(*current.Load()).ServeHTTP(w, r)
			}))

			client, recorder := newRecordedClient(server.URL+"/rpc/thing", tc.codec)
			want := thingV1{ID: 1, Name: "one"}
			call := func() {
				var res thingV1
				mustDo(t, client, "echo", want, &res)
				if diff := cmp.Diff(want, res); diff != "" {
					t.Fatalf("mismatch (-want +got):\n%s", diff)
				}
			}

			call()
			recorder.Take()

			withoutCodecs := mount(lokerpc.NewService("thing", "", endpoints))
			current.Store(&withoutCodecs)

			call()
			exchanges := recorder.Take()
			if len(exchanges) != 2 || exchanges[0].RequestType != tc.codec.ContentType() ||
				exchanges[0].Status != http.StatusBadRequest ||
				exchanges[1].Status != http.StatusOK || !strings.HasPrefix(exchanges[1].RequestType, "application/json") {
				t.Fatalf("want rejected binary POST then JSON 200, got %+v", exchanges)
			}

			call()
			exchanges = recorder.Take()
			if len(exchanges) != 2 || exchanges[0].Method != http.MethodOptions || exchanges[0].Status != http.StatusMethodNotAllowed {
				t.Errorf("next call should re-probe, got %+v", exchanges)
			}
		}},
		{"probe response without an ETag means JSON only", func(t *testing.T, tc testCodec) {
			mux := http.NewServeMux()
			lokerpc.MountHandlers(kitlog.NewNopLogger(), mux, lokerpc.NewService("thing", "", lokerpc.EndpointCodecMap{
				"echo": lokerpc.MakeStandardEndpointCodec(func(_ context.Context, in thingV1) (thingV1, error) {
					return in, nil
				}, ""),
			}, lokerpc.ServeCodecs(tc.codec)))
			server := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mux.ServeHTTP(&etagStripper{ResponseWriter: w}, r)
			}))

			client, recorder := newRecordedClient(server.URL+"/rpc/thing", tc.codec)
			for range 2 {
				var res thingV1
				mustDo(t, client, "echo", thingV1{ID: 2, Name: "two"}, &res)
			}

			exchanges := recorder.Take()
			if len(exchanges) != 3 || countMethod(exchanges, http.MethodOptions) != 1 {
				t.Fatalf("want one probe and two POSTs, got %+v", exchanges)
			}
			for _, exchange := range exchanges[1:] {
				if exchange.IfMatch != "" || !strings.HasPrefix(exchange.RequestType, "application/json") {
					t.Errorf("POST should be plain JSON, got %+v", exchange)
				}
			}
		}},
		{"older and newer client structs match JSON behaviour", func(t *testing.T, tc testCodec) {
			service := lokerpc.NewService("items", "", lokerpc.EndpointCodecMap{
				"getItem": lokerpc.MakeStandardEndpointCodec(func(_ context.Context, in itemRequestServer) (itemServer, error) {
					return itemServer{ID: in.ID, Name: "n:" + in.Note, Extra: "x", Tags: []string{"a", "b"}}, nil
				}, ""),
			}, lokerpc.ServeCodecs(tc.codec))
			server := serveService(t, service)

			type oldRequest struct {
				ID int `json:"id"`
			}
			type oldItem struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			}
			type newerRequest struct {
				ID       int    `json:"id"`
				Note     string `json:"note"`
				NotKnown string `json:"notKnown"`
			}
			type newerItem struct {
				ID       int      `json:"id"`
				Name     string   `json:"name"`
				Tags     []string `json:"tags"`
				NotKnown string   `json:"notKnown"`
			}

			tests := []struct {
				name    string
				request any
				result  func() any
			}{
				{"client older than server", oldRequest{ID: 3}, func() any { return &oldItem{} }},
				{"client has fields the server lacks", newerRequest{ID: 4, Note: "hi", NotKnown: "ignored"}, func() any { return &newerItem{} }},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					jsonClient, _ := newRecordedClient(server.URL + "/rpc/items")
					binaryClient, recorder := newRecordedClient(server.URL+"/rpc/items", tc.codec)

					jsonResult, binaryResult := tt.result(), tt.result()
					mustDo(t, jsonClient, "getItem", tt.request, jsonResult)
					mustDo(t, binaryClient, "getItem", tt.request, binaryResult)

					if diff := cmp.Diff(jsonResult, binaryResult, compareOpts); diff != "" {
						t.Errorf("binary differs from JSON (-json +binary):\n%s", diff)
					}
					posts := recorder.Take()
					if last := posts[len(posts)-1]; last.ResponseType != tc.codec.ContentType() {
						t.Errorf("expected a binary response, got %q", last.ResponseType)
					}
				})
			}
		}},
		{"meta endpoints are identical with and without codecs", func(t *testing.T, tc testCodec) {
			withCodecs := serveService(t, newMenuService(allCodecs()...))
			withoutCodecs := serveService(t, newMenuService())

			for _, path := range []string{"/rpc", "/rpc/menu"} {
				_, withBody := rawRequest(t, http.MethodGet, withCodecs.URL+path, nil, nil)
				_, withoutBody := rawRequest(t, http.MethodGet, withoutCodecs.URL+path, nil, nil)
				if !bytes.Equal(withBody, withoutBody) {
					t.Errorf("GET %s differs:\n%s\n%s", path, withBody, withoutBody)
				}
			}

			res, _ := rawRequest(t, http.MethodOptions, withoutCodecs.URL+"/rpc/menu", nil, nil)
			if res.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("OPTIONS without codecs = %d, want 405", res.StatusCode)
			}

			res, body := rawRequest(t, http.MethodOptions, withCodecs.URL+"/rpc/menu", nil, nil)
			_, getBody := rawRequest(t, http.MethodGet, withCodecs.URL+"/rpc/menu", nil, nil)
			if res.StatusCode != http.StatusOK || !bytes.Equal(body, getBody) {
				t.Errorf("OPTIONS = %d, body equals GET meta: %v", res.StatusCode, bytes.Equal(body, getBody))
			}
			for header, want := range map[string]string{
				"Allow":         "GET, POST, OPTIONS",
				"Accept-Post":   "application/json, application/x-protobuf",
				"Cache-Control": "max-age=300",
			} {
				if got := res.Header.Get(header); got != want {
					t.Errorf("%s = %q, want %q", header, got, want)
				}
			}
			if etag := res.Header.Get("ETag"); !strings.HasPrefix(etag, `"`) || len(etag) != 66 {
				t.Errorf("ETag = %q, want quoted sha256", etag)
			}
		}},
		{"invalid binary requests are rejected", func(t *testing.T, tc testCodec) {
			custom := lokerpc.EndpointCodec{
				Endpoint: func(context.Context, any) (any, error) { return nil, nil },
				Decode:   func(context.Context, json.RawMessage) (any, error) { return nil, nil },
			}
			service := lokerpc.NewService("menu", "", lokerpc.EndpointCodecMap{
				"getMenu": lokerpc.MakeStandardEndpointCodec(func(context.Context, GetMenuRequest) (Menu, error) { return Menu{}, nil }, ""),
				"custom":  custom,
			}, lokerpc.ServeCodecs(tc.codec))
			server := serveService(t, service)

			res, _ := rawRequest(t, http.MethodOptions, server.URL+"/rpc/menu", nil, nil)
			etag := res.Header.Get("ETag")
			contentType := tc.codec.ContentType()

			tests := []struct {
				name       string
				path       string
				headers    map[string]string
				body       []byte
				wantStatus int
				wantBody   string
			}{
				{"garbage body", "getMenu", map[string]string{"Content-Type": contentType, "If-Match": etag}, []byte("\xff\xff\xffgarbage\x00\x01"), 400, "Invalid request"},
				{"truncated body", "getMenu", map[string]string{"Content-Type": contentType, "If-Match": etag}, []byte{0x0a, 0x7f, 0x01}, 400, "Invalid request"},
				{"wrong If-Match", "getMenu", map[string]string{"Content-Type": contentType, "If-Match": `"stale"`}, nil, 412, `{"message":"schema changed"}`},
				{"missing If-Match", "getMenu", map[string]string{"Content-Type": contentType}, nil, 412, `{"message":"schema changed"}`},
				{"binary Accept with wrong If-Match", "getMenu", map[string]string{"Content-Type": "application/json", "Accept": contentType, "If-Match": `"stale"`}, []byte(`{}`), 412, `{"message":"schema changed"}`},
				{"codec on endpoint without request type", "custom", map[string]string{"Content-Type": contentType, "If-Match": etag}, nil, 415, "does not support"},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					res, body := rawRequest(t, http.MethodPost, server.URL+"/rpc/menu/"+tt.path, tt.headers, tt.body)
					if res.StatusCode != tt.wantStatus {
						t.Errorf("status = %d, want %d (%s)", res.StatusCode, tt.wantStatus, body)
					}
					if !strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") || !json.Valid(body) {
						t.Errorf("error should be JSON, got %q %q", res.Header.Get("Content-Type"), body)
					}
					if !strings.Contains(string(body), tt.wantBody) {
						t.Errorf("body = %s, want it to contain %q", body, tt.wantBody)
					}
				})
			}

			t.Run("JSON request, binary Accept", func(t *testing.T) {
				headers := map[string]string{"Content-Type": "application/json", "Accept": contentType + ", application/json;q=0.5", "If-Match": etag}
				res, _ := rawRequest(t, http.MethodPost, server.URL+"/rpc/menu/getMenu", headers, []byte(`{"menuId":"x"}`))
				if res.StatusCode != 200 || res.Header.Get("Content-Type") != contentType || res.Header.Get("Vary") != "Accept" {
					t.Errorf("status=%d type=%q vary=%q", res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("Vary"))
				}
			})
		}},
		{"edge values round-trip like JSON", func(t *testing.T, tc testCodec) {
			service := lokerpc.NewService("edge", "", lokerpc.EndpointCodecMap{
				"echo": lokerpc.MakeStandardEndpointCodec(func(_ context.Context, in Edge) (Edge, error) { return in, nil }, ""),
			}, lokerpc.ServeCodecs(tc.codec))
			server := serveService(t, service)

			utcPlus10 := time.FixedZone("UTC+10", 10*60*60)
			tests := []struct {
				name  string
				edge  Edge
				check func(t *testing.T, got Edge)
			}{
				{name: "zero value"},
				{name: "optional int absent", edge: Edge{Negative: 1}, check: func(t *testing.T, got Edge) {
					if got.OptInt != nil {
						t.Errorf("OptInt = %d, want nil", *got.OptInt)
					}
				}},
				{name: "optional int pointer to zero", edge: Edge{OptInt: new(0)}, check: func(t *testing.T, got Edge) {
					if got.OptInt == nil || *got.OptInt != 0 {
						t.Errorf("OptInt = %v, want pointer to 0", got.OptInt)
					}
				}},
				{name: "negative and extreme ints", edge: Edge{OptInt: new(math.MinInt32), Negative: math.MinInt32, Small: -128, Unsigned: math.MaxUint32}},
				{name: "positive extremes", edge: Edge{OptInt: new(math.MaxInt32), Negative: math.MaxInt32, Small: 127}},
				{name: "floats", edge: Edge{Float32: -1.5, Float64: math.SmallestNonzeroFloat64}},
				{name: "nested slices", edge: Edge{Matrix: [][]int{{1, 2}, {}, {3, -4}}}},
				{name: "nullable elements", edge: Edge{Pointers: []*string{new("a"), nil, new("")}}, check: func(t *testing.T, got Edge) {
					if len(got.Pointers) != 3 || got.Pointers[1] != nil || got.Pointers[2] == nil {
						t.Errorf("Pointers = %v, want [a nil ptr-to-empty]", got.Pointers)
					}
				}},
				{name: "map of slices", edge: Edge{Groups: map[string][]int{"a": {1, 2}, "b": {-1}, "": {0}}}},
				{name: "recursive tree", edge: Edge{Tree: &Node{Name: "root", Children: []Node{{Name: "a", Children: []Node{{Name: "a1"}}}, {Name: "b"}}}}},
				{name: "time with zone and nanos", edge: Edge{When: time.Date(2026, 7, 8, 9, 10, 11, 123456789, utcPlus10)}},
				{name: "time before the epoch", edge: Edge{When: time.Date(1901, 2, 3, 4, 5, 6, 7, time.UTC)}},
				{name: "any field", edge: Edge{Anything: map[string]any{"k": []any{1.0, "two", nil, true}}}},
				{name: "hyphenated key and unicode", edge: Edge{Hyphenated: "ünï-☃-\x00"}},
				{name: "text marshaler", edge: Edge{Addr: netip.MustParseAddr("2001:db8::1")}},
				{name: "empty slices and maps", edge: Edge{Names: []string{}, Matrix: [][]int{}, Groups: map[string][]int{}}, check: func(t *testing.T, got Edge) {
					if got.Names != nil || got.Matrix != nil || got.Groups != nil {
						t.Errorf("want empty containers to decode as nil, got %#v %#v %#v", got.Names, got.Matrix, got.Groups)
					}
				}},
			}

			jsonClient, _ := newRecordedClient(server.URL + "/rpc/edge")
			binaryClient, recorder := newRecordedClient(server.URL+"/rpc/edge", tc.codec)

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					var viaJSON, viaBinary Edge
					mustDo(t, jsonClient, "echo", tt.edge, &viaJSON)
					mustDo(t, binaryClient, "echo", tt.edge, &viaBinary)

					if diff := cmp.Diff(viaJSON, viaBinary, compareOpts); diff != "" {
						t.Errorf("binary differs from JSON (-json +binary):\n%s", diff)
					}
					if tt.check != nil {
						tt.check(t, viaBinary)
					}
					for _, exchange := range recorder.Take() {
						if exchange.Method == http.MethodPost && exchange.ResponseType != tc.codec.ContentType() {
							t.Errorf("response type = %q, want binary", exchange.ResponseType)
						}
					}
				})
			}

			t.Run("nil pointer response", func(t *testing.T) {
				type reply struct {
					Name string `json:"name"`
				}
				nilService := lokerpc.NewService("nilreply", "", lokerpc.EndpointCodecMap{
					"get": lokerpc.MakeStandardEndpointCodec(func(context.Context, struct{}) (*reply, error) { return nil, nil }, ""),
				}, lokerpc.ServeCodecs(tc.codec))
				nilClient, _ := newRecordedClient(serveService(t, nilService).URL+"/rpc/nilreply", tc.codec)

				res := &reply{Name: "untouched"}
				mustDo(t, nilClient, "get", struct{}{}, &res)
				if res != nil {
					t.Errorf("result = %+v, want nil", res)
				}
			})

			t.Run("int overflow is an error", func(t *testing.T) {
				recorder.Take()
				var res Edge
				err := binaryClient.DoRequest(context.Background(), "echo", Edge{Negative: math.MaxInt32 + 1}, &res)
				if err == nil {
					t.Fatal("want overflow error, got nil")
				}
				if posts := countMethod(recorder.Take(), http.MethodPost); posts != 0 {
					t.Errorf("overflowing request was sent (%d POSTs)", posts)
				}
			})
		}},
	}

	for _, tc := range testCodecs {
		t.Run(tc.name, func(t *testing.T) {
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					c.run(t, tc)
				})
			}
		})
	}
}
