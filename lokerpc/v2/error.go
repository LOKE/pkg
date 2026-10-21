package lokerpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/LOKE/pkg/lokerpc/v2/rpcv1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

const errorDetailType = "loke.rpc.v1.Error"

func findError(err error, matches func(error) bool) error {
	for err != nil {
		if matches(err) {
			return err
		}
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			for _, cause := range joined.Unwrap() {
				if found := findError(cause, matches); found != nil {
					return found
				}
			}
			return nil
		}
		err = errors.Unwrap(err)
	}
	return nil
}

func standardErrorDetail(err error) *connect.ErrorDetail {
	var result *connect.ErrorDetail
	findError(err, func(err error) bool {
		if connectErr, ok := err.(*connect.Error); ok {
			for _, detail := range connectErr.Details() {
				if detail.Type() == errorDetailType {
					result = detail
					return true
				}
			}
		}
		return false
	})
	return result
}

type errorInterceptor struct{}

func (errorInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		response, err := next(ctx, request)
		return response, withErrorDetail(err)
	}
}

func (errorInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (errorInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, connection connect.StreamingHandlerConn) error {
		return withErrorDetail(next(ctx, connection))
	}
}

func withErrorDetail(err error) error {
	if err == nil {
		return nil
	}
	detail := standardErrorDetail(err)
	var coded error
	if detail == nil {
		coded = findError(err, func(err error) bool {
			value, ok := err.(interface{ ErrorCode() string })
			return ok && value.ErrorCode() != ""
		})
		if coded == nil {
			return err
		}
		code := coded.(interface{ ErrorCode() string }).ErrorCode()
		value := &rpcv1.Error{Code: code, Type: code}
		if public, ok := coded.(interface{ Public() bool }); ok {
			value.Expose = public.Public()
		}
		if id, ok := coded.(interface{ ErrorID() string }); ok {
			value.Instance = id.ErrorID()
		}
		if errorType, ok := coded.(interface{ ErrorType() string }); ok && errorType.ErrorType() != "" {
			value.Type = errorType.ErrorType()
		}
		if namespace, ok := coded.(interface{ ErrorNamespace() string }); ok {
			value.Namespace = namespace.ErrorNamespace()
		}
		if help, ok := coded.(interface{ ErrorHelp() string }); ok {
			value.Help = help.ErrorHelp()
		}
		if meta, ok := coded.(interface{ ErrorMeta() map[string]any }); ok {
			value.Meta = errorMetadata(meta.ErrorMeta())
		}
		var detailErr error
		detail, detailErr = connect.NewErrorDetail(value)
		if detailErr != nil {
			return err
		}
	}
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		for _, existing := range connectErr.Details() {
			if existing == detail {
				return err
			}
		}
		// Service errors may be shared sentinels: never mutate their details or
		// lazily initialized metadata while serving concurrent requests.
		original := *connectErr
		underlying := original.Unwrap()
		var codedConnect *connect.Error
		if coded != nil && errors.As(coded, &codedConnect) && codedConnect == connectErr {
			underlying = coded
		}
		if connect.IsWireError(connectErr) {
			connectErr = connect.NewWireError(original.Code(), underlying)
		} else {
			connectErr = connect.NewError(original.Code(), underlying)
		}
		for _, existing := range original.Details() {
			connectErr.AddDetail(existing)
		}
		for name, values := range original.Meta() {
			connectErr.Meta()[name] = append([]string(nil), values...)
		}
	} else {
		connectErr = connect.NewError(connect.CodeUnknown, err)
	}
	connectErr.AddDetail(detail)
	return connectErr
}

func reservedErrorField(name string) bool {
	switch name {
	case "message", "instance", "expose", "code", "namespace", "type", "help", "name", "stack":
		return true
	default:
		return false
	}
}

func errorMetadata(fields map[string]any) *structpb.Struct {
	if fields == nil {
		return nil
	}
	result := &structpb.Struct{Fields: make(map[string]*structpb.Value, len(fields))}
	for name, raw := range fields {
		if reservedErrorField(name) {
			continue
		}
		value, err := structpb.NewValue(raw)
		if err != nil {
			// Accept JSON-compatible Go structs and typed collections too.
			data, marshalErr := json.Marshal(raw)
			if marshalErr != nil {
				continue
			}
			var compatible any
			if json.Unmarshal(data, &compatible) != nil {
				continue
			}
			value, err = structpb.NewValue(compatible)
		}
		if err == nil {
			result.Fields[name] = value
		}
	}
	return result
}

func decodeErrorDetail(value string) (*rpcv1.Error, bool) {
	body, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		body, err = base64.StdEncoding.DecodeString(value)
	}
	if err != nil {
		return nil, false
	}
	detail := new(rpcv1.Error)
	if err := proto.Unmarshal(body, detail); err != nil {
		return nil, false
	}
	return detail, true
}

