package lokerpc

import (
	"reflect"
	"time"

	jtd "github.com/jsontypedef/json-typedef-go"
)

var timeType = reflect.TypeOf(time.Time{})

func TypeSchema(t reflect.Type, defs map[string]jtd.Schema) *jtd.Schema {
	var schema jtd.Schema

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

			schema.Properties = make(map[string]jtd.Schema)
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				name, omit := parseTag(f.Tag.Get("json"))
				if name == "" {
					name = f.Name
				}
				s := TypeSchema(f.Type, defs)
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

			if name := t.Name(); name != "" {
				if s, ok := defs[name]; ok {
					if reflect.DeepEqual(s, schema) {
						schema = jtd.Schema{Ref: &name}
					}
				} else {
					defs[name] = schema

					schema = jtd.Schema{Ref: &name}

				}
			}
		}
	case reflect.Pointer:
		schema = *TypeSchema(t.Elem(), defs)
		schema.Nullable = true
	case reflect.Slice:
		elems := TypeSchema(t.Elem(), defs)
		schema.Elements = elems
		schema.Nullable = true
	case reflect.Map:
		vals := TypeSchema(t.Elem(), defs)
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
