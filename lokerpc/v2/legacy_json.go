package lokerpc

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
)

// legacyJSONCodec is private to the bridge: ordinary Connect JSON keeps its
// standard codec and omission rules.
type legacyJSONCodec struct{}

func (legacyJSONCodec) Name() string { return "loke-legacy-json" }

var legacyMarshal = protojson.MarshalOptions{EmitDefaultValues: true}

// A large outlier must not make every subsequent small response reserve megabytes.
const legacySizeHintLimit = 512 << 10

// Protojson deliberately varies comma whitespace between binaries. Probe once
// rather than depending on protobuf's internal implementation or normalizing
// strings (which would also risk changing string values).
var legacyJSONComma = func() string {
	data, _ := legacyMarshal.Marshal(&structpb.ListValue{Values: []*structpb.Value{
		structpb.NewBoolValue(false), structpb.NewBoolValue(true),
	}})
	if bytes.Contains(data, []byte(", ")) {
		return ", "
	}
	return ","
}()

type legacyJSONField struct {
	descriptor protoreflect.FieldDescriptor
	name       []byte
}

var legacyJSONFields sync.Map // protoreflect.MessageDescriptor -> []legacyJSONField

func legacyFields(descriptor protoreflect.MessageDescriptor) ([]legacyJSONField, error) {
	if fields, ok := legacyJSONFields.Load(descriptor); ok {
		return fields.([]legacyJSONField), nil
	}
	fields := make([]legacyJSONField, descriptor.Fields().Len())
	for i := range fields {
		field := descriptor.Fields().Get(i)
		name, err := appendLegacyString(nil, field.JSONName())
		if err != nil {
			return nil, err
		}
		fields[i] = legacyJSONField{descriptor: field, name: append(name, ':')}
	}
	actual, _ := legacyJSONFields.LoadOrStore(descriptor, fields)
	return actual.([]legacyJSONField), nil
}

var legacyJSONBuffers = sync.Pool{New: func() any {
	buffer := make([]byte, 0, 4096)
	return &buffer
}}

func (codec legacyJSONCodec) Marshal(value any) ([]byte, error) {
	buffer := legacyJSONBuffers.Get().(*[]byte)
	data, err := codec.MarshalAppend((*buffer)[:0], value)
	if err != nil {
		legacyJSONBuffers.Put(buffer)
		return nil, err
	}
	result := bytes.Clone(data)
	*buffer = data[:0]
	legacyJSONBuffers.Put(buffer)
	return result, nil
}

// MarshalAppend lets Connect reuse its response buffer.
func (legacyJSONCodec) MarshalAppend(data []byte, value any) ([]byte, error) {
	message, ok := value.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("legacy JSON: %T is not a protobuf message", value)
	}
	if message == nil || !message.ProtoReflect().IsValid() {
		return legacyMarshal.MarshalAppend(data, message)
	}
	original := data
	var err error
	if plan := legacyPlan(reflect.TypeOf(message), message.ProtoReflect().Descriptor()); plan != nil {
		// A pool miss should allocate once, not grow through every size class.
		// Connect owns and recycles the returned buffer after writing it.
		if size := int(plan.encodedSize.Load()); size > cap(data)-len(data) {
			grown := make([]byte, len(data), len(data)+size)
			copy(grown, data)
			data = grown
		}
		data, err = plan.append(data, reflect.ValueOf(message).Elem(), 0)
		if err == nil {
			plan.encodedSize.Store(int64(min(len(data)-len(original), legacySizeHintLimit)))
		}
	} else {
		data, err = appendLegacyMessage(data, message.ProtoReflect(), 0)
	}
	if err == nil {
		err = proto.CheckInitialized(message)
	}
	if err != nil {
		// Keep protojson's validation and error contract, including invalid UTF-8,
		// required fields, unsupported descriptors and invalid well-known values.
		return legacyMarshal.MarshalAppend(original, message)
	}
	return data, nil
}

