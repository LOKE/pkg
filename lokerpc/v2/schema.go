package lokerpc

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/LOKE/pkg/lokerpc"
	jtd "github.com/jsontypedef/json-typedef-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func serviceMeta(name string, descriptor protoreflect.ServiceDescriptor) (Meta, error) {
	messages := make(map[protoreflect.FullName]protoreflect.MessageDescriptor)
	var collect func(protoreflect.MessageDescriptor)
	collect = func(message protoreflect.MessageDescriptor) {
		if _, special := wellKnownSchema(message.FullName()); special {
			return
		}
		if _, exists := messages[message.FullName()]; exists {
			return
		}
		messages[message.FullName()] = message
		fields := message.Fields()
		for i := range fields.Len() {
			field := fields.Get(i)
			if field.IsMap() {
				field = field.MapValue()
			}
			if field.Message() != nil {
				collect(field.Message())
			}
		}
	}
	methods := descriptor.Methods()
	for i := range methods.Len() {
		method := methods.Get(i)
		if !method.IsStreamingClient() && !method.IsStreamingServer() {
			collect(method.Input())
			collect(method.Output())
		}
	}
	names, err := definitionNames(messages)
	if err != nil {
		return Meta{}, err
	}
	definitions := make(map[string]jtd.Schema, len(messages))
	for fullName, message := range messages {
		schema := jtd.Schema{
			Properties:           make(map[string]jtd.Schema),
			OptionalProperties:   make(map[string]jtd.Schema),
			AdditionalProperties: true,
		}
		fields := message.Fields()
		for i := range fields.Len() {
			field := fields.Get(i)
			value := fieldSchema(field, names)
			if field.HasPresence() {
				schema.OptionalProperties[field.JSONName()] = value
			} else {
				schema.Properties[field.JSONName()] = value
			}
		}
		definitions[names[fullName]] = schema
	}
	meta := Meta{
		Meta: lokerpc.Meta{
			ServiceName: name,
			Help:        leadingComment(descriptor, name+" service"),
			Interfaces:  make([]lokerpc.EndpointMeta, 0, methods.Len()),
			Definitions: definitions,
		},
		ProtoService: string(descriptor.FullName()),
	}
	methodNames := make(map[string]bool, methods.Len())
	for i := range methods.Len() {
		method := methods.Get(i)
		if method.IsStreamingClient() || method.IsStreamingServer() {
			continue
		}
		methodName := protoMethodName(string(method.Name()))
		if methodNames[methodName] {
			return Meta{}, fmt.Errorf("duplicate legacy method name %q in %s", methodName, descriptor.FullName())
		}
		methodNames[methodName] = true
		fields := method.Input().Fields()
		params := make([]string, fields.Len())
		for j := range fields.Len() {
			params[j] = fields.Get(j).JSONName()
		}
		request, response := messageSchema(method.Input(), names), messageSchema(method.Output(), names)
		meta.Interfaces = append(meta.Interfaces, lokerpc.EndpointMeta{
			MethodName:      methodName,
			ParamNames:      params,
			MethodTimeout:   60000,
			Help:            leadingComment(method, methodName+" method"),
			RequestTypeDef:  &request,
			ResponseTypeDef: &response,
		})
	}
	sort.Slice(meta.Interfaces, func(i, j int) bool { return meta.Interfaces[i].MethodName < meta.Interfaces[j].MethodName })
	files, err := descriptorSet(descriptor.ParentFile())
	if err != nil {
		return Meta{}, err
	}
	data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(files)
	if err != nil {
		return Meta{}, fmt.Errorf("encode service descriptors: %w", err)
	}
	meta.Proto = base64.StdEncoding.EncodeToString(data)
	return meta, nil
}

func protoMethodName(name string) string {
	first, size := utf8.DecodeRuneInString(name)
	return string(unicode.ToLower(first)) + name[size:]
}

func leadingComment(descriptor protoreflect.Descriptor, fallback string) string {
	comment := strings.TrimSpace(descriptor.ParentFile().SourceLocations().ByDescriptor(descriptor).LeadingComments)
	if comment == "" {
		return fallback
	}
	return comment
}

func definitionNames(messages map[protoreflect.FullName]protoreflect.MessageDescriptor) (map[protoreflect.FullName]string, error) {
	counts := make(map[protoreflect.Name]int, len(messages))
	fullNames := make([]protoreflect.FullName, 0, len(messages))
	for fullName, message := range messages {
		counts[message.Name()]++
		fullNames = append(fullNames, fullName)
	}
	sort.Slice(fullNames, func(i, j int) bool { return fullNames[i] < fullNames[j] })
	names := make(map[protoreflect.FullName]string, len(messages))
	used := make(map[string]protoreflect.FullName, len(messages))
	for _, fullName := range fullNames {
		name := string(messages[fullName].Name())
		if counts[messages[fullName].Name()] > 1 {
			var flattened strings.Builder
			for _, part := range strings.FieldsFunc(string(fullName), func(r rune) bool { return r == '.' || r == '_' }) {
				first, size := utf8.DecodeRuneInString(part)
				flattened.WriteRune(unicode.ToUpper(first))
				flattened.WriteString(part[size:])
			}
			name = flattened.String()
		}
		if previous, exists := used[name]; exists {
			return nil, fmt.Errorf("definition name %q collides for %s and %s", name, previous, fullName)
		}
		used[name] = fullName
		names[fullName] = name
	}
	return names, nil
}

