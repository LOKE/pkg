package lokerpc

import (
	"math/rand/v2"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func compareLegacyDecode(t testing.TB, data []byte, initial proto.Message) {
	t.Helper()
	got, want := proto.Clone(initial), proto.Clone(initial)
	wantErr := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, want)
	gotErr := (legacyJSONCodec{}).Unmarshal(data, got)
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("%T decoding %q: got error %v, want %v", initial, data, gotErr, wantErr)
	}
	if wantErr != nil && gotErr.Error() != wantErr.Error() {
		t.Fatalf("%T decoding %q: got error %v, want %v", initial, data, gotErr, wantErr)
	}
	if !proto.Equal(got, want) {
		t.Fatalf("%T decoding %q: got %v, want %v", initial, data, got, want)
	}
}

func TestLegacyDecodeDifferential(t *testing.T) {
	inputs := []string{
		`{}`, ` { "id": "fresh", "price": 123, "available": true } `,
		`{"id":"é日本語","name":"","description":"ordinary","tags":["one","two"],"options":[{"id":"a","price":-2147483648},{"id":"b","price":"2147483647"}]}`,
		`{"price":"-0"}`, `{"price":-0}`, `{"price":2147483648}`, `{"price":-2147483649}`,
		`{"price":1.0}`, `{"price":1e2}`, `{"price":"1e2"}`, `{"price":"+1"}`,
		`{"price":01}`, `{"price":"01"}`, `{"price":1e}`, `{"price":--1}`, `{"price":+1}`,
		`{"price":true}`, `{"price":""}`, `{"price":1.5}`, `{"price":"1.5"}`,
		`{"id":null,"tags":null,"options":null}`, `{"tags":[] ,"options":[]}`,
		`{"tags":[null]}`, `{"options":[null]}`, `{"tags":["a",null,"b"]}`,
		`{"options":[{},{}]}`, `{"options":[{"id":"a"},{"id":"b"}]}`,
		`{"id":"first","id":"second"}`, `{"id":null,"id":"second"}`,
		`{"id":"first","id":null}`, `{"tags":[],"tags":[]}`,
		`{"id":"prefix","unknown":{"nested":[null,true,1,"x"]},"price":42}`,
		`{"id":"prefix","price":999999999999999999999999999999999}`,
		`{"id":"prefix","options":[{"id":"partial","price":"invalid"}]}`,
		`{"id":"\\\"\n\t\u00e9\ud83d\ude00"}`, `{"id":"\ud800"}`, `{"id":"\udc00"}`,
		`{"id":"\u0000"}`, `{"\u0069d":"escaped key"}`,
		`{"id":"a"} {}`, `{"id":"a"}null`, `{"id":"a",}`, `{"id":"a"`,
		`{"id":"a","tags":["x",]}`, `{"options":[{},]}`, `{"options":[{]}`,
		`{"available":truefalse}`, `{"id":nullnull}`, `{"price":123x}`,
		`null`, `[]`, `true`, `"text"`, ``, ` `,
		"{\"id\":\"\xff\"}", "{\"id\":\"raw\nnewline\"}",
		`{"unknown":"` + string([]byte{255}) + `"}`,
		`{"unknown":` + strings.Repeat(`[`, 101) + `0` + strings.Repeat(`]`, 101) + `}`,
		`{"unknown":` + strings.Repeat(`[`, 10001) + `0` + strings.Repeat(`]`, 10001) + `}`,
	}
	initial := &testpb.BenchmarkProduct{Id: "stale", Price: 9, Tags: []string{"stale"}, Options: []*testpb.BenchmarkOption{{Id: "stale"}}}
	for _, input := range inputs {
		compareLegacyDecode(t, []byte(input), initial)
	}
}

