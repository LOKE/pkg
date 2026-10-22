// Package protocodec encodes Go values straight to protobuf wire bytes,
// driven by the JTD schema. Field numbers are the 1-based position of each
// property in name order, and every message is wrapped in an envelope whose
// field 1 holds the value.
//
// Lossy spots: nil and empty slices or maps are indistinguishable on the wire,
// timestamps are decoded in UTC, and Go values must be representable by the
// schema (e.g. a Go int outside int32 is an error).
package protocodec

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/LOKE/pkg/examples/negotiate/codecutil"
	jtd "github.com/jsontypedef/json-typedef-go"
	"google.golang.org/protobuf/encoding/protowire"
)

const skipField = -1

// Kind is the wire shape a Node encodes as.
type Kind uint8

const (
	KindBool Kind = iota + 1
	KindInt
	KindUint
	KindFloat32
	KindFloat64
	KindString
	KindText
	KindTimestamp
	KindStruct
	KindList
	KindMap
	KindJSON
)

// Node is the compiled plan for one (schema, Go type) pair. The reflection
// codec executes it and the protogen generator reads it, so both share the
// same mapping rules.
type Node struct {
	Kind       Kind
	GoType     reflect.Type
	SchemaType jtd.Type
	Fields     []FieldPlan
	Elem       *Node
	Key        *Node
	// WrapElem puts each element in a {1: value} message.
	WrapElem bool
	// Packed lists write all elements in one length-delimited field.
	Packed bool
}

// FieldPlan maps one schema property to a Go field. Field is nil when the Go
// struct has no field of that name; the property is then skipped.
type FieldPlan struct {
	Name  string
	Num   protowire.Number
	Field *codecutil.Field
	Node  *Node
}

type planKey struct {
	schema *jtd.Schema
	typ    reflect.Type
}

type refKey struct {
	ref string
	typ reflect.Type
}

// Codec implements lokerpc.Codec for protobuf.
type Codec struct {
	plans        sync.Map
	fingerprints sync.Map
}

type fingerprintResult struct {
	fingerprint string
	err         error
}

// generatedMatches reports whether code generated with the given fingerprint
// encodes this schema. Any doubt falls back to the reflection path.
func (c *Codec) generatedMatches(generated string, schema *jtd.Schema, defs map[string]jtd.Schema) bool {
	cached, ok := c.fingerprints.Load(schema)
	if !ok {
		fingerprint, err := Fingerprint(schema, defs)
		cached, _ = c.fingerprints.LoadOrStore(schema, fingerprintResult{fingerprint, err})
	}
	result := cached.(fingerprintResult)
	return result.err == nil && result.fingerprint == generated
}

func New() *Codec {
	return &Codec{}
}

func (*Codec) ContentType() string {
	return "application/x-protobuf"
}

func (c *Codec) plan(schema *jtd.Schema, typ reflect.Type, defs map[string]jtd.Schema) (*Node, error) {
	key := planKey{schema, typ}
	if cached, ok := c.plans.Load(key); ok {
		return cached.(*Node), nil
	}

	compiler := compiler{defs: defs, refs: map[refKey]*Node{}}
	plan, err := compiler.compile(schema, typ)
	if err != nil {
		return nil, err
	}

	cached, _ := c.plans.LoadOrStore(key, plan)
	return cached.(*Node), nil
}

func (c *Codec) Marshal(v any, schema *jtd.Schema, defs map[string]jtd.Schema) ([]byte, error) {
	if v == nil {
		return nil, nil
	}

	value := reflect.ValueOf(v)
	generated, ok := v.(Marshaler)
	nilPointer := value.Kind() == reflect.Pointer && value.IsNil()
	if ok && !nilPointer && c.generatedMatches(generated.ProtoFingerprint(), schema, defs) {
		return generated.MarshalProto()
	}

	plan, err := c.plan(schema, value.Type(), defs)
	if err != nil {
		return nil, err
	}
	return plan.appendField(nil, 1, value)
}

