package lokerpc

import (
	"fmt"
	"reflect"
	"time"

	jtd "github.com/jsontypedef/json-typedef-go"
)

var timeType = reflect.TypeOf(time.Time{})

type NamedSchema struct {
	Name   string
	Schema jtd.Schema
}

func TypeSchema(t reflect.Type, tdefs map[reflect.Type]*NamedSchema) *jtd.Schema {
	if ns, ok := tdefs[t]; ok {
		return &jtd.Schema{Ref: &ns.Name}
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
				name = fmt.Sprintf("%s.%s", t.PkgPath(), name)
				tdefs[t] = &NamedSchema{Name: name, Schema: schema}
			}

			schema.Properties = make(map[string]jtd.Schema)

			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				name, omit := parseTag(f.Tag.Get("json"))
				if name == "" {
					name = f.Name
				}
				s := TypeSchema(f.Type, tdefs)
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

func TypeDefs(tdefs map[reflect.Type]*NamedSchema) map[string]jtd.Schema {
	defs := make(map[string]jtd.Schema)

	for t, ns := range tdefs {
		name := t.Name()
		n := 1

		for {
			if _, ok := defs[name]; !ok {
				break
			}
			n++
			name = fmt.Sprintf("%s%d", t.Name(), n)
		}

		ns.Name = name
		defs[ns.Name] = ns.Schema
	}

	return defs
}
