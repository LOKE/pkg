package lokerpc

import (
	"encoding/json"
	"flag"
	"reflect"
	"testing"
	"time"

	jtd "github.com/jsontypedef/json-typedef-go"
)

type testCurrency string

var testCurrencies = NewEnumSet[testCurrency]()

var (
	testCurrencyAUD = testCurrencies.Add("AUD")
	testCurrencyGBP = testCurrencies.Add("GBP")
	testCurrencyNZD = testCurrencies.Add("NZD")
)

func (testCurrency) EnumValues() []string { return testCurrencies.Values() }

// regColor is registered via NewEnumSet and has NO EnumValues method — it
// exercises the registry-only detection path.
type regColor string

var regColors = NewEnumSet[regColor]()

var (
	regColorRed  = regColors.Add("red")
	regColorBlue = regColors.Add("blue")
)

// ifaceShade uses the EnumProvider interface only (no NewEnumSet) — it exercises
// the interface fallback path.
type ifaceShade string

const (
	ifaceShadeLight ifaceShade = "light"
	ifaceShadeDark  ifaceShade = "dark"
)

func (ifaceShade) EnumValues() []string { return Enum(ifaceShadeLight, ifaceShadeDark) }

func TestEnumHelper(t *testing.T) {
	got := Enum(testCurrencyAUD, testCurrencyGBP, testCurrencyNZD)
	want := []string{"AUD", "GBP", "NZD"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Enum() = %v, want %v", got, want)
	}
}

func TestEnumSet(t *testing.T) {
	type color string
	set := NewEnumSet[color]()
	red := set.Add("red")
	blue := set.Add("blue")

	if red != "red" || blue != "blue" {
		t.Fatalf("Add should return the value it recorded: red=%q blue=%q", red, blue)
	}

	want := []string{"red", "blue"}
	if got := set.Values(); !reflect.DeepEqual(got, want) {
		t.Errorf("Values() = %v, want %v", got, want)
	}

	// Values must return a fresh slice so callers can't corrupt the set.
	got := set.Values()
	got[0] = "mutated"
	if set.Values()[0] != "red" {
		t.Error("Values() should return a defensive copy")
	}
}

func TestEnumSetRegistersWithoutMethod(t *testing.T) {
	// Add returns the typed value, so the declared names are usable constants.
	if regColorRed != "red" || regColorBlue != "blue" {
		t.Fatalf("Add should return the typed value: red=%q blue=%q", regColorRed, regColorBlue)
	}

	// regColor has no EnumValues method; schema generation must still detect it
	// via the registry that NewEnumSet populated.
	tdefs := map[reflect.Type]*NamedSchema{}
	got := TypeSchema(reflect.TypeOf(struct {
		Color regColor `json:"color"`
	}{}), tdefs)

	ref := got.Properties["color"].Ref
	if ref == nil || *ref != "regColor" {
		t.Fatalf("expected color to ref the regColor enum, got %+v", got.Properties["color"])
	}
	if defs := TypeDefs(tdefs); !reflect.DeepEqual(defs["regColor"].Enum, []string{"red", "blue"}) {
		t.Errorf("regColor enum = %v, want [red blue]", defs["regColor"].Enum)
	}
}

func TestEnumValuesFor(t *testing.T) {
	// Registry-backed type (declared via NewEnumSet, no method).
	if vals, ok := EnumValuesFor(reflect.TypeFor[regColor]()); !ok || !reflect.DeepEqual(vals, []string{"red", "blue"}) {
		t.Errorf("EnumValuesFor(regColor) = %v, %v; want [red blue], true", vals, ok)
	}
	// Interface-backed type (EnumProvider, no NewEnumSet).
	if vals, ok := EnumValuesFor(reflect.TypeFor[ifaceShade]()); !ok || !reflect.DeepEqual(vals, []string{"light", "dark"}) {
		t.Errorf("EnumValuesFor(ifaceShade) = %v, %v; want [light dark], true", vals, ok)
	}
	// A plain string is not a known enum.
	if _, ok := EnumValuesFor(reflect.TypeFor[string]()); ok {
		t.Error("plain string should not be a known enum")
	}
}