func writeLegacyError(w http.ResponseWriter, message string, detail *rpcv1.Error) {
	keys := []string{"message", "instance"}
	values := map[string]*structpb.Value{
		"message":  structpb.NewStringValue(message),
		"instance": structpb.NewStringValue(detail.Instance),
	}
	if detail.Expose {
		keys = append(keys, "expose")
		values["expose"] = structpb.NewBoolValue(true)
	}
	keys = append(keys, "code")
	values["code"] = structpb.NewStringValue(detail.Code)
	if detail.Namespace != "" {
		keys = append(keys, "namespace")
		values["namespace"] = structpb.NewStringValue(detail.Namespace)
	}
	keys = append(keys, "type")
	kind := detail.Type
	if kind == "" {
		kind = detail.Code
	}
	values["type"] = structpb.NewStringValue(kind)
	for _, name := range legacyJSONKeys(detail.Meta.GetFields()) {
		if reservedErrorField(name) {
			continue
		}
		keys = append(keys, name)
		values[name] = detail.Meta.Fields[name]
	}
	// JSON.stringify enumerates integer indices before named properties,
	// even when the indices were added after BaseError's fixed fields.
	sort.SliceStable(keys, func(i, j int) bool {
		left, leftIndex := legacyJSONIndex(keys[i])
		right, rightIndex := legacyJSONIndex(keys[j])
		return leftIndex && (!rightIndex || left < right)
	})
	var body bytes.Buffer
	writeLegacyJSONObject(&body, keys, values)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write(body.Bytes())
}

// Array indices exclude 2^32-1 and noncanonical forms such as "01" or "-0".
func legacyJSONIndex(key string) (uint64, bool) {
	if key == "" || len(key) > 10 || (len(key) > 1 && key[0] == '0') {
		return 0, false
	}
	var index uint64
	for i := range key {
		if key[i] < '0' || key[i] > '9' {
			return 0, false
		}
		index = index*10 + uint64(key[i]-'0')
	}
	return index, index < 1<<32-1
}

func legacyJSONKeys(fields map[string]*structpb.Value) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, leftIndex := legacyJSONIndex(keys[i])
		right, rightIndex := legacyJSONIndex(keys[j])
		if leftIndex || rightIndex {
			return leftIndex && (!rightIndex || left < right)
		}
		return legacyJSONStringLess(keys[i], keys[j])
	})
	return keys
}

// JavaScript sorts strings by UTF-16 code units, not UTF-8 bytes. Supplementary
// runes sort before BMP runes starting at U+E000; other rune ordering agrees.
func legacyJSONStringLess(left, right string) bool {
	for left != "" && right != "" {
		l, ln := utf8.DecodeRuneInString(left)
		r, rn := utf8.DecodeRuneInString(right)
		if l != r {
			if l >= 0x10000 && r >= 0xe000 && r < 0x10000 {
				return true
			}
			if r >= 0x10000 && l >= 0xe000 && l < 0x10000 {
				return false
			}
			return l < r
		}
		left, right = left[ln:], right[rn:]
	}
	return left == "" && right != ""
}

func writeLegacyJSONObject(body *bytes.Buffer, keys []string, values map[string]*structpb.Value) {
	body.WriteByte('{')
	for i, key := range keys {
		if i != 0 {
			body.WriteByte(',')
		}
		writeLegacyJSONString(body, key)
		body.WriteByte(':')
		writeLegacyJSONValue(body, values[key])
	}
	body.WriteByte('}')
}

func writeLegacyJSONValue(body *bytes.Buffer, value *structpb.Value) {
	switch value.GetKind().(type) {
	case *structpb.Value_StringValue:
		writeLegacyJSONString(body, value.GetStringValue())
	case *structpb.Value_BoolValue:
		body.WriteString(strconv.FormatBool(value.GetBoolValue()))
	case *structpb.Value_NumberValue:
		number := value.GetNumberValue()
		switch {
		case math.IsNaN(number) || math.IsInf(number, 0):
			body.WriteString("null")
		case number == 0:
			body.WriteByte('0')
		default:
			format := byte('f')
			if abs := math.Abs(number); abs < 1e-6 || abs >= 1e21 {
				format = 'e'
			}
			var scratch [32]byte
			encoded := strconv.AppendFloat(scratch[:0], number, format, -1, 64)
			// Go pads negative exponents; ECMAScript does not.
			if n := len(encoded); n >= 4 && encoded[n-4] == 'e' && encoded[n-3] == '-' && encoded[n-2] == '0' {
				encoded[n-2] = encoded[n-1]
				encoded = encoded[:n-1]
			}
			body.Write(encoded)
		}
	case *structpb.Value_StructValue:
		fields := value.GetStructValue().GetFields()
		writeLegacyJSONObject(body, legacyJSONKeys(fields), fields)
	case *structpb.Value_ListValue:
		body.WriteByte('[')
		for i, item := range value.GetListValue().GetValues() {
			if i != 0 {
				body.WriteByte(',')
			}
			writeLegacyJSONValue(body, item)
		}
		body.WriteByte(']')
	default:
		body.WriteString("null")
	}
}

func writeLegacyJSONString(body *bytes.Buffer, value string) {
	const hex = "0123456789abcdef"
	body.WriteByte('"')
	for _, char := range value {
		switch char {
		case '"', '\\':
			body.WriteByte('\\')
			body.WriteByte(byte(char))
		case '\b':
			body.WriteString(`\b`)
		case '\f':
			body.WriteString(`\f`)
		case '\n':
			body.WriteString(`\n`)
		case '\r':
			body.WriteString(`\r`)
		case '\t':
			body.WriteString(`\t`)
		default:
			if char < 0x20 {
				body.WriteString(`\u00`)
				body.WriteByte(hex[char>>4])
				body.WriteByte(hex[char&15])
			} else {
				body.WriteRune(char)
			}
		}
	}
	body.WriteByte('"')
}
