// Package codecutil holds the schema and reflection plumbing shared by the
// binary codecs: JTD ref resolution, deterministic field order, encoding/json
// style field lookup, and numeric range checks.
package codecutil

import (
	"cmp"
	"encoding"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	jtd "github.com/jsontypedef/json-typedef-go"
)

// Resolve follows refs to the underlying schema. nullable is true if the
// reference or the definition allows null.
func Resolve(schema *jtd.Schema, defs map[string]jtd.Schema) (resolved *jtd.Schema, nullable bool, err error) {
	resolved = schema
	nullable = schema.Nullable
	for resolved.Ref != nil {
		def, ok := defs[*resolved.Ref]
		if !ok {
			return nil, false, fmt.Errorf("undefined ref %q", *resolved.Ref)
		}
		resolved = &def
		nullable = nullable || def.Nullable
	}
	return resolved, nullable, nil
}

// Property is one member of a properties-form schema.
type Property struct {
	Name     string
	Schema   jtd.Schema
	Optional bool
}

// Properties returns required and optional properties sorted by name. Position
// in this slice is the field's identity on the wire.
func Properties(schema *jtd.Schema) []Property {
	props := make([]Property, 0, len(schema.Properties)+len(schema.OptionalProperties))
	for name, s := range schema.Properties {
		props = append(props, Property{Name: name, Schema: s})
	}
	for name, s := range schema.OptionalProperties {
		props = append(props, Property{Name: name, Schema: s, Optional: true})
	}
	slices.SortFunc(props, func(a, b Property) int { return cmp.Compare(a.Name, b.Name) })
	return props
}

// Field locates a JSON-named field inside a Go struct, possibly through
// embedded structs.
type Field struct {
	Name  string
	Index []int
	Type  reflect.Type

	tagged bool
}