func TestEnumProviderIsValueReceiver(t *testing.T) {
	// EnumProvider must be satisfied by the value (not the pointer): schema
	// generation relies on reflect.Zero(t).Interface().(EnumProvider).
	var _ EnumProvider = testCurrency("")
	if _, ok := reflect.Zero(reflect.TypeOf(testCurrency(""))).Interface().(EnumProvider); !ok {
		t.Fatal("testCurrency value does not satisfy EnumProvider (pointer receiver?)")
	}
}

func TestAuditEnumValidatorTags(t *testing.T) {
	type good struct {
		Currency testCurrency `json:"currency" validate:"enum"`
	}
	if errs := AuditEnumValidatorTags(reflect.TypeOf(good{})); len(errs) != 0 {
		t.Errorf("tagged enum field should pass audit, got %v", errs)
	}

	type goodOptional struct {
		Currency *testCurrency `json:"currency" validate:"omitempty,enum"`
	}
	if errs := AuditEnumValidatorTags(reflect.TypeOf(goodOptional{})); len(errs) != 0 {
		t.Errorf("omitempty,enum field should pass audit, got %v", errs)
	}

	type bad struct {
		Currency testCurrency `json:"currency"`
	}
	if errs := AuditEnumValidatorTags(reflect.TypeOf(bad{})); len(errs) != 1 {
		t.Errorf("untagged enum field should yield 1 audit error, got %v", errs)
	}
}

type jsonMarshaler struct{}

func (jsonMarshaler) MarshalJSON() ([]byte, error) {
	return nil, nil
}

type textMarshaler struct{}

func (textMarshaler) MarshalText() ([]byte, error) {
	return nil, nil
}

