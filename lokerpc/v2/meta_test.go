package lokerpc

import (
	"encoding/json"
	"reflect"
	"testing"

	v1 "github.com/LOKE/pkg/lokerpc"
	jtd "github.com/jsontypedef/json-typedef-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestServiceMetaMarshalsEmptyObjectSchemas(t *testing.T) {
	file := schemaTestFile(t, &descriptorpb.FileDescriptorProto{
		Name: proto.String("empty-wire.proto"), Package: proto.String("wire"), Syntax: proto.String("proto3"),
		Dependency:  []string{emptypb.File_google_protobuf_empty_proto.Path()},
		MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("EmptyMessage")}},
		Service: []*descriptorpb.ServiceDescriptorProto{{Name: proto.String("Example"), Method: []*descriptorpb.MethodDescriptorProto{
			{Name: proto.String("Call"), InputType: proto.String(".wire.EmptyMessage"), OutputType: proto.String(".google.protobuf.Empty")},
		}}},
	})
	meta, err := serviceMeta("example", file.Services().Get(0))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Interfaces []struct {
			RequestTypeDef  json.RawMessage `json:"requestTypeDef"`
			ResponseTypeDef json.RawMessage `json:"responseTypeDef"`
		} `json:"interfaces"`
		Definitions  map[string]json.RawMessage `json:"definitions"`
		Proto        string                     `json:"proto"`
		ProtoService string                     `json:"protoService"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Interfaces) != 1 {
		t.Fatalf("interfaces = %s", data)
	}
	if got := string(wire.Interfaces[0].ResponseTypeDef); got != `{"properties":{}}` {
		t.Fatalf("Empty wire schema = %s", got)
	}
	if got := string(wire.Interfaces[0].RequestTypeDef); got != `{"ref":"EmptyMessage"}` {
		t.Fatalf("request wire schema = %s", got)
	}
	if got := string(wire.Definitions["EmptyMessage"]); got != `{"properties":{},"optionalProperties":{},"additionalProperties":true}` {
		t.Fatalf("empty message wire schema = %s", got)
	}
	if wire.Proto != meta.Proto || wire.ProtoService != "wire.Example" {
		t.Fatalf("lost proto metadata: %s", data)
	}
}

func TestMetaMarshalsNestedEmptyProperties(t *testing.T) {
	empty := jtd.Schema{Properties: map[string]jtd.Schema{}}
	for _, test := range []struct {
		name   string
		schema jtd.Schema
		want   string
	}{
		{"direct", empty, `{"properties":{}}`},
		{"emptyOptionalProperties", jtd.Schema{OptionalProperties: map[string]jtd.Schema{}}, `{"optionalProperties":{}}`},
		{"elements", jtd.Schema{Elements: &empty}, `{"elements":{"properties":{}}}`},
		{"values", jtd.Schema{Values: &empty}, `{"values":{"properties":{}}}`},
		{"properties", jtd.Schema{Properties: map[string]jtd.Schema{"item": empty}}, `{"properties":{"item":{"properties":{}}}}`},
		{"optionalProperties", jtd.Schema{OptionalProperties: map[string]jtd.Schema{"item": empty}}, `{"optionalProperties":{"item":{"properties":{}}}}`},
		{"mapping", jtd.Schema{Discriminator: "kind", Mapping: map[string]jtd.Schema{"item": empty}}, `{"discriminator":"kind","mapping":{"item":{"properties":{}}}}`},
		{"definitions", jtd.Schema{Definitions: map[string]jtd.Schema{"Item": empty}}, `{"definitions":{"Item":{"properties":{}}}}`},
		{"unconstrained", jtd.Schema{}, `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			meta := Meta{Meta: v1.Meta{
				ServiceName: "example", Help: "example service",
				Interfaces:  []v1.EndpointMeta{{MethodName: "call", RequestTypeDef: &test.schema, ResponseTypeDef: &test.schema}},
				Definitions: map[string]jtd.Schema{"Test": test.schema},
			}, Proto: "descriptor"}
			data, err := json.Marshal(meta)
			if err != nil {
				t.Fatal(err)
			}
			var wire struct {
				Interfaces []struct {
					RequestTypeDef  json.RawMessage `json:"requestTypeDef"`
					ResponseTypeDef json.RawMessage `json:"responseTypeDef"`
				} `json:"interfaces"`
				Definitions map[string]json.RawMessage `json:"definitions"`
			}
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatal(err)
			}
			for _, got := range []json.RawMessage{wire.Interfaces[0].RequestTypeDef, wire.Interfaces[0].ResponseTypeDef, wire.Definitions["Test"]} {
				var actual, expected any
				if err := json.Unmarshal(got, &actual); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(test.want), &expected); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(actual, expected) {
					t.Fatalf("wire schema = %s, want %s", got, test.want)
				}
			}
		})
	}
}

func TestMetaMarshalPreservesLegacyFields(t *testing.T) {
	meta := Meta{Meta: v1.Meta{
		ServiceName: "legacy", MultiArg: true, Help: "legacy help",
		Interfaces: []v1.EndpointMeta{{MethodName: "call", ParamNames: []string{"arg"}, MethodTimeout: 42, Help: "method help"}},
	}}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	var decoded v1.Meta
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, meta.Meta) {
		t.Fatalf("legacy fields changed: %+v, want %+v", decoded, meta.Meta)
	}
}