// Get returns the field of v, or false if an embedded pointer on the way is nil.
func (f Field) Get(v reflect.Value) (reflect.Value, bool) {
	for _, i := range f.Index {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v, true
}

// Ensure returns the settable field of v, allocating embedded pointers.
func (f Field) Ensure(v reflect.Value) reflect.Value {
	for _, i := range f.Index {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}

var fieldCache sync.Map

// StructFields maps JSON names to Go fields using encoding/json's visibility
// rules: embedded promotion, shallower fields shadow deeper ones, a tagged
// field beats an untagged one at the same depth, and `json:"-"` is skipped.
// Ambiguous names are dropped.
func StructFields(t reflect.Type) map[string]Field {
	if cached, ok := fieldCache.Load(t); ok {
		return cached.(map[string]Field)
	}

	type level struct {
		typ   reflect.Type
		index []int
	}

	var candidates []Field
	visited := map[reflect.Type]bool{}
	next := []level{{typ: t}}

	for len(next) > 0 {
		current := next
		next = nil

		for _, l := range current {
			if visited[l.typ] {
				continue
			}
			visited[l.typ] = true

			for i := range l.typ.NumField() {
				sf := l.typ.Field(i)
				fieldType := sf.Type
				if fieldType.Kind() == reflect.Pointer {
					fieldType = fieldType.Elem()
				}

				if sf.Anonymous && !sf.IsExported() && fieldType.Kind() != reflect.Struct {
					continue
				}
				if !sf.Anonymous && !sf.IsExported() {
					continue
				}

				name, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
				if name == "-" {
					continue
				}

				index := append(slices.Clone(l.index), i)
				promote := sf.Anonymous && name == "" && fieldType.Kind() == reflect.Struct && fieldType != reflect.TypeFor[time.Time]()
				if promote {
					next = append(next, level{typ: fieldType, index: index})
					continue
				}
				if !sf.IsExported() {
					continue
				}

				candidates = append(candidates, Field{Name: cmp.Or(name, sf.Name), Index: index, Type: sf.Type, tagged: name != ""})
			}
		}
	}

	byName := map[string][]Field{}
	for _, f := range candidates {
		byName[f.Name] = append(byName[f.Name], f)
	}

	fields := make(map[string]Field, len(byName))
	for name, group := range byName {
		minDepth := slices.MinFunc(group, func(a, b Field) int { return cmp.Compare(len(a.Index), len(b.Index)) })
		shallowest := slices.DeleteFunc(group, func(f Field) bool { return len(f.Index) != len(minDepth.Index) })
		if len(shallowest) > 1 {
			shallowest = slices.DeleteFunc(shallowest, func(f Field) bool { return !f.tagged })
		}
		if len(shallowest) == 1 {
			fields[name] = shallowest[0]
		}
	}

	cached, _ := fieldCache.LoadOrStore(t, fields)
	return cached.(map[string]Field)
}

// IsTextType reports whether t is a non-string type that round-trips through
// encoding.TextMarshaler and TextUnmarshaler.
func IsTextType(t reflect.Type) bool {
	if t.Kind() == reflect.String {
		return false
	}
	ptr := reflect.PointerTo(t)
	return ptr.Implements(reflect.TypeFor[encoding.TextMarshaler]()) && ptr.Implements(reflect.TypeFor[encoding.TextUnmarshaler]())
}

// MarshalText calls MarshalText on v, which must be of an IsTextType type.
func MarshalText(v reflect.Value) ([]byte, error) {
	if !v.CanAddr() {
		addressable := reflect.New(v.Type()).Elem()
		addressable.Set(v)
		v = addressable
	}
	return v.Addr().Interface().(encoding.TextMarshaler).MarshalText()
}

// UnmarshalText calls UnmarshalText on addressable v, which must be of an IsTextType type.
func UnmarshalText(v reflect.Value, text []byte) error {
	return v.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText(text)
}

// IntForSchema reads a Go integer as an int64 and checks it against the range
// of the schema's integer type, so a Go int that no longer fits int32 fails.
func IntForSchema(v reflect.Value, schemaType jtd.Type) (int64, error) {
	var n int64
	switch {
	case v.CanInt():
		n = v.Int()
	case v.CanUint():
		if v.Uint() > math.MaxInt64 {
			return 0, fmt.Errorf("value %d out of range for %s", v.Uint(), schemaType)
		}
		n = int64(v.Uint())
	default:
		return 0, fmt.Errorf("cannot encode %s as %s", v.Type(), schemaType)
	}

	low, high, ok := intBounds(schemaType)
	if !ok {
		return 0, fmt.Errorf("%s is not an integer type", schemaType)
	}
	if n < low || n > high {
		return 0, fmt.Errorf("value %d out of range for %s", n, schemaType)
	}
	return n, nil
}

// SetInt stores n into a Go integer, failing if it overflows that type.
func SetInt(v reflect.Value, n int64) error {
	switch {
	case v.CanInt():
		if v.OverflowInt(n) {
			return fmt.Errorf("value %d overflows %s", n, v.Type())
		}
		v.SetInt(n)
	case v.CanUint():
		if n < 0 || v.OverflowUint(uint64(n)) {
			return fmt.Errorf("value %d overflows %s", n, v.Type())
		}
		v.SetUint(uint64(n))
	default:
		return fmt.Errorf("cannot decode integer into %s", v.Type())
	}
	return nil
}

// SetFloat stores f into a Go float, failing if it overflows that type.
func SetFloat(v reflect.Value, f float64) error {
	if !v.CanFloat() {
		return fmt.Errorf("cannot decode float into %s", v.Type())
	}
	if v.OverflowFloat(f) {
		return fmt.Errorf("value %g overflows %s", f, v.Type())
	}
	v.SetFloat(f)
	return nil
}

func intBounds(t jtd.Type) (low, high int64, ok bool) {
	switch t {
	case jtd.TypeInt8:
		return math.MinInt8, math.MaxInt8, true
	case jtd.TypeInt16:
		return math.MinInt16, math.MaxInt16, true
	case jtd.TypeInt32:
		return math.MinInt32, math.MaxInt32, true
	case jtd.TypeUint8:
		return 0, math.MaxUint8, true
	case jtd.TypeUint16:
		return 0, math.MaxUint16, true
	case jtd.TypeUint32:
		return 0, math.MaxUint32, true
	}
	return 0, 0, false
}
