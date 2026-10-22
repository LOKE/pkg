package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"reflect"
	"time"

	"github.com/LOKE/pkg/examples/negotiate/fixtures"
	"github.com/LOKE/pkg/examples/negotiate/msgpackcodec"
	"github.com/LOKE/pkg/examples/negotiate/protocodec"
	"github.com/LOKE/pkg/lokerpc"
	kitlog "github.com/go-kit/log"
	jtd "github.com/jsontypedef/json-typedef-go"
)

// The node/ tests drive these subcommands: serve hosts the menu, edge and bench
// services, dump prints the Go wire bytes for fixed values, and call runs a Go
// client against a server started by the Node tests.
func runInterop(args []string) error {
	switch args[0] {
	case "serve":
		return serveInterop(args[1:])
	case "dump":
		return dumpWire()
	case "call":
		return callServer(args[1:])
	}
	return fmt.Errorf("unknown subcommand %q, want serve, dump or call", args[0])
}

func serveInterop(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := flags.String("addr", "127.0.0.1:0", "listen address")
	plain := flags.Bool("plain", false, "serve without binary codecs, like a server that predates them")
	flags.Parse(args)

	var codecs []lokerpc.Codec
	if !*plain {
		codecs = []lokerpc.Codec{protocodec.New(), msgpackcodec.New()}
	}
	var opts []lokerpc.ServiceOption
	if len(codecs) > 0 {
		opts = append(opts, lokerpc.ServeCodecs(codecs...))
	}

	edge := lokerpc.NewService("edge", "", lokerpc.EndpointCodecMap{
		"echo": lokerpc.MakeStandardEndpointCodec(func(_ context.Context, in fixtures.Edge) (fixtures.Edge, error) { return in, nil }, ""),
	}, opts...)
	bench := lokerpc.NewService("bench", "", lokerpc.EndpointCodecMap{
		"encode": lokerpc.MakeStandardEndpointCodec(func(context.Context, struct{}) (fixtures.BenchMenu, error) { return fixtures.LargeMenu(), nil }, ""),
		"decode": lokerpc.MakeStandardEndpointCodec(func(_ context.Context, in fixtures.BenchMenu) (int, error) { return len(in.Products), nil }, ""),
	}, opts...)

	mux := http.NewServeMux()
	lokerpc.MountHandlers(kitlog.NewNopLogger(), mux, newMenuService(codecs...), edge, bench)

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	fmt.Printf("listening http://%s\n", listener.Addr())

	server := &http.Server{Handler: mux}
	go func() {
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)
		<-interrupt
		server.Close()
	}()
	err = server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

type interopCase struct {
	Name   string
	Schema string
	Value  any
}

// interopEdges avoid the zero netip.Addr, which the Go codec writes as an empty
// length-delimited field that other implementations have no reason to repeat.
func interopEdges() []interopCase {
	addr := netip.MustParseAddr("2001:db8::1")
	one, str, empty := 1, "a", ""
	edges := []struct {
		name string
		edge fixtures.Edge
	}{
		{"zero value", fixtures.Edge{}},
		{"optional int zero", fixtures.Edge{OptInt: new(0)}},
		{"extreme ints", fixtures.Edge{OptInt: new(-2147483648), Negative: -2147483648, Small: -128, Unsigned: 4294967295}},
		{"positive extremes", fixtures.Edge{OptInt: &one, Negative: 2147483647, Small: 127}},
		{"floats", fixtures.Edge{Float32: -1.5, Float64: 5e-324}},
		{"nested slices", fixtures.Edge{Matrix: [][]int{{1, 2}, {}, {3, -4}}}},
		{"nullable elements", fixtures.Edge{Pointers: []*string{&str, nil, &empty}}},
		{"map of slices", fixtures.Edge{Groups: map[string][]int{"a": {1, 2}, "b": {-1}, "": {0}}}},
		{"recursive tree", fixtures.Edge{Tree: &fixtures.Node{Name: "root", Children: []fixtures.Node{{Name: "a", Children: []fixtures.Node{{Name: "a1"}}}, {Name: "b"}}}}},
		{"time with nanos", fixtures.Edge{When: time.Date(2026, 7, 8, 9, 10, 11, 123456789, time.UTC)}},
		{"time at the epoch", fixtures.Edge{When: time.Unix(0, 0).UTC()}},
		{"time before the epoch", fixtures.Edge{When: time.Date(1901, 2, 3, 4, 5, 6, 7, time.UTC)}},
		{"any field", fixtures.Edge{Anything: map[string]any{"k": []any{1.0, "two", nil, true}}}},
		{"hyphenated and unicode", fixtures.Edge{Hyphenated: "ünï-☃-\x00"}},
		{"long strings and lists", fixtures.Edge{Names: longNames()}},
	}

	var out []interopCase
	for _, e := range edges {
		e.edge.Addr = addr
		out = append(out, interopCase{Name: "edge/" + e.name, Schema: "edge", Value: e.edge})
	}
	return out
}

func longNames() []string {
	names := make([]string, 40)
	for i := range names {
		names[i] = fmt.Sprintf("name-%d-%0200d", i, i)
	}
	return names
}

