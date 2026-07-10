package lokerpc

import (
	"fmt"
	"reflect"
	"sort"
	"time"

	jtd "github.com/jsontypedef/json-typedef-go"
)

var timeType = reflect.TypeOf(time.Time{})

type NamedSchema struct {
	Name    string
	SortKey string
	Schema  jtd.Schema
}

// addStructFields writes t's fields into schema, promoting the fields of
// untagged embedded structs the way encoding/json does.
func addStructFields(t reflect.Type, schema *jtd.Schema, tdefs map[reflect.Type]*NamedSchema) {
	addStructFieldsSeen(t, schema, tdefs, map[reflect.Type]bool{t: true})
}

func addStructFieldsSeen(t reflect.Type, schema *jtd.Schema, tdefs map[reflect.Type]*NamedSchema, seen map[reflect.Type]bool) {
	var embedded []reflect.Type
	typeMetadata, hasTypeMetadata := metadataForType(t)

	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}

		name, omit := parseTag(f.Tag.Get("json"))
		if name == "-" {
			continue
		}

		if f.Anonymous && name == "" {
			if ft := indirect(f.Type); ft.Kind() == reflect.Struct && ft != timeType {
				embedded = append(embedded, ft)
				continue
			}
		}

		if name == "" {
			name = f.Name
		}
		s := TypeSchema(f.Type, tdefs)
		if hasTypeMetadata {
			setDescription(s, typeMetadata.Fields[f.Name])
		}
		if omit {
			if schema.OptionalProperties == nil {
				schema.OptionalProperties = make(map[string]jtd.Schema)
			}

			s.Nullable = false // maybe shouldn't be necessary
			schema.OptionalProperties[name] = *s
		} else {
			schema.Properties[name] = *s
		}
	}

	// Promoted fields are merged last so an outer field always shadows an
	// embedded one of the same name, whatever the declaration order.
	for _, ft := range embedded {
		// Recursive embedding (Node embeds *Node) can't be promoted;
		// encoding/json drops the deeper copies too.
		if seen[ft] {
			continue
		}
		seen[ft] = true

		sub := jtd.Schema{Properties: make(map[string]jtd.Schema)}
		addStructFieldsSeen(ft, &sub, tdefs, seen)
		delete(seen, ft)

		for name, s := range sub.Properties {
			if !hasProperty(schema, name) {
				schema.Properties[name] = s
			}
		}
		for name, s := range sub.OptionalProperties {
			if hasProperty(schema, name) {
				continue
			}
			if schema.OptionalProperties == nil {
				schema.OptionalProperties = make(map[string]jtd.Schema)
			}
			schema.OptionalProperties[name] = s
		}
	}
}

func hasProperty(schema *jtd.Schema, name string) bool {
	if _, ok := schema.Properties[name]; ok {
		return true
	}
	_, ok := schema.OptionalProperties[name]
	return ok
}

func indirect(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		return t.Elem()
	}
	return t
}

func TypeSchema(t reflect.Type, tdefs map[reflect.Type]*NamedSchema) *jtd.Schema {
	if ns, ok := tdefs[t]; ok {
		return &jtd.Schema{Ref: &ns.Name}
	}

	if t.Name() != "" && t != timeType && t.Kind() != reflect.Struct {
		if metadata, ok := metadataForType(t); ok && (metadata.Description != "" || len(metadata.Enum) > 0) {
			return namedSourceSchema(t, metadata, tdefs)
		}
	}

	schema := jtd.Schema{}

	switch t.Kind() {
	case reflect.Struct:
		switch t {
		case timeType:
			schema.Type = jtd.TypeTimestamp
		default:
			if _, ok := t.MethodByName("MarshalJSON"); ok {
				// Do nothing, empty schema, any
				break
			}

			if _, ok := t.MethodByName("MarshalText"); ok {
				schema.Type = jtd.TypeString
				break
			}

			name := t.Name()

			if name != "" {
				tdefs[t] = &NamedSchema{
					Name:    name,
					SortKey: fmt.Sprintf("%s.%s", t.PkgPath(), name),
					Schema:  schema,
				}
			}

			schema.Properties = make(map[string]jtd.Schema)

			addStructFields(t, &schema, tdefs)
			if metadata, ok := metadataForType(t); ok {
				setDescription(&schema, metadata.Description)
			}

			if nt, ok := tdefs[t]; ok {
				nt.Schema = schema
				return &jtd.Schema{Ref: &nt.Name}
			}
		}
	case reflect.Pointer:
		schema = *TypeSchema(t.Elem(), tdefs)
		schema.Nullable = true
	case reflect.Slice:
		elems := TypeSchema(t.Elem(), tdefs)
		schema.Elements = elems
		schema.Nullable = true
	case reflect.Map:
		vals := TypeSchema(t.Elem(), tdefs)
		schema.Values = vals
		schema.Nullable = true
	case reflect.String:
		schema.Type = jtd.TypeString
	case reflect.Int:
		schema.Type = jtd.TypeInt32
	case reflect.Int8:
		schema.Type = jtd.TypeInt8
	case reflect.Int16:
		schema.Type = jtd.TypeInt16
	case reflect.Int32:
		schema.Type = jtd.TypeInt32
	case reflect.Int64:
		panic("int64 not supported")
	case reflect.Uint:
		schema.Type = jtd.TypeUint32
	case reflect.Uint8:
		schema.Type = jtd.TypeUint8
	case reflect.Uint16:
		schema.Type = jtd.TypeUint16
	case reflect.Uint32:
		schema.Type = jtd.TypeUint32
	case reflect.Uint64:
		panic("uint64 not supported")
	case reflect.Float32:
		schema.Type = jtd.TypeFloat32
	case reflect.Float64:
		schema.Type = jtd.TypeFloat64
	case reflect.Bool:
		schema.Type = jtd.TypeBoolean
	case reflect.Interface:
		// Do nothing, empty schema
	default:
		panic("Unknown type: " + t.String())
	}

	return &schema
}

