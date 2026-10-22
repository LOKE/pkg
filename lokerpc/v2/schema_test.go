package lokerpc

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	jtd "github.com/jsontypedef/json-typedef-go"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func schemaTestField(name string, number int32, kind descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name: proto.String(name), Number: proto.Int32(number), Type: kind.Enum(),
		Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
	}
}

func schemaTestFile(t *testing.T, file *descriptorpb.FileDescriptorProto) protoreflect.FileDescriptor {
	t.Helper()
	descriptor, err := protodesc.NewFile(file, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func TestServiceMetaSchema(t *testing.T) {
	fields := []*descriptorpb.FieldDescriptorProto{
		schemaTestField("display_name", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
		schemaTestField("count", 2, descriptorpb.FieldDescriptorProto_TYPE_INT64),
		schemaTestField("labels", 3, descriptorpb.FieldDescriptorProto_TYPE_STRING),
		schemaTestField("attributes", 4, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
		schemaTestField("child", 5, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
		schemaTestField("state", 6, descriptorpb.FieldDescriptorProto_TYPE_ENUM),
		schemaTestField("enabled", 7, descriptorpb.FieldDescriptorProto_TYPE_BOOL),
		schemaTestField("choice_text", 8, descriptorpb.FieldDescriptorProto_TYPE_STRING),
		schemaTestField("when", 9, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
		schemaTestField("wrapped", 10, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
	}
	fields[0].JsonName = proto.String("customName")
	fields[2].Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	fields[3].Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	fields[3].TypeName = proto.String(".sample.Request.AttributesEntry")
	fields[4].TypeName = proto.String(".sample.Request")
	fields[5].TypeName = proto.String(".sample.State")
	fields[6].Proto3Optional = proto.Bool(true)
	fields[6].OneofIndex = proto.Int32(1)
	fields[7].OneofIndex = proto.Int32(0)
	fields[8].TypeName = proto.String(".google.protobuf.Timestamp")
	fields[9].TypeName = proto.String(".google.protobuf.StringValue")
	key := schemaTestField("key", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)
	value := schemaTestField("value", 2, descriptorpb.FieldDescriptorProto_TYPE_UINT32)
	descriptor := schemaTestFile(t, &descriptorpb.FileDescriptorProto{
		Name: proto.String("schema.proto"), Package: proto.String("sample"), Syntax: proto.String("proto3"),
		Dependency: []string{timestamppb.File_google_protobuf_timestamp_proto.Path(), wrapperspb.File_google_protobuf_wrappers_proto.Path(), emptypb.File_google_protobuf_empty_proto.Path()},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Request"), Field: fields,
			OneofDecl:  []*descriptorpb.OneofDescriptorProto{{Name: proto.String("choice")}, {Name: proto.String("_enabled")}},
			NestedType: []*descriptorpb.DescriptorProto{{Name: proto.String("AttributesEntry"), Field: []*descriptorpb.FieldDescriptorProto{key, value}, Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)}}},
		}},
		EnumType: []*descriptorpb.EnumDescriptorProto{{Name: proto.String("State"), Value: []*descriptorpb.EnumValueDescriptorProto{{Name: proto.String("UNKNOWN"), Number: proto.Int32(0)}, {Name: proto.String("READY"), Number: proto.Int32(1)}}}},
		Service: []*descriptorpb.ServiceDescriptorProto{{Name: proto.String("Example"), Method: []*descriptorpb.MethodDescriptorProto{
			{Name: proto.String("Zulu"), InputType: proto.String(".sample.Request"), OutputType: proto.String(".google.protobuf.Empty")},
			{Name: proto.String("Alpha"), InputType: proto.String(".sample.Request"), OutputType: proto.String(".sample.Request")},
			{Name: proto.String("Stream"), InputType: proto.String(".sample.Request"), OutputType: proto.String(".sample.Request"), ServerStreaming: proto.Bool(true)},
		}}},
		SourceCodeInfo: &descriptorpb.SourceCodeInfo{Location: []*descriptorpb.SourceCodeInfo_Location{
			{Path: []int32{6, 0}, Span: []int32{0, 0, 1}, LeadingComments: proto.String(" Example service documentation.\n")},
			{Path: []int32{6, 0, 2, 1}, Span: []int32{1, 0, 1}, LeadingComments: proto.String(" Alpha documentation.\n")},
		}},
	})
	meta, err := serviceMeta("example", descriptor.Services().Get(0))
	if err != nil {
		t.Fatal(err)
	}
	if meta.ServiceName != "example" || meta.MultiArg || meta.Help != "Example service documentation." {
		t.Fatalf("unexpected service metadata: %+v", meta)
	}
	if len(meta.Interfaces) != 2 || meta.Interfaces[0].MethodName != "alpha" || meta.Interfaces[1].MethodName != "zulu" {
		t.Fatalf("unary interfaces not sorted: %+v", meta.Interfaces)
	}
	alpha := meta.Interfaces[0]
	if alpha.Help != "Alpha documentation." || alpha.MethodTimeout != 60000 || meta.Interfaces[1].Help != "zulu method" {
		t.Fatalf("unexpected method metadata: %+v", alpha)
	}
	wantParams := []string{"customName", "count", "labels", "attributes", "child", "state", "enabled", "choiceText", "when", "wrapped"}
	if !reflect.DeepEqual(alpha.ParamNames, wantParams) {
		t.Fatalf("params = %v, want %v", alpha.ParamNames, wantParams)
	}
	if len(meta.Definitions) != 1 || alpha.RequestTypeDef.Ref == nil || *alpha.RequestTypeDef.Ref != "Request" {
		t.Fatalf("unexpected message references: %+v", meta.Definitions)
	}
	schema := meta.Definitions["Request"]
	if !schema.AdditionalProperties || len(schema.Properties) != 5 || len(schema.OptionalProperties) != 5 {
		t.Fatalf("unexpected field presence: %+v", schema)
	}
	wantProperties := map[string]jtd.Schema{
		"customName": {Type: jtd.TypeString}, "count": {Type: jtd.TypeString},
		"labels": {Elements: &jtd.Schema{Type: jtd.TypeString}}, "attributes": {Values: &jtd.Schema{Type: jtd.TypeUint32}},
		"state": {Enum: []string{"UNKNOWN", "READY"}},
	}
	if !reflect.DeepEqual(schema.Properties, wantProperties) {
		t.Fatalf("properties = %+v, want %+v", schema.Properties, wantProperties)
	}
	if schema.OptionalProperties["when"].Type != jtd.TypeTimestamp || !schema.OptionalProperties["wrapped"].Nullable || schema.OptionalProperties["child"].Ref == nil {
		t.Fatalf("unexpected message/WKT fields: %+v", schema.OptionalProperties)
	}
	if meta.Interfaces[1].ResponseTypeDef.Properties == nil || len(meta.Interfaces[1].ResponseTypeDef.Properties) != 0 {
		t.Fatal("Empty response must be an empty object")
	}
	set, err := descriptorSet(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(set.File))
	for i, file := range set.File {
		paths[i] = file.GetName()
	}
	wantPaths := []string{"google/protobuf/empty.proto", "google/protobuf/timestamp.proto", "google/protobuf/wrappers.proto", "schema.proto"}
	if !reflect.DeepEqual(paths, wantPaths) || !proto.Equal(set.File[3].SourceCodeInfo, protodesc.ToFileDescriptorProto(descriptor).SourceCodeInfo) {
		t.Fatalf("descriptor closure/source info not preserved: %v", paths)
	}
	again, err := descriptorSet(descriptor)
	if err != nil || !proto.Equal(again, set) {
		t.Fatalf("descriptor closure not deterministic: %v", err)
	}
}

func TestScalarAndWellKnownSchemas(t *testing.T) {
	for _, test := range []struct {
		kind descriptorpb.FieldDescriptorProto_Type
		want jtd.Type
	}{
		{descriptorpb.FieldDescriptorProto_TYPE_STRING, jtd.TypeString},
		{descriptorpb.FieldDescriptorProto_TYPE_BYTES, jtd.TypeString},
		{descriptorpb.FieldDescriptorProto_TYPE_BOOL, jtd.TypeBoolean},
		{descriptorpb.FieldDescriptorProto_TYPE_INT32, jtd.TypeInt32},
		{descriptorpb.FieldDescriptorProto_TYPE_SINT32, jtd.TypeInt32},
		{descriptorpb.FieldDescriptorProto_TYPE_SFIXED32, jtd.TypeInt32},
		{descriptorpb.FieldDescriptorProto_TYPE_UINT32, jtd.TypeUint32},
		{descriptorpb.FieldDescriptorProto_TYPE_FIXED32, jtd.TypeUint32},
		{descriptorpb.FieldDescriptorProto_TYPE_INT64, jtd.TypeString},
		{descriptorpb.FieldDescriptorProto_TYPE_SINT64, jtd.TypeString},
		{descriptorpb.FieldDescriptorProto_TYPE_SFIXED64, jtd.TypeString},
		{descriptorpb.FieldDescriptorProto_TYPE_UINT64, jtd.TypeString},
		{descriptorpb.FieldDescriptorProto_TYPE_FIXED64, jtd.TypeString},
		{descriptorpb.FieldDescriptorProto_TYPE_FLOAT, jtd.TypeFloat32},
		{descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, jtd.TypeFloat64},
	} {
		t.Run(test.kind.String(), func(t *testing.T) {
			file := schemaTestFile(t, &descriptorpb.FileDescriptorProto{
				Name: proto.String("scalar.proto"), Syntax: proto.String("proto3"),
				MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Message"), Field: []*descriptorpb.FieldDescriptorProto{schemaTestField("value", 1, test.kind)}}},
			})
			got := fieldSchema(file.Messages().Get(0).Fields().Get(0), nil)
			if got.Type != test.want {
				t.Fatalf("type = %v, want %v", got.Type, test.want)
			}
		})
	}
	for _, test := range []struct {
		name string
		want jtd.Schema
	}{
		{"Timestamp", jtd.Schema{Type: jtd.TypeTimestamp}},
		{"Duration", jtd.Schema{Type: jtd.TypeString}},
		{"FieldMask", jtd.Schema{Type: jtd.TypeString}},
		{"Struct", jtd.Schema{}}, {"Value", jtd.Schema{}}, {"ListValue", jtd.Schema{}}, {"Any", jtd.Schema{}},
		{"Empty", jtd.Schema{Properties: map[string]jtd.Schema{}}},
		{"StringValue", jtd.Schema{Type: jtd.TypeString, Nullable: true}},
		{"BytesValue", jtd.Schema{Type: jtd.TypeString, Nullable: true}},
		{"Int64Value", jtd.Schema{Type: jtd.TypeString, Nullable: true}},
		{"UInt64Value", jtd.Schema{Type: jtd.TypeString, Nullable: true}},
		{"Int32Value", jtd.Schema{Type: jtd.TypeInt32, Nullable: true}},
		{"UInt32Value", jtd.Schema{Type: jtd.TypeUint32, Nullable: true}},
		{"BoolValue", jtd.Schema{Type: jtd.TypeBoolean, Nullable: true}},
		{"FloatValue", jtd.Schema{Type: jtd.TypeFloat32, Nullable: true}},
		{"DoubleValue", jtd.Schema{Type: jtd.TypeFloat64, Nullable: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := wellKnownSchema(protoreflect.FullName("google.protobuf." + test.name))
			if !ok || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("schema = %+v, %v; want %+v", got, ok, test.want)
			}
		})
	}
}

func TestDefinitionNames(t *testing.T) {
	messages := make(map[protoreflect.FullName]protoreflect.MessageDescriptor)
	for _, pkg := range []string{"first.version", "second.version"} {
		file := schemaTestFile(t, &descriptorpb.FileDescriptorProto{
			Name: proto.String(pkg + ".proto"), Package: proto.String(pkg), Syntax: proto.String("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Shared")}},
		})
		message := file.Messages().Get(0)
		messages[message.FullName()] = message
	}
	names, err := definitionNames(messages)
	if err != nil {
		t.Fatal(err)
	}
	if names["first.version.Shared"] != "FirstVersionShared" || names["second.version.Shared"] != "SecondVersionShared" {
		t.Fatalf("unexpected collision names: %v", names)
	}
}