func (c *Codec) Unmarshal(data []byte, v any, schema *jtd.Schema, defs map[string]jtd.Schema) error {
	target := reflect.ValueOf(v)
	if target.Kind() != reflect.Pointer || target.IsNil() {
		return fmt.Errorf("protocodec: unmarshal target must be a non-nil pointer, got %T", v)
	}

	generated, ok := v.(Unmarshaler)
	if ok && c.generatedMatches(generated.ProtoFingerprint(), schema, defs) {
		return generated.UnmarshalProto(data)
	}

	plan, err := c.plan(schema, target.Type().Elem(), defs)
	if err != nil {
		return err
	}

	return eachField(data, func(num protowire.Number, wireType protowire.Type, value []byte) (int, error) {
		if num != 1 {
			return skipField, nil
		}
		return plan.readField(value, wireType, target.Elem())
	})
}

// Compile builds the plan for schema mapped onto goType.
func Compile(schema *jtd.Schema, goType reflect.Type, defs map[string]jtd.Schema) (*Node, error) {
	c := compiler{defs: defs, refs: map[refKey]*Node{}}
	return c.compile(schema, goType)
}

type compiler struct {
	defs map[string]jtd.Schema
	refs map[refKey]*Node
}

func (c *compiler) compile(schema *jtd.Schema, goType reflect.Type) (*Node, error) {
	resolved, _, err := codecutil.Resolve(schema, c.defs)
	if err != nil {
		return nil, err
	}

	if schema.Ref == nil {
		n := &Node{}
		return n, c.build(n, resolved, goType)
	}

	key := refKey{*schema.Ref, goType}
	if n, ok := c.refs[key]; ok {
		return n, nil
	}
	n := &Node{}
	c.refs[key] = n
	return n, c.build(n, resolved, goType)
}

func (c *compiler) build(n *Node, schema *jtd.Schema, goType reflect.Type) error {
	n.GoType = goType
	base := goType
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}

	switch schema.Form() {
	case jtd.FormEmpty, jtd.FormDiscriminator:
		n.Kind = KindJSON
	case jtd.FormEnum:
		return n.buildString(base)
	case jtd.FormType:
		return n.buildType(schema.Type, base)
	case jtd.FormElements:
		return c.buildList(n, schema, base)
	case jtd.FormValues:
		return c.buildMap(n, schema, base)
	case jtd.FormProperties:
		return c.buildStruct(n, schema, base)
	default:
		return fmt.Errorf("unsupported schema form %v", schema.Form())
	}
	return nil
}

func (n *Node) buildString(base reflect.Type) error {
	n.Kind = KindString
	if base.Kind() == reflect.String {
		return nil
	}
	if !codecutil.IsTextType(base) {
		return fmt.Errorf("cannot map string schema to %s", base)
	}
	n.Kind = KindText
	return nil
}

func (n *Node) buildType(schemaType jtd.Type, base reflect.Type) error {
	n.SchemaType = schemaType

	var want string
	switch schemaType {
	case jtd.TypeBoolean:
		n.Kind, want = KindBool, "bool"
		if base.Kind() == reflect.Bool {
			return nil
		}
	case jtd.TypeString:
		return n.buildString(base)
	case jtd.TypeTimestamp:
		n.Kind, want = KindTimestamp, "time.Time"
		if base == reflect.TypeFor[time.Time]() {
			return nil
		}
	case jtd.TypeInt8, jtd.TypeInt16, jtd.TypeInt32:
		n.Kind, want = KindInt, "integer"
		if isInteger(base) {
			return nil
		}
	case jtd.TypeUint8, jtd.TypeUint16, jtd.TypeUint32:
		n.Kind, want = KindUint, "integer"
		if isInteger(base) {
			return nil
		}
	case jtd.TypeFloat32:
		n.Kind, want = KindFloat32, "float"
		if base.Kind() == reflect.Float32 || base.Kind() == reflect.Float64 {
			return nil
		}
	case jtd.TypeFloat64:
		n.Kind, want = KindFloat64, "float"
		if base.Kind() == reflect.Float32 || base.Kind() == reflect.Float64 {
			return nil
		}
	default:
		return fmt.Errorf("unsupported schema type %q", schemaType)
	}
	return fmt.Errorf("cannot map %s schema to %s, want %s", schemaType, base, want)
}

