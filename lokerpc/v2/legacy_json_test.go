package lokerpc

import (
	"bytes"
	"math"
	"math/rand/v2"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func compareLegacyJSON(t *testing.T, message proto.Message) {
	t.Helper()
	want, wantErr := (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(message)
	got, gotErr := (legacyJSONCodec{}).Marshal(message)
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("%T: error mismatch: got %v, want %v", message, gotErr, wantErr)
	}
	if wantErr != nil {
		if gotErr.Error() != wantErr.Error() {
			t.Fatalf("%T: error mismatch: got %v, want %v", message, gotErr, wantErr)
		}
		return
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%T: JSON mismatch\ngot:  %s\nwant: %s", message, got, want)
	}
}

func legacyJSONFixture() *testpb.Scalars {
	anyValue, _ := anypb.New(&testpb.Scalars_Nested{Name: "nested any"})
	return &testpb.Scalars{
		DoubleValue: math.Inf(1), FloatValue: float32(math.Inf(-1)),
		Int32Value: math.MinInt32, Int64Value: math.MinInt64,
		Uint32Value: math.MaxUint32, Uint64Value: math.MaxUint64,
		Sint32Value: -123, Sint64Value: -456, Fixed32Value: 123,
		Fixed64Value: math.MaxUint64, Sfixed32Value: -123, Sfixed64Value: math.MinInt64,
		BoolValue: true, StringValue: "\x00\b\f\n\r\t\"\\<>&\u2028\u2029é�",
		BytesValue: []byte{0, 1, 254, 255}, State: testpb.State_STATE_READY,
		Tags:          []string{"first", "second"},
		Entries:       map[string]*testpb.Scalars_Nested{"z": {Name: "last"}, "a": {}},
		Labels:        map[int32]string{10: "ten", -1: "negative", 2: "two"},
		OptionalValue: proto.String(""), Choice: &testpb.Scalars_ChoiceText{ChoiceText: ""},
		Nested: &testpb.Scalars_Nested{}, Left: &testpb.Left_Item{LeftValue: "left"},
		Right: &testpb.Right_Item{RightValue: 42}, Renamed: "custom name",
		Timestamp:   &timestamppb.Timestamp{Seconds: 1700000000, Nanos: 123000000},
		Duration:    &durationpb.Duration{Seconds: -3, Nanos: -123},
		FieldMask:   &fieldmaskpb.FieldMask{Paths: []string{"nested.name", "string_value"}},
		StructValue: &structpb.Struct{Fields: map[string]*structpb.Value{"z": structpb.NewNullValue(), "a": structpb.NewNumberValue(1)}},
		Value:       structpb.NewStringValue("value"),
		ListValue:   &structpb.ListValue{Values: []*structpb.Value{structpb.NewBoolValue(true), structpb.NewNullValue()}},
		AnyValue:    anyValue, Empty: &emptypb.Empty{},
		WrappedDouble: wrapperspb.Double(math.NaN()), WrappedFloat: wrapperspb.Float(float32(math.Inf(1))),
		WrappedInt64: wrapperspb.Int64(math.MinInt64), WrappedUint64: wrapperspb.UInt64(math.MaxUint64),
		WrappedInt32: wrapperspb.Int32(-1), WrappedUint32: wrapperspb.UInt32(2),
		WrappedBool: wrapperspb.Bool(false), WrappedString: wrapperspb.String("<>&"),
		WrappedBytes: wrapperspb.Bytes([]byte{255}),
		Children:     []*testpb.Scalars_Nested{{Name: "first"}, {}},
	}
}

func TestLegacyJSONMatchesProtoJSON(t *testing.T) {
	compareLegacyJSON(t, &testpb.Scalars{})
	compareLegacyJSON(t, (*testpb.Scalars)(nil))
	fixture := legacyJSONFixture()
	compareLegacyJSON(t, fixture)
	fixture.Choice = &testpb.Scalars_ChoiceNested{ChoiceNested: &testpb.Scalars_Nested{Name: "oneof"}}
	fixture.State = testpb.State(123456)
	compareLegacyJSON(t, fixture)
	for _, number := range []float64{0, math.Copysign(0, -1), 1e-7, 1e-6, 1e20, 1e21, math.SmallestNonzeroFloat64, math.MaxFloat64, math.NaN(), math.Inf(1), math.Inf(-1)} {
		compareLegacyJSON(t, &testpb.Scalars{DoubleValue: number, FloatValue: float32(number)})
	}
	compareLegacyJSON(t, &testpb.Scalars{StringValue: string([]byte{255})})
	compareLegacyJSON(t, &testpb.Scalars{Entries: map[string]*testpb.Scalars_Nested{string([]byte{255}): {}}})
	compareLegacyJSON(t, &testpb.Scalars{Timestamp: &timestamppb.Timestamp{Seconds: math.MaxInt64}})
	compareLegacyJSON(t, &testpb.Scalars{FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"invalid_Path"}}})
}