func TestLegacyDecodeFallbackSchemasAndAliases(t *testing.T) {
	for _, input := range []string{
		`{"int32Value":1,"int32_value":2}`,
		`{"stringValue":"a","string_value":"b"}`,
		`{"customName":"a","renamed":"b"}`,
		`{"int64Value":"9223372036854775807","uint64Value":"18446744073709551615"}`,
		`{"int64Value":9223372036854775807,"int32Value":"-2147483648"}`,
		`{"nested":{"name":"child"},"children":[{"name":"one"},{}]}`,
		`{"entries":{"x":{"name":"child"}},"optionalValue":"present"}`,
		`{"doubleValue":"NaN","floatValue":"Infinity","bytesValue":"AAE="}`,
		`{"state":"UNKNOWN_ENUM_NAME"}`, `{"state":123456}`, `{"state":null}`,
		`{"timestamp":"2020-01-01T00:00:00Z","wrappedInt64":"12"}`,
		`{"nested":null,"children":[null]}`,
	} {
		compareLegacyDecode(t, []byte(input), legacyJSONFixture())
	}
	for _, input := range []string{
		`{"rightValue":1}`, `{"right_value":"2"}`,
		`{"rightValue":1,"right_value":2}`, `{"right_value":1,"rightValue":2}`,
		`{"rightValue":null,"right_value":2}`, `{"right_value":null,"rightValue":null}`,
	} {
		compareLegacyDecode(t, []byte(input), &testpb.Right_Item{RightValue: 9})
	}
	for _, input := range []string{`"2020-01-01T00:00:00Z"`, `null`, `{}`, `"invalid"`} {
		compareLegacyDecode(t, []byte(input), &timestamppb.Timestamp{Seconds: 1})
	}
	for _, input := range []string{`{"x":[null,1,true,"é"]}`, `null`, `[]`, `{"x":1,"x":2}`} {
		compareLegacyDecode(t, []byte(input), &structpb.Struct{})
	}
}

func TestLegacyDecodeListSizingBounds(t *testing.T) {
	for _, test := range []struct {
		input    string
		minBytes int
		want     int
		ok       bool
	}{
		{`{},{}]`, 3, 2, true},
		{`0,1]`, 2, 2, true},
		{`"",""]`, 2, 2, true},
		{`,,,,,]`, 2, 6, false},
		{`,,,,,]`, 3, 6, false},
	} {
		scanner := legacyJSONDecoder{data: []byte(test.input)}
		count, ok := scanner.listSize(test.minBytes)
		if count != test.want || ok != test.ok {
			t.Fatalf("listSize(%q, %d) = (%d, %v), want (%d, %v)", test.input, test.minBytes, count, ok, test.want, test.ok)
		}
	}
}

func TestLegacyDecodeNullListsReplay(t *testing.T) {
	// Value and NullValue have special null semantics in protojson, even
	// when repeated. An unsupported child plan must not hide that distinction.
	descriptors := []proto.Message{&structpb.ListValue{}, &structpb.Value{}}
	for _, message := range descriptors {
		descriptor := message.ProtoReflect().Descriptor().Fields().Get(0)
		plan := &legacyMessagePlan{fields: []legacyFieldPlan{{
			index: 0, descriptor: descriptor, kind: descriptor.Kind(), list: true,
		}}}
		value := reflect.ValueOf(&struct{ Values []int }{}).Elem()
		data := []byte(`{"` + descriptor.JSONName() + `":null}`)
		scanner := legacyJSONDecoder{data: data}
		if scanner.message(plan, value, 0) {
			t.Fatalf("null list %s bypassed reference decoding", descriptor.FullName())
		}
	}
}