func fieldSchema(field protoreflect.FieldDescriptor, names map[protoreflect.FullName]string) jtd.Schema {
	if field.IsMap() {
		value := scalarSchema(field.MapValue(), names)
		return jtd.Schema{Values: &value}
	}
	value := scalarSchema(field, names)
	if field.IsList() {
		return jtd.Schema{Elements: &value}
	}
	return value
}

func scalarSchema(field protoreflect.FieldDescriptor, names map[protoreflect.FullName]string) jtd.Schema {
	switch field.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return messageSchema(field.Message(), names)
	case protoreflect.BoolKind:
		return jtd.Schema{Type: jtd.TypeBoolean}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return jtd.Schema{Type: jtd.TypeInt32}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return jtd.Schema{Type: jtd.TypeUint32}
	case protoreflect.FloatKind:
		return jtd.Schema{Type: jtd.TypeFloat32}
	case protoreflect.DoubleKind:
		return jtd.Schema{Type: jtd.TypeFloat64}
	case protoreflect.EnumKind:
		values := field.Enum().Values()
		names := make([]string, 0, values.Len())
		for i := range values.Len() {
			value := values.Get(i)
			if values.ByNumber(value.Number()).Name() == value.Name() {
				names = append(names, string(value.Name()))
			}
		}
		return jtd.Schema{Enum: names}
	default:
		return jtd.Schema{Type: jtd.TypeString}
	}
}

func messageSchema(message protoreflect.MessageDescriptor, names map[protoreflect.FullName]string) jtd.Schema {
	if schema, special := wellKnownSchema(message.FullName()); special {
		return schema
	}
	name := names[message.FullName()]
	return jtd.Schema{Ref: &name}
}

func wellKnownSchema(name protoreflect.FullName) (jtd.Schema, bool) {
	switch name {
	case "google.protobuf.Timestamp":
		return jtd.Schema{Type: jtd.TypeTimestamp}, true
	case "google.protobuf.Duration", "google.protobuf.FieldMask":
		return jtd.Schema{Type: jtd.TypeString}, true
	case "google.protobuf.Struct", "google.protobuf.Value", "google.protobuf.ListValue", "google.protobuf.Any":
		return jtd.Schema{}, true
	case "google.protobuf.Empty":
		return jtd.Schema{Properties: map[string]jtd.Schema{}}, true
	case "google.protobuf.StringValue", "google.protobuf.BytesValue", "google.protobuf.Int64Value", "google.protobuf.UInt64Value":
		return jtd.Schema{Type: jtd.TypeString, Nullable: true}, true
	case "google.protobuf.BoolValue":
		return jtd.Schema{Type: jtd.TypeBoolean, Nullable: true}, true
	case "google.protobuf.Int32Value":
		return jtd.Schema{Type: jtd.TypeInt32, Nullable: true}, true
	case "google.protobuf.UInt32Value":
		return jtd.Schema{Type: jtd.TypeUint32, Nullable: true}, true
	case "google.protobuf.FloatValue":
		return jtd.Schema{Type: jtd.TypeFloat32, Nullable: true}, true
	case "google.protobuf.DoubleValue":
		return jtd.Schema{Type: jtd.TypeFloat64, Nullable: true}, true
	default:
		return jtd.Schema{}, false
	}
}

func descriptorSet(root protoreflect.FileDescriptor) (*descriptorpb.FileDescriptorSet, error) {
	set := &descriptorpb.FileDescriptorSet{}
	seen := make(map[string]bool)
	var visit func(protoreflect.FileDescriptor) error
	visit = func(file protoreflect.FileDescriptor) error {
		if file.IsPlaceholder() {
			return fmt.Errorf("unresolved protobuf import %q", file.Path())
		}
		if seen[file.Path()] {
			return nil
		}
		seen[file.Path()] = true
		imports := file.Imports()
		dependencies := make([]protoreflect.FileDescriptor, imports.Len())
		for i := range imports.Len() {
			dependencies[i] = imports.Get(i).FileDescriptor
		}
		sort.Slice(dependencies, func(i, j int) bool { return dependencies[i].Path() < dependencies[j].Path() })
		for _, dependency := range dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		set.File = append(set.File, protodesc.ToFileDescriptorProto(file))
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	return set, nil
}