func TestLegacyJSONProto2Fallback(t *testing.T) {
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: proto.String("legacy_codec_test.proto"), Package: proto.String("legacy.codec.test"),
		Syntax: proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Required"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("name"), Number: proto.Int32(1),
				Label: descriptorpb.FieldDescriptorProto_LABEL_REQUIRED.Enum(),
				Type:  descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{Start: proto.Int32(100), End: proto.Int32(101)}},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name: proto.String("extra"), Number: proto.Int32(100),
			Extendee: proto.String(".legacy.codec.test.Required"),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	message := dynamicpb.NewMessage(file.Messages().Get(0))
	compareLegacyJSON(t, message)
	message.Set(message.Descriptor().Fields().Get(0), protoreflect.ValueOfString("required"))
	extension := dynamicpb.NewExtensionType(file.Extensions().Get(0))
	message.Set(extension.TypeDescriptor(), protoreflect.ValueOfString("extension"))
	compareLegacyJSON(t, message)
	prefix := []byte("prefix:")
	want, err := legacyMarshal.MarshalAppend(bytes.Clone(prefix), message)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (legacyJSONCodec{}).MarshalAppend(bytes.Clone(prefix), message)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("MarshalAppend: got %s (%v), want %s", got, err, want)
	}
}

func TestLegacyJSONInvalidFieldNameFallback(t *testing.T) {
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: proto.String("legacy_invalid_name_test.proto"), Package: proto.String("legacy.codec.test"),
		Syntax: proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("InvalidName"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("name"), JsonName: proto.String(string([]byte{255})), Number: proto.Int32(1),
				Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:  descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	compareLegacyJSON(t, dynamicpb.NewMessage(file.Messages().Get(0)))
}

func TestLegacyJSONRejectsNonJSONWhitespace(t *testing.T) {
	for _, data := range []string{"\v{}", "\f{}", "\u00a0{}", "{}\v", "{}\f", "{}\u00a0"} {
		t.Run(data, func(t *testing.T) {
			got, want := legacyJSONFixture(), legacyJSONFixture()
			gotErr := (legacyJSONCodec{}).Unmarshal([]byte(data), got)
			wantErr := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(data), want)
			if gotErr == nil || wantErr == nil || gotErr.Error() != wantErr.Error() {
				t.Fatalf("error mismatch: got %v, want %v", gotErr, wantErr)
			}
			if !proto.Equal(got, want) {
				t.Fatal("decoded messages differ")
			}
		})
	}
}

// Arbitrary wire messages exercise valid and invalid scalar values, duplicate
// fields, unknown enums, oneofs, numeric maps, nested WKTs and wrappers without
// constraining the fuzzer to JSON already accepted by protojson.
func FuzzLegacyJSONMatchesProtoJSON(f *testing.F) {
	for _, message := range []*testpb.Scalars{{}, legacyJSONFixture(), {State: testpb.State(-1)}, {Choice: &testpb.Scalars_ChoiceNested{ChoiceNested: &testpb.Scalars_Nested{}}}} {
		data, err := proto.Marshal(message)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		message := new(testpb.Scalars)
		if err := proto.Unmarshal(data, message); err != nil {
			return
		}
		compareLegacyJSON(t, message)
	})
}

func FuzzLegacyJSONDecodeMatchesProtoJSON(f *testing.F) {
	for _, data := range []string{`{}`, ` { } `, "\v{}", "{}\f", "\u00a0{}", `{"int64Value":"9223372036854775807","unknown":true}`, `{"state":123,"choiceText":""}`, `{"labels":{"10":"ten","-1":"negative"}}`, `{"timestamp":"2026-01-01T00:00:00Z","wrappedInt64":"42"}`, `{"int32Value":1,"int32_value":2}`, `null`} {
		f.Add([]byte(data))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		got, want := legacyJSONFixture(), legacyJSONFixture()
		gotErr := (legacyJSONCodec{}).Unmarshal(data, got)
		wantErr := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, want)
		if (gotErr == nil) != (wantErr == nil) || gotErr != nil && gotErr.Error() != wantErr.Error() {
			t.Fatalf("decode error mismatch: got %v, want %v", gotErr, wantErr)
		}
		if !proto.Equal(got, want) {
			t.Fatal("decoded messages differ")
		}
	})
}