func TestTypeSchema(t *testing.T) {
	type NamedStruct struct {
		Foo  string `json:"foo"`
		nope string
	}

	// Name to conflict with flag.Flag
	// Just chose it because its small
	type Flag struct {
		Foo string `json:"foo"`
	}

	type RecusiveStruct struct {
		Name   string          `json:"name"`
		Loopsy *RecusiveStruct `json:"loopsy"`
	}

	type args struct {
		t    reflect.Type
		defs map[reflect.Type]*NamedSchema
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "string",
			args: args{
				t: reflect.TypeOf(""),
			},
			want: `{"type":"string"}`,
		},
		{
			name: "slice of strings",
			args: args{
				t: reflect.TypeOf([]string{}),
			},
			want: `{
				"elements":{"type":"string"},
				"nullable": true
			}`,
		},
		{
			name: "map of ints",
			args: args{
				t: reflect.TypeOf(map[string]int32{}),
			},
			want: `{
				"values":{"type":"int32"},
				"nullable": true
			}`,
		},
		{
			name: "int 32",
			args: args{
				t: reflect.TypeOf(int32(0)),
			},
			want: `{"type":"int32"}`,
		},
		{
			name: "int",
			args: args{
				t: reflect.TypeOf(0),
			},
			want: `{"type":"int32"}`,
		},
		{
			name: "basic struct",
			args: args{
				t: reflect.TypeOf(struct {
					Foo string
					Bar *string
				}{}),
			},
			want: `{
				"properties": {
					"Foo": { "type": "string" },
					"Bar": { "type": "string", "nullable": true }
				}
			}`,
		},
		{
			name: "basic struct with json tag",
			args: args{
				t: reflect.TypeOf(struct {
					Foo string  `json:"foo"`
					Bar *string `json:"bar"`
					Baz string  `json:"baz,omitempty"`
					Taz *string `json:"taz,omitempty"`
				}{}),
			},
			want: `{
				"properties": {
					"foo": { "type": "string" },
					"bar": { "type": "string", "nullable": true }
				},
				"optionalProperties": {
					"baz": { "type": "string" },
					"taz": { "type": "string" }
				}
			}`,
		},
		{
			name: "empty struct",
			args: args{
				t: reflect.TypeOf(struct{}{}),
			},
			want: `{"properties": {}}`,
		},
		{
			name: "timestamp",
			args: args{
				t: reflect.TypeOf(struct {
					Foo time.Time
				}{}),
			},
			want: `{
				"properties": {
					"Foo": { "type": "timestamp" }
				}
			}`,
		},
		{
			name: "named structs",
			args: args{
				t: reflect.TypeOf(struct {
					Foo NamedStruct
					Bar *NamedStruct
				}{}),
			},
			want: `{
				"definitions": {
					"NamedStruct": {
						"properties": {
							"foo": { "type": "string" }
						}
					}
				},
				"properties": {
					"Foo": { "ref": "NamedStruct" },
					"Bar": { "ref": "NamedStruct", "nullable": true }
				}
			}`,
		},
		{
			name: "deal with name conflicts",
			args: args{
				t: reflect.TypeOf(struct {
					Foo NamedStruct
					Bar flag.Flag
					Baz Flag
				}{}),
			},
			want: `{
				"definitions": {
					"NamedStruct": {
						"properties": {
							"foo": { "type": "string" }
						}
					},
					"Flag": {
						"properties": {
							"Name": { "type": "string" },
							"DefValue": { "type": "string" },
							"Usage": { "type": "string" },
							"Value": {}
						}
					},
					"Flag2": {
						"properties": {
							"foo": { "type": "string" }
						}
					}
				},
				"properties": {
					"Foo": { "ref": "NamedStruct" },
					"Bar": { "ref": "Flag" },
					"Baz": { "ref": "Flag2" }
				}
			}`,
		},
		{
			name: "json marshaler",
			args: args{
				t: reflect.TypeOf(jsonMarshaler{}),
			},
			want: `{}`,
		},
		{
			name: "json marshaler",
			args: args{
				t: reflect.TypeOf(&jsonMarshaler{}),
			},
			// Maybe this should just be empty `{}` ?
			want: `{"nullable": true}`,
		},
		{
			name: "text marshaler",
			args: args{
				t: reflect.TypeOf(textMarshaler{}),
			},
			want: `{"type":"string"}`,
		},
		{
			name: "text marshaler",
			args: args{
				t: reflect.TypeOf(&textMarshaler{}),
			},
			want: `{"type":"string","nullable": true}`,
		},
		{
			name: "named string implementing EnumProvider becomes enum ref",
			args: args{
				t: reflect.TypeOf(struct {
					Currency testCurrency `json:"currency"`
				}{}),
			},
			want: `{
				"definitions": {
					"testCurrency": {"enum": ["AUD", "GBP", "NZD"]}
				},
				"properties": {
					"currency": {"ref": "testCurrency"}
				}
			}`,
		},
		{
			name: "pointer to EnumProvider type is nullable ref",
			args: args{
				t: reflect.TypeOf(struct {
					Currency *testCurrency `json:"currency"`
				}{}),
			},
			want: `{
				"definitions": {
					"testCurrency": {"enum": ["AUD", "GBP", "NZD"]}
				},
				"properties": {
					"currency": {"ref": "testCurrency", "nullable": true}
				}
			}`,
		},
		{
			name: "recusive type",
			args: args{
				t: reflect.TypeOf(&RecusiveStruct{}),
			},
			want: `{
				"definitions": {
					"RecusiveStruct": {
						"properties": {
							"loopsy": {
								"nullable": true,
								"ref": "RecusiveStruct"
							},
							"name": {
								"type": "string"
							}
						}
					}
				},
				"nullable": true,
				"ref": "RecusiveStruct"
			}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.defs == nil {
				tt.args.defs = map[reflect.Type]*NamedSchema{}
			}

			var want jtd.Schema

			err := json.Unmarshal([]byte(tt.want), &want)
			if err != nil {
				t.Fatal(err)
			}

			got := TypeSchema(tt.args.t, tt.args.defs)

			got.Definitions = TypeDefs(tt.args.defs)
			if len(got.Definitions) == 0 {
				got.Definitions = nil
			}

			if err := got.Validate(); err != nil {
				t.Errorf("Validate() error = %v", err)
			}

			if !reflect.DeepEqual(got, &want) {
				gotstr, _ := json.MarshalIndent(got, "", "  ")
				wantstr, _ := json.MarshalIndent(want, "", "  ")
				t.Errorf("TypeSchema() = %q, want %q", gotstr, wantstr)
			}
		})
	}
}
