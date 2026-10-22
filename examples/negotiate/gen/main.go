// Command gen writes the generated protobuf code for the fixture packages.
// The server side is generated from the Go types and lokerpc.TypeSchema; the
// client side from the committed JTD meta file, like a generated client.
package main

//go:generate go run .

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"

	"github.com/LOKE/pkg/examples/negotiate/fixtures"
	"github.com/LOKE/pkg/examples/negotiate/fixtures/menuclient"
	"github.com/LOKE/pkg/examples/negotiate/protogen"
	"github.com/LOKE/pkg/lokerpc"
)

const command = "go generate (examples/negotiate/gen)"

func main() {
	files, err := render(".")
	if err != nil {
		log.Fatal(err)
	}
	for name, content := range files {
		err := os.WriteFile(name, content, 0o644)
		if err != nil {
			log.Fatal(err)
		}
	}
}

// render returns the generated files keyed by path, relative to dir (the gen directory).
func render(dir string) (map[string][]byte, error) {
	typeDefs := map[reflect.Type]*lokerpc.NamedSchema{}
	menuSchema := lokerpc.TypeSchema(reflect.TypeFor[fixtures.BenchMenu](), typeDefs)
	optionSchema := lokerpc.TypeSchema(reflect.TypeFor[fixtures.BenchOption](), typeDefs)
	edgeSchema := lokerpc.TypeSchema(reflect.TypeFor[fixtures.Edge](), typeDefs)
	emptySchema := lokerpc.TypeSchema(reflect.TypeFor[struct{}](), typeDefs)
	defs := lokerpc.TypeDefs(typeDefs)

	server, err := protogen.Generate(protogen.Config{
		PkgName: "fixtures",
		PkgPath: "github.com/LOKE/pkg/examples/negotiate/fixtures",
		Command: command,
		Defs:    defs,
		Roots: []protogen.Root{
			{Type: reflect.TypeFor[fixtures.BenchMenu](), Schema: *menuSchema},
			{Type: reflect.TypeFor[fixtures.BenchOption](), Schema: *optionSchema},
			{Type: reflect.TypeFor[fixtures.Edge](), Schema: *edgeSchema},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("server side: %w", err)
	}

	meta := lokerpc.Meta{
		ServiceName: "menu",
		Definitions: defs,
		Interfaces: []lokerpc.EndpointMeta{
			{MethodName: "decode", RequestTypeDef: menuSchema, ResponseTypeDef: emptySchema},
			{MethodName: "encode", RequestTypeDef: emptySchema, ResponseTypeDef: menuSchema},
			{MethodName: "small", RequestTypeDef: optionSchema, ResponseTypeDef: optionSchema},
		},
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	metaJSON = append(metaJSON, '\n')

	var served lokerpc.Meta
	err = json.Unmarshal(metaJSON, &served)
	if err != nil {
		return nil, err
	}
	endpoints := map[string]lokerpc.EndpointMeta{}
	for _, endpoint := range served.Interfaces {
		endpoints[endpoint.MethodName] = endpoint
	}

	client, err := protogen.Generate(protogen.Config{
		PkgName: "menuclient",
		PkgPath: "github.com/LOKE/pkg/examples/negotiate/fixtures/menuclient",
		Command: command,
		Defs:    served.Definitions,
		Roots: []protogen.Root{
			{Type: reflect.TypeFor[menuclient.Menu](), Schema: *endpoints["decode"].RequestTypeDef},
			{Type: reflect.TypeFor[menuclient.Option](), Schema: *endpoints["small"].RequestTypeDef},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("client side: %w", err)
	}

	return map[string][]byte{
		filepath.Join(dir, "../fixtures/proto_gen.go"):              server,
		filepath.Join(dir, "../fixtures/menuclient/menu.meta.json"): metaJSON,
		filepath.Join(dir, "../fixtures/menuclient/proto_gen.go"):   client,
	}, nil
}