func (legacyJSONCodec) Unmarshal(data []byte, value any) error {
	message, ok := value.(proto.Message)
	if !ok {
		return fmt.Errorf("legacy JSON: %T is not a protobuf message", value)
	}
	// Empty requests are common for legacy endpoints. Protojson resets its
	// destination too, but ordinary empty objects need no JSON tokenization.
	if bytes.Equal(bytes.Trim(data, " \t\r\n"), []byte("{}")) && legacyOrdinaryMessage(message.ProtoReflect().Descriptor()) {
		proto.Reset(message)
		return nil
	}
	return unmarshalLegacyJSON(data, message)
}

func legacyOrdinaryMessage(descriptor protoreflect.MessageDescriptor) bool {
	return descriptor.ParentFile().Syntax() == protoreflect.Proto3 &&
		!strings.HasPrefix(string(descriptor.FullName()), "google.protobuf.")
}

func appendLegacyMessage(data []byte, message protoreflect.Message, depth int) ([]byte, error) {
	if depth > 100 {
		return nil, fmt.Errorf("legacy JSON: message nesting exceeds fast path")
	}
	if !legacyOrdinaryMessage(message.Descriptor()) {
		return legacyMarshal.MarshalAppend(data, message.Interface())
	}
	fields, err := legacyFields(message.Descriptor())
	if err != nil {
		return nil, err
	}
	data = append(data, '{')
	first := true
	for _, field := range fields {
		descriptor := field.descriptor
		if descriptor.HasPresence() && !message.Has(descriptor) {
			continue
		}
		if !first {
			data = append(data, legacyJSONComma...)
		}
		first = false
		data = append(data, field.name...)
		var err error
		data, err = appendLegacyValue(data, message.Get(descriptor), descriptor, depth+1)
		if err != nil {
			return nil, err
		}
	}
	return append(data, '}'), nil
}

func appendLegacyValue(data []byte, value protoreflect.Value, descriptor protoreflect.FieldDescriptor, depth int) ([]byte, error) {
	if descriptor.IsList() {
		data = append(data, '[')
		list := value.List()
		for i := range list.Len() {
			if i > 0 {
				data = append(data, legacyJSONComma...)
			}
			var err error
			data, err = appendLegacyScalar(data, list.Get(i), descriptor, depth)
			if err != nil {
				return nil, err
			}
		}
		return append(data, ']'), nil
	}
	if descriptor.IsMap() {
		data = append(data, '{')
		entries := value.Map()
		keys := make([]protoreflect.MapKey, 0, entries.Len())
		entries.Range(func(key protoreflect.MapKey, _ protoreflect.Value) bool {
			keys = append(keys, key)
			return true
		})
		sort.Slice(keys, func(i, j int) bool {
			switch descriptor.MapKey().Kind() {
			case protoreflect.BoolKind:
				return !keys[i].Bool() && keys[j].Bool()
			case protoreflect.StringKind:
				return keys[i].String() < keys[j].String()
			case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Fixed32Kind, protoreflect.Fixed64Kind:
				return keys[i].Uint() < keys[j].Uint()
			default:
				return keys[i].Int() < keys[j].Int()
			}
		})
		for i, key := range keys {
			if i > 0 {
				data = append(data, legacyJSONComma...)
			}
			var err error
			data, err = appendLegacyString(data, key.String())
			if err != nil {
				return nil, err
			}
			data = append(data, ':')
			data, err = appendLegacyScalar(data, entries.Get(key), descriptor.MapValue(), depth)
			if err != nil {
				return nil, err
			}
		}
		return append(data, '}'), nil
	}
	return appendLegacyScalar(data, value, descriptor, depth)
}