func isInteger(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

// needsWrap reports whether a container element must sit in a {1: value}
// message so that null, empty and nested containers stay distinguishable.
func (c *compiler) needsWrap(schema *jtd.Schema) (bool, error) {
	resolved, nullable, err := codecutil.Resolve(schema, c.defs)
	if err != nil {
		return false, err
	}

	switch resolved.Form() {
	case jtd.FormElements, jtd.FormValues, jtd.FormEmpty, jtd.FormDiscriminator:
		return true, nil
	}
	return nullable, nil
}

func (c *compiler) buildList(n *Node, schema *jtd.Schema, base reflect.Type) error {
	if base.Kind() != reflect.Slice {
		return fmt.Errorf("cannot map elements schema to %s", base)
	}

	n.Kind = KindList

	var err error
	n.WrapElem, err = c.needsWrap(schema.Elements)
	if err != nil {
		return err
	}
	n.Elem, err = c.compile(schema.Elements, base.Elem())
	if err != nil {
		return err
	}

	resolved, _, err := codecutil.Resolve(schema.Elements, c.defs)
	if err != nil {
		return err
	}
	n.Packed = !n.WrapElem && resolved.Form() == jtd.FormType && resolved.Type != jtd.TypeString && resolved.Type != jtd.TypeTimestamp
	return nil
}

func (c *compiler) buildMap(n *Node, schema *jtd.Schema, base reflect.Type) error {
	if base.Kind() != reflect.Map {
		return fmt.Errorf("cannot map values schema to %s", base)
	}

	n.Kind = KindMap

	var err error
	n.Key, err = c.compile(&jtd.Schema{Type: jtd.TypeString}, base.Key())
	if err != nil {
		return err
	}
	n.WrapElem, err = c.needsWrap(schema.Values)
	if err != nil {
		return err
	}
	n.Elem, err = c.compile(schema.Values, base.Elem())
	return err
}

func (c *compiler) buildStruct(n *Node, schema *jtd.Schema, base reflect.Type) error {
	if base.Kind() != reflect.Struct {
		return fmt.Errorf("cannot map properties schema to %s", base)
	}

	n.Kind = KindStruct
	goFields := codecutil.StructFields(base)

	for i, prop := range codecutil.Properties(schema) {
		plan := FieldPlan{Name: prop.Name, Num: protowire.Number(i + 1)}

		goField, ok := goFields[prop.Name]
		if ok {
			var err error
			plan.Field = &goField
			plan.Node, err = c.compile(&prop.Schema, goField.Type)
			if err != nil {
				return fmt.Errorf("%s: %w", prop.Name, err)
			}
		}
		n.Fields = append(n.Fields, plan)
	}
	return nil
}

func (n *Node) wireType() protowire.Type {
	switch n.Kind {
	case KindBool, KindInt, KindUint:
		return protowire.VarintType
	case KindFloat32:
		return protowire.Fixed32Type
	case KindFloat64:
		return protowire.Fixed64Type
	}
	return protowire.BytesType
}

// appendField encodes a field with proto3 presence: nil pointers and
// zero non-pointer scalars are omitted, a non-nil pointer is always written.
func (n *Node) appendField(b []byte, num protowire.Number, v reflect.Value) ([]byte, error) {
	explicit := false
	if n.Kind != KindJSON {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return b, nil
			}
			v = v.Elem()
			explicit = true
		}
	}

	switch n.Kind {
	case KindList:
		return n.appendList(b, num, v)
	case KindMap:
		return n.appendMap(b, num, v)
	case KindJSON:
		switch v.Kind() {
		case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice:
			if v.IsNil() {
				return b, nil
			}
		}
	case KindBool, KindInt, KindUint, KindFloat32, KindFloat64, KindString, KindTimestamp:
		if !explicit && v.IsZero() {
			return b, nil
		}
	}

	b = protowire.AppendTag(b, num, n.wireType())
	return n.appendPayload(b, v)
}

