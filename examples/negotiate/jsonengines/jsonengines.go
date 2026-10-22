// Package jsonengines adapts alternative JSON libraries to lokerpc.JSON. The
// wire format stays JSON; only the engine that reads and writes bodies changes.
package jsonengines

import (
	jsonv2 "encoding/json/v2"

	gojson "github.com/goccy/go-json"
)

// V2 is the standard library's encoding/json/v2 with default options.
type V2 struct{}

func (V2) Marshal(v any) ([]byte, error) { return jsonv2.Marshal(v) }

func (V2) Unmarshal(data []byte, v any) error { return jsonv2.Unmarshal(data, v) }

// GoJSON is github.com/goccy/go-json, which follows encoding/json v1 semantics.
type GoJSON struct {
	// OptimizeFieldOrder lets the encoder group struct fields by kind for speed.
	// The members are the same but their order differs from the struct's.
	OptimizeFieldOrder bool
}

func (g GoJSON) Marshal(v any) ([]byte, error) {
	if g.OptimizeFieldOrder {
		return gojson.MarshalWithOption(v, gojson.OptimizeFieldOrder())
	}
	return gojson.Marshal(v)
}

func (GoJSON) Unmarshal(data []byte, v any) error { return gojson.Unmarshal(data, v) }