func TestLegacyJSONCompiledPlan(t *testing.T) {
	message := &testpb.BenchmarkMenu{}
	if legacyPlan(reflect.TypeOf(message), message.ProtoReflect().Descriptor()) == nil {
		t.Fatal("benchmark message must use the compiled encoder")
	}
	random := rand.New(rand.NewPCG(1, 2))
	for range 500 {
		text := make([]rune, random.IntN(128))
		alphabet := []rune("ASCII words <>&\"\\\n\té日\u2028🙂")
		for i := range text {
			text[i] = alphabet[random.IntN(len(alphabet))]
		}
		product := &testpb.BenchmarkProduct{
			Id: string(text), Name: string(text), Description: string(text),
			Price: int32(random.Uint32()), Available: random.IntN(2) == 0,
			Tags:    []string{string(text), ""},
			Options: []*testpb.BenchmarkOption{nil, {Id: string(text), Price: int32(random.Uint32())}},
		}
		message.Products = []*testpb.BenchmarkProduct{nil, product, {}}
		compareLegacyJSON(t, message)
	}
	for offset := range 17 {
		for _, suffix := range []string{"é", "日", "🙂", "\u2028", "é\"\\\n"} {
			text := string(bytes.Repeat([]byte("a"), offset)) + suffix
			compareLegacyJSON(t, &testpb.BenchmarkOption{Name: text})
		}
	}
	// Exercise every byte at each position surrounding a word-scan boundary.
	for length := range 25 {
		for b := range 256 {
			text := bytes.Repeat([]byte("a"), length)
			for i := range text {
				text[i] = byte(b)
				compareLegacyJSON(t, &testpb.BenchmarkOption{Name: string(text)})
				text[i] = 'a'
			}
		}
	}
}

func FuzzLegacyJSONCompiledPlan(f *testing.F) {
	for _, text := range []string{"", "plain ASCII text across word boundaries", "aaaaaaaé\u2028", "before\\after\n", string([]byte{255})} {
		f.Add([]byte(text), int32(42))
	}
	f.Fuzz(func(t *testing.T, text []byte, price int32) {
		compareLegacyJSON(t, &testpb.BenchmarkMenu{Products: []*testpb.BenchmarkProduct{{
			Id: string(text), Name: string(text), Description: string(text), Price: price,
			Tags: []string{string(text)}, Options: []*testpb.BenchmarkOption{nil, {Name: string(text), Price: price}},
		}}})
	})
}

func TestLegacyJSONPlanScalarKinds(t *testing.T) {
	message := legacyJSONFixture()
	reflected := message.ProtoReflect()
	goValue := reflect.ValueOf(message).Elem()
	for i := range reflected.Descriptor().Fields().Len() {
		descriptor := reflected.Descriptor().Fields().Get(i)
		if descriptor.IsList() || descriptor.IsMap() || descriptor.ContainingOneof() != nil {
			continue
		}
		// Field tags link the native Go representation to the same descriptor
		// used by the already differential-tested reflection encoder.
		for j := range goValue.NumField() {
			goField := goValue.Type().Field(j)
			if !goField.IsExported() || goField.Tag.Get("protobuf") == "" {
				continue
			}
			parts := strings.Split(goField.Tag.Get("protobuf"), ",")
			if parts[1] != strconv.Itoa(int(descriptor.Number())) {
				continue
			}
			field := legacyFieldPlan{descriptor: descriptor, kind: descriptor.Kind()}
			got, gotErr := field.append(nil, goValue.Field(j), 0)
			want, wantErr := appendLegacyScalar(nil, reflected.Get(descriptor), descriptor, 0)
			if (gotErr == nil) != (wantErr == nil) || !bytes.Equal(got, want) {
				t.Fatalf("%s: got %s (%v), want %s (%v)", descriptor.FullName(), got, gotErr, want, wantErr)
			}
		}
	}
}

func TestLegacyJSONPlanBufferSizing(t *testing.T) {
	codec := legacyJSONCodec{}
	for _, length := range []int{1000, 10, 10000, 0} {
		message := &testpb.BenchmarkOption{Name: strings.Repeat("a", length)}
		for _, capacity := range []int{0, 32, 20000} {
			prefix := append(make([]byte, 0, capacity), "prefix:"...)
			want, err := legacyMarshal.MarshalAppend(bytes.Clone(prefix), message)
			if err != nil {
				t.Fatal(err)
			}
			got, err := codec.MarshalAppend(prefix, message)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("length %d capacity %d: got %s (%v), want %s", length, capacity, got, err, want)
			}
		}
	}
	message := &testpb.BenchmarkOption{Name: strings.Repeat("a", 10000)}
	if _, err := codec.MarshalAppend(nil, message); err != nil {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if _, err := codec.MarshalAppend(nil, message); err != nil {
			t.Fatal(err)
		}
	}); allocations != 1 {
		t.Fatalf("warmed-up encoder allocated %v times, want one response buffer", allocations)
	}
	message.Name = strings.Repeat("x", legacySizeHintLimit*2)
	if _, err := codec.MarshalAppend(nil, message); err != nil {
		t.Fatal(err)
	}
	plan := legacyPlan(reflect.TypeOf(message), message.ProtoReflect().Descriptor())
	if size := plan.encodedSize.Load(); size != legacySizeHintLimit {
		t.Fatalf("outlier capacity hint = %d, want %d", size, legacySizeHintLimit)
	}
}