func (n *Node) appendPayload(b []byte, v reflect.Value) ([]byte, error) {
	switch n.Kind {
	case KindBool:
		return protowire.AppendVarint(b, protowire.EncodeBool(v.Bool())), nil
	case KindInt:
		num, err := codecutil.IntForSchema(v, n.SchemaType)
		if err != nil {
			return nil, err
		}
		return protowire.AppendVarint(b, protowire.EncodeZigZag(num)), nil
	case KindUint:
		num, err := codecutil.IntForSchema(v, n.SchemaType)
		if err != nil {
			return nil, err
		}
		return protowire.AppendVarint(b, uint64(num)), nil
	case KindFloat32:
		return protowire.AppendFixed32(b, math.Float32bits(float32(v.Float()))), nil
	case KindFloat64:
		return protowire.AppendFixed64(b, math.Float64bits(v.Float())), nil
	case KindString:
		return protowire.AppendString(b, v.String()), nil
	case KindText:
		text, err := codecutil.MarshalText(v)
		if err != nil {
			return nil, err
		}
		return protowire.AppendBytes(b, text), nil
	case KindTimestamp:
		t := v.Interface().(time.Time).UTC()
		var msg []byte
		if seconds := t.Unix(); seconds != 0 {
			msg = protowire.AppendTag(msg, 1, protowire.VarintType)
			msg = protowire.AppendVarint(msg, uint64(seconds))
		}
		if nanos := t.Nanosecond(); nanos != 0 {
			msg = protowire.AppendTag(msg, 2, protowire.VarintType)
			msg = protowire.AppendVarint(msg, uint64(nanos))
		}
		return protowire.AppendBytes(b, msg), nil
	case KindStruct:
		// Reserve a one byte length prefix, widening it afterwards if the body is longer.
		prefixAt := len(b)
		b = append(b, 0)
		bodyAt := len(b)
		for _, plan := range n.Fields {
			if plan.Field == nil {
				continue
			}
			fieldValue, ok := plan.Field.Get(v)
			if !ok {
				continue
			}
			var err error
			b, err = plan.Node.appendField(b, plan.Num, fieldValue)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", plan.Name, err)
			}
		}
		bodyLen := uint64(len(b) - bodyAt)
		if extra := protowire.SizeVarint(bodyLen) - 1; extra > 0 {
			b = slices.Insert(b, bodyAt, make([]byte, extra)...)
		}
		protowire.AppendVarint(b[:prefixAt], bodyLen)
		return b, nil
	case KindJSON:
		encoded, err := json.Marshal(v.Interface())
		if err != nil {
			return nil, err
		}
		return protowire.AppendBytes(b, encoded), nil
	}
	return nil, fmt.Errorf("kind %d has no scalar payload", n.Kind)
}

func derefElement(v reflect.Value) (reflect.Value, error) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return v, fmt.Errorf("nil element in non-nullable list")
		}
		v = v.Elem()
	}
	return v, nil
}

func (n *Node) appendList(b []byte, num protowire.Number, v reflect.Value) ([]byte, error) {
	if v.Len() == 0 {
		return b, nil
	}

	if n.Packed {
		var packed []byte
		for i := range v.Len() {
			element, err := derefElement(v.Index(i))
			if err != nil {
				return nil, err
			}
			packed, err = n.Elem.appendPayload(packed, element)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
		}
		b = protowire.AppendTag(b, num, protowire.BytesType)
		return protowire.AppendBytes(b, packed), nil
	}

	for i := range v.Len() {
		var err error
		if n.WrapElem {
			var wrapper []byte
			wrapper, err = n.Elem.appendField(nil, 1, v.Index(i))
			b = protowire.AppendTag(b, num, protowire.BytesType)
			b = protowire.AppendBytes(b, wrapper)
		} else {
			var element reflect.Value
			element, err = derefElement(v.Index(i))
			if err != nil {
				return nil, err
			}
			b = protowire.AppendTag(b, num, n.Elem.wireType())
			b, err = n.Elem.appendPayload(b, element)
		}
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
	}
	return b, nil
}