func namedSourceSchema(t reflect.Type, metadata TypeMetadata, tdefs map[reflect.Type]*NamedSchema) *jtd.Schema {
	named := &NamedSchema{
		Name:    t.Name(),
		SortKey: fmt.Sprintf("%s.%s", t.PkgPath(), t.Name()),
	}
	tdefs[t] = named

	var schema jtd.Schema
	if len(metadata.Enum) > 0 {
		schema.Enum = make([]string, 0, len(metadata.Enum))
		names := make([]string, 0, len(metadata.Enum))
		for _, enum := range metadata.Enum {
			schema.Enum = append(schema.Enum, enum.Value)
			names = append(names, enum.Name)
		}
		schema.Metadata = map[string]any{
			enumNamesMetadataKey: names,
			enumTypeMetadataKey:  t.Name(),
		}
	} else {
		schema = *unnamedTypeSchema(t, tdefs)
	}
	setDescription(&schema, metadata.Description)
	named.Schema = schema

	return &jtd.Schema{Ref: &named.Name}
}

func unnamedTypeSchema(t reflect.Type, tdefs map[reflect.Type]*NamedSchema) *jtd.Schema {
	schema := jtd.Schema{}
	switch t.Kind() {
	case reflect.String:
		schema.Type = jtd.TypeString
	case reflect.Int:
		schema.Type = jtd.TypeInt32
	case reflect.Int8:
		schema.Type = jtd.TypeInt8
	case reflect.Int16:
		schema.Type = jtd.TypeInt16
	case reflect.Int32:
		schema.Type = jtd.TypeInt32
	case reflect.Uint:
		schema.Type = jtd.TypeUint32
	case reflect.Uint8:
		schema.Type = jtd.TypeUint8
	case reflect.Uint16:
		schema.Type = jtd.TypeUint16
	case reflect.Uint32:
		schema.Type = jtd.TypeUint32
	case reflect.Float32:
		schema.Type = jtd.TypeFloat32
	case reflect.Float64:
		schema.Type = jtd.TypeFloat64
	case reflect.Bool:
		schema.Type = jtd.TypeBoolean
	case reflect.Slice:
		schema.Elements = TypeSchema(t.Elem(), tdefs)
		schema.Nullable = true
	case reflect.Map:
		schema.Values = TypeSchema(t.Elem(), tdefs)
		schema.Nullable = true
	default:
		panic("lokerpc: unsupported documented named type " + t.String())
	}
	return &schema
}

func setDescription(schema *jtd.Schema, description string) {
	if description == "" {
		return
	}
	if schema.Metadata == nil {
		schema.Metadata = make(map[string]any)
	}
	schema.Metadata[descriptionMetadataKey] = description
}

func TypeDefs(tdefs map[reflect.Type]*NamedSchema) map[string]jtd.Schema {
	defs := make(map[string]jtd.Schema)

	sorted := make([]*NamedSchema, 0, len(tdefs))
	for _, ns := range tdefs {
		sorted = append(sorted, ns)
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].SortKey < sorted[j].SortKey
	})

	for _, ns := range sorted {
		name := ns.Name
		n := 1

		for {
			if _, ok := defs[name]; !ok {
				break
			}
			n++
			name = fmt.Sprintf("%s%d", ns.Name, n)
		}

		ns.Name = name
		defs[ns.Name] = ns.Schema
	}

	return defs
}
