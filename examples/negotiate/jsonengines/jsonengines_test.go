package jsonengines_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/LOKE/pkg/examples/negotiate/jsonengines"
	"github.com/LOKE/pkg/lokerpc"
)

type v1 struct{}

func (v1) Marshal(v any) ([]byte, error)   { return json.Marshal(v) }
func (v1) Unmarshal(d []byte, v any) error { return json.Unmarshal(d, v) }

type sample struct {
	Name string         `json:"name"`
	Tags []string       `json:"tags"`
	Seen map[string]int `json:"seen"`
}

// TestSemantics pins where the engines differ on wire behaviour that matters
// to existing callers: encoding/json v1 is the reference.
func TestSemantics(t *testing.T) {
	engines := []struct {
		name   string
		engine lokerpc.JSON
	}{
		{"v1", v1{}},
		{"jsonv2", jsonengines.V2{}},
		{"gojson", jsonengines.GoJSON{}},
	}

	decode := func(engine lokerpc.JSON, body string) string {
		var got sample
		err := engine.Unmarshal([]byte(body), &got)
		if err != nil {
			return "error"
		}
		out, _ := json.Marshal(got)
		return string(out)
	}

	tests := []struct {
		name string
		run  func(engine lokerpc.JSON) string
		want [3]string
	}{
		{"nil slice and map encode", func(e lokerpc.JSON) string {
			out, _ := e.Marshal(sample{Name: "x"})
			return string(out)
		}, [3]string{`{"name":"x","tags":null,"seen":null}`, `{"name":"x","tags":[],"seen":{}}`, `{"name":"x","tags":null,"seen":null}`}},
		{"HTML characters encode", func(e lokerpc.JSON) string {
			out, _ := e.Marshal(sample{Name: "<a&b>"})
			return string(out)
		}, [3]string{`{"name":"\u003ca\u0026b\u003e","tags":null,"seen":null}`, `{"name":"<a&b>","tags":[],"seen":{}}`, `{"name":"\u003ca\u0026b\u003e","tags":null,"seen":null}`}},
		{"case-insensitive field match", func(e lokerpc.JSON) string {
			return decode(e, `{"NAME":"x"}`)
		}, [3]string{`{"name":"x","tags":null,"seen":null}`, `{"name":"","tags":null,"seen":null}`, `{"name":"x","tags":null,"seen":null}`}},
		{"duplicate member names", func(e lokerpc.JSON) string {
			return decode(e, `{"name":"a","name":"b"}`)
		}, [3]string{`{"name":"b","tags":null,"seen":null}`, "error", `{"name":"b","tags":null,"seen":null}`}},
		{"invalid UTF-8", func(e lokerpc.JSON) string {
			return decode(e, "{\"name\":\"\xff\"}")
		}, [3]string{"{\"name\":\"�\",\"tags\":null,\"seen\":null}", "error", "{\"name\":\"�\",\"tags\":null,\"seen\":null}"}},
		{"unknown members are ignored", func(e lokerpc.JSON) string {
			return decode(e, `{"name":"x","extra":1}`)
		}, [3]string{`{"name":"x","tags":null,"seen":null}`, `{"name":"x","tags":null,"seen":null}`, `{"name":"x","tags":null,"seen":null}`}},
		{"null clears containers", func(e lokerpc.JSON) string {
			return decode(e, `{"tags":null,"seen":null}`)
		}, [3]string{`{"name":"","tags":null,"seen":null}`, `{"name":"","tags":null,"seen":null}`, `{"name":"","tags":null,"seen":null}`}},
		{"wrong member type", func(e lokerpc.JSON) string {
			return decode(e, `{"name":1}`)
		}, [3]string{"error", "error", "error"}},
		{"trailing data", func(e lokerpc.JSON) string {
			return decode(e, `{} trailing`)
		}, [3]string{"error", "error", "error"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i, engine := range engines {
				if got := tt.run(engine.engine); got != tt.want[i] {
					t.Errorf("%s: got %s, want %s", engine.name, got, tt.want[i])
				}
			}
		})
	}
}

type mixed struct {
	Name   string            `json:"name"`
	Count  int               `json:"count"`
	Label  string            `json:"label"`
	Ratio  float64           `json:"ratio"`
	On     bool              `json:"on"`
	Tags   []string          `json:"tags"`
	Fields map[string]string `json:"fields"`
	Child  *mixed            `json:"child,omitempty"`
}

// TestOptimizedFieldOrder checks the option changes member order only.
func TestOptimizedFieldOrder(t *testing.T) {
	plain, ordered := jsonengines.GoJSON{}, jsonengines.GoJSON{OptimizeFieldOrder: true}

	tests := []struct {
		name  string
		value mixed
	}{
		{"zero value", mixed{}},
		{"interleaved kinds", mixed{Name: "a", Count: 2, Label: "b", Ratio: 1.5, On: true, Tags: []string{"x", "y"}, Fields: map[string]string{"k": "v"}}},
		{"nested", mixed{Name: "outer", Count: 1, Child: &mixed{Name: "inner", Label: "l", Count: -4}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plainBytes, err := plain.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			orderedBytes, err := ordered.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}

			var plainTree, orderedTree any
			err = json.Unmarshal(plainBytes, &plainTree)
			if err != nil {
				t.Fatal(err)
			}
			err = json.Unmarshal(orderedBytes, &orderedTree)
			if err != nil {
				t.Fatalf("ordered output is not JSON: %v\n%s", err, orderedBytes)
			}
			if !reflect.DeepEqual(plainTree, orderedTree) {
				t.Errorf("same members expected\nplain:   %s\nordered: %s", plainBytes, orderedBytes)
			}

			var decoded mixed
			err = ordered.Unmarshal(orderedBytes, &decoded)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tt.value, decoded) {
				t.Errorf("round trip changed value: %+v != %+v", tt.value, decoded)
			}
		})
	}

	t.Run("order differs for interleaved kinds", func(t *testing.T) {
		value := tests[1].value
		plainBytes, _ := plain.Marshal(value)
		orderedBytes, _ := ordered.Marshal(value)
		if bytes.Equal(plainBytes, orderedBytes) {
			t.Errorf("expected a different member order, both gave %s", plainBytes)
		}
	})
}
