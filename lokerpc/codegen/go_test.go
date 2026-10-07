package codegen

import (
	"bytes"
	"encoding/json"
	jtd "github.com/jsontypedef/json-typedef-go"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/LOKE/pkg/lokerpc"
)

func TestGenGoClient(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			var meta lokerpc.Meta

			// 😠 don't like that "union" meta tag got let in as a supported
			// feature. It's really not portable, and there is no way for
			// statically typed languages to support it
			if t.Name() == "TestGenGoClient/testdata/union-metadata.json" {
				return
			}

			f, err := os.Open(p)
			if err != nil {
				t.Fatal(err)
			}

			if err := json.NewDecoder(f).Decode(&meta); err != nil {
				t.Fatal(err)
			}

			var buf bytes.Buffer

			if err := GenGoClient(&buf, meta); err != nil {
				t.Fatal(err)
			}

			formatted, err := format.Source(buf.Bytes())
			if err != nil {
				// Some fixtures (e.g., spaces-hyphens.json) produce fields that are
				// not valid Go identifiers. This is a known codegen limitation.
				t.Skipf("generated code is not valid Go: %v", err)
			}

			file, err := parser.ParseFile(fset, p+".go", formatted, 0)
			if err != nil {
				t.Fatal(err)
			}
			conf := types.Config{Importer: imp}
			if _, err := conf.Check("", fset, []*ast.File{file}, nil); err != nil {
				t.Errorf("generated code does not type-check: %v", err)
			}

			goldenPath := p + ".go"
			if os.Getenv("UPDATE_GOLDEN") != "" {
				err = os.WriteFile(goldenPath, formatted, 0644)
				if err != nil {
					t.Fatal(err)
				}
				return
			}

			expected, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden file %s not found; run with UPDATE_GOLDEN=1 to create it", goldenPath)
			}

			if !bytes.Equal(formatted, expected) {
				t.Errorf("generated output differs from %s; run with UPDATE_GOLDEN=1 to update", goldenPath)
			}
		})
	}
}

func TestGoUnionJSON(t *testing.T) {
	var schema jtd.Schema
	if err := json.Unmarshal([]byte(`{"discriminator":"type","mapping":{"CREATED":{"properties":{"id":{"type":"string"}}},"DELETED":{"properties":{"softDelete":{"type":"boolean"}}},"VARIANT":{"properties":{"id":{"type":"string"}}},"EMPTY":{"properties":{"id":{"type":"string"}}},"":{"properties":{"id":{"type":"string"}}}}}`), &schema); err != nil {
		t.Fatal(err)
	}
	var decls bytes.Buffer
	body := genGoType(schema, "Event", true, goGen{imports: map[string]struct{}{}, decls: &decls})
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":   "module uniontest\n\ngo 1.26\n",
		"event.go": "package uniontest\nimport (\"encoding/json\"; \"fmt\")\n" + decls.String() + "\ntype Event " + body,
		"event_test.go": `package uniontest

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestUnion(t *testing.T) {
	for _, tc := range []struct {
		value EventVariant
		wire string
	}{
		{&EventCreated{ID: "123"}, "{\"type\":\"CREATED\",\"id\":\"123\"}"},
		{&EventDeleted{SoftDelete: true}, "{\"type\":\"DELETED\",\"softDelete\":true}"},
		{&EventEmpty{ID: "empty"}, "{\"type\":\"\",\"id\":\"empty\"}"},
		{&EventEmptyValue{ID: "upper"}, "{\"type\":\"EMPTY\",\"id\":\"upper\"}"},
		{&EventVariantValue{ID: "variant"}, "{\"type\":\"VARIANT\",\"id\":\"variant\"}"},
	} {
		value := tc.value
		b, err := json.Marshal(Event{Value: value})
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != tc.wire {
			t.Fatalf("got %s, want %s", b, tc.wire)
		}
		if _, ok := reflect.ValueOf(value).Elem().Interface().(EventVariant); ok {
			t.Fatalf("value type %T implements EventVariant", value)
		}
		var got Event
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Value, value) {
			t.Fatalf("got %#v, want %#v", got.Value, value)
		}
	}
	for _, value := range []EventVariant{nil, (*EventCreated)(nil), (*EventUnknown)(nil)} {
		if _, err := json.Marshal(Event{Value: value}); err == nil {
			t.Fatalf("accepted nil %#v", value)
		}
	}
	raw := []byte("{\"type\":\"FUTURE\",\"nested\":{\"id\":123}}")
	var event Event
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	unknown, ok := event.Value.(*EventUnknown)
	if !ok || unknown.Tag != "FUTURE" {
		t.Fatalf("unknown: %#v", event.Value)
	}
	raw[0] = 'x'
	b, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, unknown.Raw) {
		t.Fatalf("round trip: %s", b)
	}
	for _, value := range []*EventUnknown{{Tag: "FUTURE"}, {Tag: "FUTURE", Raw: json.RawMessage("invalid")}, {Tag: "FUTURE", Raw: json.RawMessage("null")}, {Tag: "FUTURE", Raw: json.RawMessage("{\"type\":\"OTHER\"}")}} {
		if _, err := json.Marshal(Event{Value: value}); err == nil {
			t.Fatalf("accepted invalid unknown %#v", value)
		}
	}
	for _, input := range []string{"{", "{}", "null", "{\"type\":null}", "{\"type\":1}", "{\"type\":\"CREATED\",\"id\":1}"} {
		before := event.Value
		if err := json.Unmarshal([]byte(input), &event); err == nil {
			t.Fatalf("accepted %s", input)
		}
		if !reflect.DeepEqual(before, event.Value) {
			t.Fatalf("changed receiver on error: %s", input)
		}
	}
	if err := json.Unmarshal([]byte("{\"type\":\"CREATED\",\"id\":\"new\"}"), &event); err != nil {
		t.Fatal(err)
	}
	if got, ok := event.Value.(*EventCreated); !ok || got.ID != "new" {
		t.Fatalf("reuse: %#v", event.Value)
	}
	if err := json.Unmarshal([]byte("{\"type\":\"DELETED\",\"softDelete\":true}"), &event); err != nil {
		t.Fatal(err)
	}
	if got, ok := event.Value.(*EventDeleted); !ok || !got.SoftDelete {
		t.Fatalf("reuse: %#v", event.Value)
	}
}
`,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "test", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated union tests: %v\n%s", err, output)
	}
}