func interopCases() []interopCase {
	cases := interopEdges()
	cases = append(cases,
		interopCase{Name: "menu/seed5", Schema: "menu", Value: seedMenu(5)},
		interopCase{Name: "bench/large", Schema: "bench", Value: fixtures.LargeMenu()},
	)
	return cases
}

var interopTypes = map[string]reflect.Type{
	"edge":  reflect.TypeFor[fixtures.Edge](),
	"menu":  reflect.TypeFor[Menu](),
	"bench": reflect.TypeFor[fixtures.BenchMenu](),
}

type wireSchema struct {
	Schema *jtd.Schema           `json:"schema"`
	Defs   map[string]jtd.Schema `json:"defs"`
}

// normalizedSchema round-trips through JSON so the codec sees the schema the way
// a client of the served meta does.
func normalizedSchema(t reflect.Type) (*jtd.Schema, map[string]jtd.Schema, error) {
	typeDefs := map[reflect.Type]*lokerpc.NamedSchema{}
	schema := lokerpc.TypeSchema(t, typeDefs)
	encoded, err := json.Marshal(wireSchema{schema, lokerpc.TypeDefs(typeDefs)})
	if err != nil {
		return nil, nil, err
	}
	var wire wireSchema
	err = json.Unmarshal(encoded, &wire)
	return wire.Schema, wire.Defs, err
}

func dumpWire() error {
	proto := protocodec.New()
	msgpack := msgpackcodec.New()

	type dumped struct {
		Name    string          `json:"name"`
		Schema  string          `json:"schema"`
		JSON    json.RawMessage `json:"json"`
		Proto   string          `json:"proto"`
		Msgpack string          `json:"msgpack"`
	}
	out := struct {
		Schemas map[string]wireSchema `json:"schemas"`
		Cases   []dumped              `json:"cases"`
	}{Schemas: map[string]wireSchema{}}

	for name, typ := range interopTypes {
		schema, defs, err := normalizedSchema(typ)
		if err != nil {
			return err
		}
		out.Schemas[name] = wireSchema{schema, defs}
	}

	for _, c := range interopCases() {
		wire := out.Schemas[c.Schema]
		encodedJSON, err := json.Marshal(c.Value)
		if err != nil {
			return err
		}
		protoBytes, err := proto.Marshal(c.Value, wire.Schema, wire.Defs)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
		msgpackBytes, err := msgpack.Marshal(c.Value, wire.Schema, wire.Defs)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
		out.Cases = append(out.Cases, dumped{c.Name, c.Schema, encodedJSON, hex.EncodeToString(protoBytes), hex.EncodeToString(msgpackBytes)})
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

func callServer(args []string) error {
	flags := flag.NewFlagSet("call", flag.ExitOnError)
	url := flags.String("url", "", "server base URL")
	codecName := flags.String("codec", "json", "json, protobuf or msgpack")
	flags.Parse(args)

	var opts []lokerpc.ClientOption
	switch *codecName {
	case "protobuf":
		opts = append(opts, lokerpc.ClientCodecs(protocodec.New()))
	case "msgpack":
		opts = append(opts, lokerpc.ClientCodecs(msgpackcodec.New()))
	case "json":
	default:
		return fmt.Errorf("unknown codec %q", *codecName)
	}

	recorder := &Recorder{}
	opts = append(opts, lokerpc.WithHTTPClient(&http.Client{Transport: recorder}))
	ctx := context.Background()
	menu := MenuClient{lokerpc.NewClient(*url+"/rpc/menu", opts...)}
	edge := lokerpc.NewClient(*url+"/rpc/edge", opts...)

	type result struct {
		Call  string `json:"call"`
		Value any    `json:"value,omitempty"`
		Error string `json:"error,omitempty"`
	}
	var results []result

	want := seedMenu(5)
	saved, err := menu.SaveMenu(ctx, want)
	results = append(results, result{"saveMenu", saved.Revision, errString(err)})
	got, err := menu.GetMenu(ctx, GetMenuRequest{MenuID: want.ID})
	results = append(results, result{"getMenu", got, errString(err)})
	_, err = menu.GetMenu(ctx, GetMenuRequest{MenuID: "missing"})
	results = append(results, result{"getMenu/missing", nil, errString(err)})

	for _, c := range interopEdges() {
		var echoed fixtures.Edge
		err := edge.DoRequest(ctx, "echo", c.Value, &echoed)
		results = append(results, result{"echo/" + c.Name, echoed, errString(err)})
	}

	exchanges := recorder.Take()
	type exchange struct {
		Method       string `json:"method"`
		Path         string `json:"path"`
		Status       int    `json:"status"`
		RequestType  string `json:"requestType"`
		ResponseType string `json:"responseType"`
		IfMatch      string `json:"ifMatch"`
	}
	summary := make([]exchange, len(exchanges))
	for i, e := range exchanges {
		summary[i] = exchange{e.Method, e.Path, e.Status, e.RequestType, e.ResponseType, e.IfMatch}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"results": results, "exchanges": summary})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