func TestDescriptorSetTransitiveClosure(t *testing.T) {
	files := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{
		{Name: proto.String("root.proto"), Syntax: proto.String("proto3"), Dependency: []string{"z.proto", "a.proto"}},
		{Name: proto.String("z.proto"), Syntax: proto.String("proto3"), Dependency: []string{"shared.proto"}},
		{Name: proto.String("a.proto"), Syntax: proto.String("proto3"), Dependency: []string{"shared.proto"}},
		{Name: proto.String("shared.proto"), Syntax: proto.String("proto3")},
	}}
	registry, err := protodesc.NewFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	root, err := registry.FindFileByPath("root.proto")
	if err != nil {
		t.Fatal(err)
	}
	set, err := descriptorSet(root)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(set.File))
	for i, file := range set.File {
		paths[i] = file.GetName()
	}
	if want := []string{"shared.proto", "a.proto", "z.proto", "root.proto"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestServiceMetaRejectsMethodNameCollision(t *testing.T) {
	file := schemaTestFile(t, &descriptorpb.FileDescriptorProto{
		Name: proto.String("collision.proto"), Syntax: proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Message")}},
		Service: []*descriptorpb.ServiceDescriptorProto{{Name: proto.String("Example"), Method: []*descriptorpb.MethodDescriptorProto{
			{Name: proto.String("Get"), InputType: proto.String(".Message"), OutputType: proto.String(".Message")},
			{Name: proto.String("get"), InputType: proto.String(".Message"), OutputType: proto.String(".Message")},
		}}},
	})
	if _, err := serviceMeta("example", file.Services().Get(0)); err == nil {
		t.Fatal("expected duplicate lowerCamel method name error")
	}
}

func TestServiceMetaEmptyService(t *testing.T) {
	file := schemaTestFile(t, &descriptorpb.FileDescriptorProto{
		Name: proto.String("empty.proto"), Syntax: proto.String("proto3"),
		Service: []*descriptorpb.ServiceDescriptorProto{{Name: proto.String("Example")}},
	})
	meta, err := serviceMeta("example", file.Services().Get(0))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Help != "example service" || meta.Interfaces == nil || len(meta.Interfaces) != 0 {
		t.Fatalf("unexpected empty service metadata: %+v", meta)
	}
}

func TestServiceMetaExplicitPresenceMatchesWire(t *testing.T) {
	for _, syntax := range []string{"proto2", "editions"} {
		t.Run(syntax, func(t *testing.T) {
			explicit := schemaTestField("explicit_value", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)
			repeated := schemaTestField("items", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING)
			repeated.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
			fields := []*descriptorpb.FieldDescriptorProto{explicit, repeated}
			file := &descriptorpb.FileDescriptorProto{
				Name: proto.String("presence.proto"), Syntax: proto.String(syntax),
				MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Message"), Field: fields}},
				Service: []*descriptorpb.ServiceDescriptorProto{{Name: proto.String("Example"), Method: []*descriptorpb.MethodDescriptorProto{
					{Name: proto.String("Call"), InputType: proto.String(".Message"), OutputType: proto.String(".Message")},
				}}},
			}
			if syntax == "editions" {
				file.Edition = descriptorpb.Edition_EDITION_2023.Enum()
				implicit := schemaTestField("implicit_value", 3, descriptorpb.FieldDescriptorProto_TYPE_STRING)
				implicit.Options = &descriptorpb.FieldOptions{Features: &descriptorpb.FeatureSet{
					FieldPresence: descriptorpb.FeatureSet_IMPLICIT.Enum(),
				}}
				file.MessageType[0].Field = append(fields, implicit)
			}
			descriptor := schemaTestFile(t, file)
			meta, err := serviceMeta("example", descriptor.Services().Get(0))
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(meta)
			if err != nil {
				t.Fatal(err)
			}
			var metadata struct {
				Definitions map[string]struct {
					Properties         map[string]json.RawMessage `json:"properties"`
					OptionalProperties map[string]json.RawMessage `json:"optionalProperties"`
				} `json:"definitions"`
			}
			if err := json.Unmarshal(data, &metadata); err != nil {
				t.Fatal(err)
			}
			schema := metadata.Definitions["Message"]
			if len(schema.OptionalProperties) != 1 || schema.OptionalProperties["explicitValue"] == nil {
				t.Fatalf("explicit presence metadata = %s", data)
			}
			message := dynamicpb.NewMessage(descriptor.Messages().Get(0))
			for _, populated := range []bool{false, true} {
				if populated {
					message.Set(message.Descriptor().Fields().ByName("explicit_value"), protoreflect.ValueOfString(""))
				}
				wire, err := (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(message)
				if err != nil {
					t.Fatal(err)
				}
				var output map[string]json.RawMessage
				if err := json.Unmarshal(wire, &output); err != nil {
					t.Fatal(err)
				}
				if _, present := output["explicitValue"]; present != populated {
					t.Fatalf("explicit default presence = %t, want %t: %s", present, populated, wire)
				}
				for name := range schema.Properties {
					if _, present := output[name]; !present {
						t.Fatalf("required schema field %q missing from wire: %s", name, wire)
					}
				}
				for name := range output {
					if schema.Properties[name] == nil && schema.OptionalProperties[name] == nil {
						t.Fatalf("wire field %q missing from schema: %s", name, data)
					}
				}
			}
		})
	}
}

func TestServiceMetaEnumAliasesMatchWire(t *testing.T) {
	field := schemaTestField("state", 1, descriptorpb.FieldDescriptorProto_TYPE_ENUM)
	field.TypeName = proto.String(".State")
	descriptor := schemaTestFile(t, &descriptorpb.FileDescriptorProto{
		Name: proto.String("alias.proto"), Syntax: proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("Message"), Field: []*descriptorpb.FieldDescriptorProto{field}}},
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: proto.String("State"), Options: &descriptorpb.EnumOptions{AllowAlias: proto.Bool(true)},
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: proto.String("UNKNOWN"), Number: proto.Int32(0)},
				{Name: proto.String("UNSPECIFIED"), Number: proto.Int32(0)},
				{Name: proto.String("READY"), Number: proto.Int32(1)},
				{Name: proto.String("AVAILABLE"), Number: proto.Int32(1)},
				{Name: proto.String("DONE"), Number: proto.Int32(2)},
			},
		}},
		Service: []*descriptorpb.ServiceDescriptorProto{{Name: proto.String("Example"), Method: []*descriptorpb.MethodDescriptorProto{
			{Name: proto.String("Call"), InputType: proto.String(".Message"), OutputType: proto.String(".Message")},
		}}},
	})
	meta, err := serviceMeta("example", descriptor.Services().Get(0))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Definitions map[string]struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
			OptionalProperties map[string]json.RawMessage `json:"optionalProperties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	schema := metadata.Definitions["Message"]
	if schema.OptionalProperties == nil || len(schema.OptionalProperties) != 0 {
		t.Fatalf("implicit-only message must carry empty optionalProperties: %s", data)
	}
	names := schema.Properties["state"].Enum
	if want := []string{"UNKNOWN", "READY", "DONE"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("enum names = %v, want %v", names, want)
	}
	for number, name := range names {
		message := dynamicpb.NewMessage(descriptor.Messages().Get(0))
		message.Set(message.Descriptor().Fields().Get(0), protoreflect.ValueOfEnum(protoreflect.EnumNumber(number)))
		wire, err := (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		var output struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(wire, &output); err != nil {
			t.Fatal(err)
		}
		if output.State != name {
			t.Fatalf("enum wire name = %q, want schema name %q", output.State, name)
		}
	}
}

func TestDescriptorSetUnresolvedImport(t *testing.T) {
	descriptor, err := (protodesc.FileOptions{AllowUnresolvable: true}).New(&descriptorpb.FileDescriptorProto{
		Name: proto.String("root.proto"), Syntax: proto.String("proto3"), Dependency: []string{"missing.proto"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := descriptorSet(descriptor); err == nil || !strings.Contains(err.Error(), "missing.proto") {
		t.Fatalf("unresolved import error = %v, want missing import path", err)
	}
}