func (n *Node) appendMap(b []byte, num protowire.Number, v reflect.Value) ([]byte, error) {
	type entryBytes struct {
		key   string
		bytes []byte
	}
	entries := make([]entryBytes, 0, v.Len())

	for iter := v.MapRange(); iter.Next(); {
		entry, err := n.Key.appendField(nil, 1, iter.Key())
		if err != nil {
			return nil, err
		}
		sortKey := iter.Key().String()
		if n.Key.Kind == KindText {
			text, err := codecutil.MarshalText(iter.Key())
			if err != nil {
				return nil, err
			}
			sortKey = string(text)
		}

		if n.WrapElem {
			wrapper, err := n.Elem.appendField(nil, 1, iter.Value())
			if err != nil {
				return nil, fmt.Errorf("[%v]: %w", iter.Key(), err)
			}
			entry = protowire.AppendTag(entry, 2, protowire.BytesType)
			entry = protowire.AppendBytes(entry, wrapper)
		} else {
			entry, err = n.Elem.appendField(entry, 2, iter.Value())
			if err != nil {
				return nil, fmt.Errorf("[%v]: %w", iter.Key(), err)
			}
		}
		entries = append(entries, entryBytes{sortKey, entry})
	}

	slices.SortFunc(entries, func(a, b entryBytes) int { return strings.Compare(a.key, b.key) })
	for _, entry := range entries {
		b = protowire.AppendTag(b, num, protowire.BytesType)
		b = protowire.AppendBytes(b, entry.bytes)
	}
	return b, nil
}

// eachField walks a message, skipping fields whose visit returns skipField.
func eachField(msg []byte, visit func(num protowire.Number, wireType protowire.Type, value []byte) (int, error)) error {
	for len(msg) > 0 {
		num, wireType, tagLen := protowire.ConsumeTag(msg)
		if tagLen < 0 {
			return protowire.ParseError(tagLen)
		}
		msg = msg[tagLen:]

		consumed, err := visit(num, wireType, msg)
		if err != nil {
			return err
		}
		if consumed == skipField {
			consumed = protowire.ConsumeFieldValue(num, wireType, msg)
			if consumed < 0 {
				return protowire.ParseError(consumed)
			}
		}
		msg = msg[consumed:]
	}
	return nil
}

func allocate(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	return v
}

// readField decodes one occurrence of the field whose value starts at b into
// dst and returns the bytes consumed. Lists and maps accumulate over repeats.
func (n *Node) readField(b []byte, wireType protowire.Type, dst reflect.Value) (int, error) {
	switch n.Kind {
	case KindList:
		return n.readListItem(b, wireType, allocate(dst))
	case KindMap:
		return n.readMapEntry(b, wireType, allocate(dst))
	case KindJSON:
		return n.readPayload(b, wireType, dst)
	}
	return n.readPayload(b, wireType, allocate(dst))
}

func (n *Node) readPayload(b []byte, wireType protowire.Type, dst reflect.Value) (int, error) {
	if wireType != n.wireType() {
		return 0, fmt.Errorf("wire type %d, want %d", wireType, n.wireType())
	}

	switch n.wireType() {
	case protowire.VarintType:
		raw, consumed := protowire.ConsumeVarint(b)
		if consumed < 0 {
			return 0, protowire.ParseError(consumed)
		}
		return consumed, n.setVarint(dst, raw)
	case protowire.Fixed32Type:
		raw, consumed := protowire.ConsumeFixed32(b)
		if consumed < 0 {
			return 0, protowire.ParseError(consumed)
		}
		return consumed, codecutil.SetFloat(dst, float64(math.Float32frombits(raw)))
	case protowire.Fixed64Type:
		raw, consumed := protowire.ConsumeFixed64(b)
		if consumed < 0 {
			return 0, protowire.ParseError(consumed)
		}
		return consumed, codecutil.SetFloat(dst, math.Float64frombits(raw))
	}

	body, consumed := protowire.ConsumeBytes(b)
	if consumed < 0 {
		return 0, protowire.ParseError(consumed)
	}
	return consumed, n.setBytes(dst, body)
}