func TestLegacyDecodeGeneratedFastPath(t *testing.T) {
	data := []byte(`{"products":[{"id":"one","price":"-10","available":true,"tags":["a","b"],"options":[{"id":"first","price":1},{"id":"second","price":2}]},{"id":"two","price":0,"available":false,"tags":[],"options":[]}]}`)
	message := &testpb.BenchmarkMenu{}
	plan := legacyPlan(reflect.TypeOf(message), message.ProtoReflect().Descriptor())
	if plan == nil {
		t.Fatal("benchmark schema has no compiled plan")
	}
	scanner := legacyJSONDecoder{data: data}
	if !scanner.message(plan, reflect.ValueOf(message).Elem(), 0) || scanner.space() != len(data) {
		t.Fatal("ordinary generated menu did not use the fast path")
	}
	want := &testpb.BenchmarkMenu{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, want); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(message, want) {
		t.Fatalf("got %v, want %v", message, want)
	}
	message.Products[0].Options[0].Id = "changed"
	if message.Products[0].Options[1].Id != "second" || message.Products[1].Id != "two" {
		t.Fatal("contiguous message storage aliases distinct elements")
	}
	for i := range data {
		data[i] = 0
	}
	if message.Products[0].Id != "one" {
		t.Fatal("decoded strings retain input buffer")
	}
}

func TestLegacyDecodeProperty(t *testing.T) {
	random := rand.New(rand.NewPCG(17, 29))
	for range 200 {
		message := &testpb.BenchmarkMenu{}
		for range random.IntN(8) {
			product := &testpb.BenchmarkProduct{
				Id: string(rune(random.IntN(0x80))), Name: "日本語", Description: "line\nquoted\"",
				Price: int32(random.Uint32()), Available: random.IntN(2) == 0,
			}
			if random.IntN(2) == 0 {
				product.Id, product.Description = "plain", "ordinary description"
			}
			for range random.IntN(5) {
				product.Tags = append(product.Tags, "tag")
				product.Options = append(product.Options, &testpb.BenchmarkOption{Id: "option", Price: int32(random.Uint32())})
			}
			message.Products = append(message.Products, product)
		}
		data, err := (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		initial := &testpb.BenchmarkMenu{Products: []*testpb.BenchmarkProduct{{Id: "stale"}}}
		compareLegacyDecode(t, data, initial)
		if len(data) > 2 {
			mutated := append([]byte(nil), data...)
			mutated[random.IntN(len(mutated))] = byte(random.IntN(256))
			compareLegacyDecode(t, mutated, initial)
			compareLegacyDecode(t, data[:random.IntN(len(data))], initial)
		}
	}
}

func FuzzLegacyDecode(f *testing.F) {
	for _, input := range []string{
		`{}`, `{"products":[{"id":"one","price":"42","tags":["a"],"options":[{"price":-1}]}]}`,
		`{"products":[null]}`, `{"id":"one","id":"two"}`, `{"price":01}`,
		`{"id":"\ud800"}`, `{"int32Value":1,"int32_value":2}`, `{"unknown":[{}]}`,
	} {
		f.Add([]byte(input))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		compareLegacyDecode(t, data, &testpb.BenchmarkMenu{Products: []*testpb.BenchmarkProduct{{Id: "stale"}}})
		compareLegacyDecode(t, data, &testpb.BenchmarkProduct{Id: "stale", Price: 1})
		compareLegacyDecode(t, data, &testpb.Scalars{StringValue: "stale"})
	})
}

