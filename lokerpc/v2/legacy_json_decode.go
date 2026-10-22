package lokerpc

import (
	"reflect"
	"strconv"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// unmarshalLegacyJSON handles the common generated, open-message shape. The
// scanner deliberately recognizes a subset of protojson: anything ambiguous or
// unsupported is replayed through protojson, including its partial-result and
// error behavior. No scanner error escapes this boundary.
func unmarshalLegacyJSON(data []byte, message proto.Message) error {
	if message != nil && message.ProtoReflect().IsValid() {
		if plan := legacyPlan(reflect.TypeOf(message), message.ProtoReflect().Descriptor()); plan != nil {
			proto.Reset(message)
			scanner := legacyJSONDecoder{data: data}
			if scanner.message(plan, reflect.ValueOf(message).Elem(), 0) && scanner.space() == len(data) {
				return nil
			}
		}
	}
	// Unmarshal resets the destination before replaying, so mutations made by an
	// abandoned fast parse cannot leak into either success or partial failure.
	return (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, message)
}

type legacyJSONDecoder struct {
	data []byte
	pos  int
}

func (d *legacyJSONDecoder) space() int {
	for d.pos < len(d.data) {
		switch d.data[d.pos] {
		case ' ', '\t', '\r', '\n':
			d.pos++
		default:
			return d.pos
		}
	}
	return d.pos
}

func (d *legacyJSONDecoder) take(c byte) bool {
	if d.space() == len(d.data) || d.data[d.pos] != c {
		return false
	}
	d.pos++
	return true
}

// text borrows an unescaped string until the caller copies a value into its
// destination. Escapes use the reference decoder, which also validates surrogate
// pairs and the complete protobuf JSON string grammar.
func (d *legacyJSONDecoder) text() ([]byte, bool) {
	if !d.take('"') {
		return nil, false
	}
	start := d.pos
	for d.pos < len(d.data) {
		c := d.data[d.pos]
		d.pos++
		if c == '"' {
			text := d.data[start : d.pos-1]
			return text, utf8.Valid(text)
		}
		if c < 0x20 || c == '\\' {
			return nil, false
		}
	}
	return nil, false
}

func (d *legacyJSONDecoder) message(plan *legacyMessagePlan, value reflect.Value, depth int) bool {
	if depth > 100 || len(plan.fields) > 64 || !d.take('{') {
		return false
	}
	if d.take('}') {
		return true
	}
	var seen uint64
	for {
		name, ok := d.text()
		if !ok || !d.take(':') {
			return false
		}
		index := -1
		for i := range plan.fields {
			field := &plan.fields[i]
			if string(name) == field.descriptor.JSONName() {
				index = i
				break
			}
		}
		if index < 0 {
			for i := range plan.fields {
				if string(name) == string(plan.fields[i].descriptor.Name()) {
					index = i
					break
				}
			}
		}
		// Unknown fields, aliases, and duplicate names all retain the exact
		// reference behavior by replaying rather than skipping tokens here.
		if index < 0 || seen&(uint64(1)<<index) != 0 {
			return false
		}
		seen |= uint64(1) << index
		field := &plan.fields[index]
		if d.space() == len(d.data) {
			return false
		}
		if d.data[d.pos] == 'n' {
			// Repeated Value and NullValue fields do not treat null as an
			// absent list. Replay all null lists to keep that distinction.
			if field.list || !d.literal("null") {
				return false
			}
		} else if field.list {
			if !d.list(field, value.Field(field.index), depth+1) {
				return false
			}
		} else if !d.scalar(field, value.Field(field.index), depth+1) {
			return false
		}
		if d.take('}') {
			return true
		}
		if !d.take(',') {
			return false
		}
	}
}

func (d *legacyJSONDecoder) literal(text string) bool {
	if len(d.data)-d.pos < len(text) || string(d.data[d.pos:d.pos+len(text)]) != text {
		return false
	}
	d.pos += len(text)
	return true
}

// listSize counts top-level elements without allocating. The real parse still
// validates every token. Exact sizing avoids growing each generated slice, and
// lets repeated messages share a contiguous backing allocation instead of one
// allocation per child message.
func (d *legacyJSONDecoder) listSize(minBytes int) (int, bool) {
	start := d.space()
	if start == len(d.data) {
		return 0, false
	}
	if d.data[start] == ']' {
		return 0, true
	}
	count, depth := 1, 0
	quoted, escaped := false, false
	for i := start; i < len(d.data); i++ {
		c := d.data[i]
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		switch c {
		case '"':
			quoted = true
		case '{', '[':
			depth++
			if depth > 100 {
				return 0, false
			}
		case '}':
			depth--
			if depth < 0 {
				return 0, false
			}
		case ']':
			if depth == 0 {
				// Even the smallest valid element plus its separator needs
				// minBytes bytes. Reject comma-only input before allocation.
				return count, count <= (i-start+1)/minBytes
			}
			depth--
		case ',':
			if depth == 0 {
				count++
			}
		}
	}
	return 0, false
}

func (d *legacyJSONDecoder) list(field *legacyFieldPlan, value reflect.Value, depth int) bool {
	if depth > 100 || !d.take('[') {
		return false
	}
	minBytes := 2 // A scalar and its comma or closing bracket.
	if field.kind == protoreflect.MessageKind {
		minBytes = 3 // An empty object and its comma or closing bracket.
	}
	count, ok := d.listSize(minBytes)
	if !ok {
		return false
	}
	if count == 0 {
		return d.take(']')
	}
	value.Set(reflect.MakeSlice(value.Type(), count, count))
	var messages reflect.Value
	if field.kind == protoreflect.MessageKind {
		if field.child == nil {
			return false
		}
		messages = reflect.MakeSlice(reflect.SliceOf(value.Type().Elem().Elem()), count, count)
	}
	for i := range count {
		if i > 0 && !d.take(',') {
			return false
		}
		if messages.IsValid() {
			value.Index(i).Set(messages.Index(i).Addr())
			if !d.message(field.child, messages.Index(i), depth+1) {
				return false
			}
		} else if !d.scalar(field, value.Index(i), depth+1) {
			return false
		}
	}
	return d.take(']')
}

func (d *legacyJSONDecoder) scalar(field *legacyFieldPlan, value reflect.Value, depth int) bool {
	if d.space() == len(d.data) {
		return false
	}
	switch field.kind {
	case protoreflect.StringKind:
		text, ok := d.text()
		if ok {
			value.SetString(string(text))
		}
		return ok
	case protoreflect.BoolKind:
		if d.literal("true") {
			value.SetBool(true)
			return true
		}
		if d.literal("false") {
			value.SetBool(false)
			return true
		}
		return false
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind, protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		text, ok := d.integer()
		if !ok {
			return false
		}
		if value.Kind() == reflect.Int32 || value.Kind() == reflect.Int64 {
			number, err := strconv.ParseInt(string(text), 10, value.Type().Bits())
			if err != nil {
				return false
			}
			value.SetInt(number)
		} else {
			number, err := strconv.ParseUint(string(text), 10, value.Type().Bits())
			if err != nil {
				return false
			}
			value.SetUint(number)
		}
		return true
	default:
		return false
	}
}

// integer accepts only canonical integral tokens. Exponents, fractional forms,
// escapes, plus signs and noncanonical quoted numbers are valid in some cases;
// the reference decoder decides those rather than duplicating its number rules.
func (d *legacyJSONDecoder) integer() ([]byte, bool) {
	quoted := d.data[d.pos] == '"'
	if quoted {
		d.pos++
	}
	start := d.pos
	if d.pos < len(d.data) && d.data[d.pos] == '-' {
		d.pos++
	}
	digits := d.pos
	for d.pos < len(d.data) && d.data[d.pos] >= '0' && d.data[d.pos] <= '9' {
		d.pos++
	}
	if digits == d.pos || d.pos-digits > 1 && d.data[digits] == '0' {
		return nil, false
	}
	end := d.pos
	if quoted && (d.pos == len(d.data) || d.data[d.pos] != '"') {
		return nil, false
	}
	if quoted {
		d.pos++
	}
	return d.data[start:end], true
}