func (n *Node) setVarint(dst reflect.Value, raw uint64) error {
	switch n.Kind {
	case KindBool:
		dst.SetBool(protowire.DecodeBool(raw))
		return nil
	case KindInt:
		return codecutil.SetInt(dst, protowire.DecodeZigZag(raw))
	}
	if raw > math.MaxInt64 {
		return fmt.Errorf("value %d out of range", raw)
	}
	return codecutil.SetInt(dst, int64(raw))
}

func (n *Node) setBytes(dst reflect.Value, body []byte) error {
	switch n.Kind {
	case KindString:
		dst.SetString(string(body))
		return nil
	case KindText:
		return codecutil.UnmarshalText(dst, body)
	case KindJSON:
		return json.Unmarshal(body, dst.Addr().Interface())
	case KindTimestamp:
		t, err := ConsumeTimestamp(body)
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(t))
		return nil
	}

	return eachField(body, func(num protowire.Number, wireType protowire.Type, value []byte) (int, error) {
		index := int(num) - 1
		if index < 0 || index >= len(n.Fields) || n.Fields[index].Field == nil {
			return skipField, nil
		}
		plan := n.Fields[index]
		consumed, err := plan.Node.readField(value, wireType, plan.Field.Ensure(dst))
		if err != nil {
			return 0, fmt.Errorf("%s: %w", plan.Name, err)
		}
		return consumed, nil
	})
}

func (n *Node) readListItem(b []byte, wireType protowire.Type, dst reflect.Value) (int, error) {
	if n.Packed && wireType == protowire.BytesType {
		packed, consumed := protowire.ConsumeBytes(b)
		if consumed < 0 {
			return 0, protowire.ParseError(consumed)
		}
		for len(packed) > 0 {
			element := appendSlot(dst)
			read, err := n.Elem.readPayload(packed, n.Elem.wireType(), element)
			if err != nil {
				return 0, err
			}
			packed = packed[read:]
		}
		return consumed, nil
	}

	element := appendSlot(dst)

	if !n.WrapElem {
		return n.Elem.readPayload(b, wireType, allocate(element))
	}

	if wireType != protowire.BytesType {
		return 0, fmt.Errorf("wire type %d, want bytes", wireType)
	}
	wrapper, consumed := protowire.ConsumeBytes(b)
	if consumed < 0 {
		return 0, protowire.ParseError(consumed)
	}
	return consumed, n.readWrapper(wrapper, element)
}

func appendSlot(slice reflect.Value) reflect.Value {
	length := slice.Len()
	slice.Grow(1)
	slice.SetLen(length + 1)
	return slice.Index(length)
}

func (n *Node) readWrapper(wrapper []byte, dst reflect.Value) error {
	return eachField(wrapper, func(num protowire.Number, wireType protowire.Type, value []byte) (int, error) {
		if num != 1 {
			return skipField, nil
		}
		return n.Elem.readField(value, wireType, dst)
	})
}

func (n *Node) readMapEntry(b []byte, wireType protowire.Type, dst reflect.Value) (int, error) {
	if wireType != protowire.BytesType {
		return 0, fmt.Errorf("wire type %d, want bytes", wireType)
	}
	entry, consumed := protowire.ConsumeBytes(b)
	if consumed < 0 {
		return 0, protowire.ParseError(consumed)
	}

	if dst.IsNil() {
		dst.Set(reflect.MakeMap(dst.Type()))
	}
	key := reflect.New(dst.Type().Key()).Elem()
	value := reflect.New(dst.Type().Elem()).Elem()

	err := eachField(entry, func(num protowire.Number, wireType protowire.Type, rest []byte) (int, error) {
		switch {
		case num == 1:
			return n.Key.readField(rest, wireType, key)
		case num == 2 && n.WrapElem:
			if wireType != protowire.BytesType {
				return 0, fmt.Errorf("wire type %d, want bytes", wireType)
			}
			wrapper, read := protowire.ConsumeBytes(rest)
			if read < 0 {
				return 0, protowire.ParseError(read)
			}
			return read, n.readWrapper(wrapper, value)
		case num == 2:
			return n.Elem.readField(rest, wireType, value)
		}
		return skipField, nil
	})
	if err != nil {
		return 0, err
	}

	dst.SetMapIndex(key, value)
	return consumed, nil
}