func TestLegacyDecodePlanScalarKinds(t *testing.T) {
	type input struct {
		token string
		fast  bool
	}
	message := &testpb.Scalars{}
	goType := reflect.TypeOf(message).Elem()
	fields := message.ProtoReflect().Descriptor().Fields()
	for i := range fields.Len() {
		descriptor := fields.Get(i)
		if descriptor.IsList() || descriptor.IsMap() || descriptor.HasPresence() || descriptor.Kind() == protoreflect.MessageKind {
			continue
		}
		t.Run(string(descriptor.Name()), func(t *testing.T) {
			index := -1
			for j := range goType.NumField() {
				field := goType.Field(j)
				parts := strings.Split(field.Tag.Get("protobuf"), ",")
				if field.IsExported() && len(parts) > 1 && parts[1] == strconv.Itoa(int(descriptor.Number())) {
					index = j
					break
				}
			}
			if index < 0 {
				t.Fatalf("no native Go field for %s", descriptor.FullName())
			}
			plan := &legacyMessagePlan{fields: []legacyFieldPlan{{
				index: index, descriptor: descriptor, kind: descriptor.Kind(),
			}}}
			var inputs []input
			var boundaries []string
			switch descriptor.Kind() {
			case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
				boundaries = []string{"-2147483648", "-1", "-0", "0", "1", "2147483647"}
				inputs = append(inputs, input{token: `2147483648`}, input{token: `-2147483649`})
			case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
				boundaries = []string{"-9223372036854775808", "-1", "-0", "0", "1", "9223372036854775807"}
				inputs = append(inputs, input{token: `9223372036854775808`}, input{token: `-9223372036854775809`})
			case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
				boundaries = []string{"0", "1", "4294967295"}
				inputs = append(inputs, input{token: `-1`}, input{token: `4294967296`})
			case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
				boundaries = []string{"0", "1", "18446744073709551615"}
				inputs = append(inputs, input{token: `-1`}, input{token: `18446744073709551616`})
			case protoreflect.BoolKind:
				inputs = append(inputs, input{token: `true`, fast: true}, input{token: `false`, fast: true},
					input{token: `"true"`}, input{token: `1`}, input{token: `truefalse`})
			case protoreflect.StringKind:
				inputs = append(inputs, input{token: `""`, fast: true}, input{token: `"é日本語"`, fast: true},
					input{token: `"escaped\n\u00e9"`}, input{token: `"\ud800"`}, input{token: `1`},
					input{token: "\"" + string([]byte{255}) + "\""})
			case protoreflect.FloatKind, protoreflect.DoubleKind:
				for _, token := range []string{`0`, `-0`, `1.25`, `1e-20`, `"1.25"`, `"NaN"`, `"Infinity"`, `"-Infinity"`, `1e400`, `"invalid"`} {
					inputs = append(inputs, input{token: token})
				}
			case protoreflect.BytesKind:
				for _, token := range []string{`""`, `"AAE="`, `"AAE"`, `"_w=="`, `"/w=="`, `"%%%"`, `1`} {
					inputs = append(inputs, input{token: token})
				}
			case protoreflect.EnumKind:
				for _, token := range []string{`"STATE_READY"`, `"STATE_UNSPECIFIED"`, `"UNKNOWN_ENUM_NAME"`, `0`, `1`, `123456`, `-1`, `"1"`, `1.5`} {
					inputs = append(inputs, input{token: token})
				}
			default:
				t.Fatalf("uncovered scalar kind %s", descriptor.Kind())
			}
			for _, token := range boundaries {
				inputs = append(inputs, input{token: token, fast: true}, input{token: `"` + token + `"`, fast: true})
			}
			if len(boundaries) > 0 {
				for _, token := range []string{`1.0`, `1e2`, `"1e2"`, `"01"`, `01`, `"+1"`, `" 1 "`, `1.5`, `"1.5"`, `true`, `""`, `--1`} {
					inputs = append(inputs, input{token: token})
				}
			}
			inputs = append(inputs, input{token: `null`, fast: true})
			for _, test := range inputs {
				data := []byte(`{"` + descriptor.JSONName() + `":` + test.token + `}`)
				want, got := &testpb.Scalars{}, &testpb.Scalars{}
				wantErr := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, want)
				scanner := legacyJSONDecoder{data: data}
				fast := scanner.message(plan, reflect.ValueOf(got).Elem(), 0) && scanner.space() == len(data)
				if test.fast && !fast {
					t.Fatalf("canonical scalar %s unexpectedly replayed", data)
				}
				if fast && (wantErr != nil || !proto.Equal(got, want)) {
					t.Fatalf("fast decode %s: got %v, reference %v (error %v)", data, got, want, wantErr)
				}
				// Check replay acceptance, exact errors and partial messages too.
				compareLegacyDecode(t, data, &testpb.Scalars{StringValue: "stale"})
			}
		})
	}
}