func appendLegacyScalar(data []byte, value protoreflect.Value, descriptor protoreflect.FieldDescriptor, depth int) ([]byte, error) {
	switch descriptor.Kind() {
	case protoreflect.BoolKind:
		return strconv.AppendBool(data, value.Bool()), nil
	case protoreflect.StringKind:
		return appendLegacyString(data, value.String())
	case protoreflect.BytesKind:
		data = append(data, '"')
		data = base64.StdEncoding.AppendEncode(data, value.Bytes())
		return append(data, '"'), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return strconv.AppendInt(data, value.Int(), 10), nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return strconv.AppendUint(data, value.Uint(), 10), nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		data = strconv.AppendInt(append(data, '"'), value.Int(), 10)
		return append(data, '"'), nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		data = strconv.AppendUint(append(data, '"'), value.Uint(), 10)
		return append(data, '"'), nil
	case protoreflect.EnumKind:
		if descriptor.Enum().FullName() == "google.protobuf.NullValue" {
			return append(data, "null"...), nil
		}
		if enum := descriptor.Enum().Values().ByNumber(value.Enum()); enum != nil {
			return appendLegacyString(data, string(enum.Name()))
		}
		return strconv.AppendInt(data, int64(value.Enum()), 10), nil
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		number := value.Float()
		switch {
		case math.IsNaN(number):
			return append(data, `"NaN"`...), nil
		case math.IsInf(number, 1):
			return append(data, `"Infinity"`...), nil
		case math.IsInf(number, -1):
			return append(data, `"-Infinity"`...), nil
		}
		bits := 64
		if descriptor.Kind() == protoreflect.FloatKind {
			bits = 32
		}
		format := byte('f')
		if abs := math.Abs(number); abs != 0 && (bits == 64 && (abs < 1e-6 || abs >= 1e21) || bits == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21)) {
			format = 'e'
		}
		data = strconv.AppendFloat(data, number, format, -1, bits)
		if n := len(data); format == 'e' && n >= 4 && data[n-4] == 'e' && data[n-3] == '-' && data[n-2] == '0' {
			data[n-2] = data[n-1]
			data = data[:n-1]
		}
		return data, nil
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return appendLegacyMessage(data, value.Message(), depth)
	default:
		return nil, fmt.Errorf("legacy JSON: unsupported field %s", descriptor.FullName())
	}
}

func appendLegacyString(data []byte, value string) ([]byte, error) {
	data = append(data, '"')
	start := 0
	// Most response text is plain ASCII. Scan eight bytes at a time without
	// allocating or touching the rune decoder on this common path.
	for start+8 <= len(value) {
		word := binary.LittleEndian.Uint64([]byte(value[start : start+8]))
		const low = uint64(0x0101010101010101)
		const high = uint64(0x8080808080808080)
		quote, slash := word^(low*'"'), word^(low*'\\')
		if word&high != 0 || (word-low*' ')&^word&high != 0 ||
			(quote-low)&^quote&high != 0 || (slash-low)&^slash&high != 0 {
			break
		}
		start += 8
	}
	data = append(data, value[:start]...)
	for i := start; i < len(value); {
		c := value[i]
		if c >= utf8.RuneSelf {
			r, size := utf8.DecodeRuneInString(value[i:])
			if r == utf8.RuneError && size == 1 {
				return nil, fmt.Errorf("legacy JSON: invalid UTF-8")
			}
			i += size
			continue
		}
		if c >= ' ' && c != '"' && c != '\\' {
			i++
			continue
		}
		data = append(data, value[start:i]...)
		switch c {
		case '"', '\\':
			data = append(data, '\\', c)
		case '\b':
			data = append(data, `\b`...)
		case '\f':
			data = append(data, `\f`...)
		case '\n':
			data = append(data, `\n`...)
		case '\r':
			data = append(data, `\r`...)
		case '\t':
			data = append(data, `\t`...)
		default:
			const hex = "0123456789abcdef"
			data = append(data, '\\', 'u', '0', '0', hex[c>>4], hex[c&15])
		}
		i++
		start = i
	}
	data = append(data, value[start:]...)
	return append(data, '"'), nil
}
