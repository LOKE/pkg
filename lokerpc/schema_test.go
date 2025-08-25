package lokerpc

import (
	"encoding/json"
	"flag"
	"reflect"
	"testing"
	"time"

	jtd "github.com/jsontypedef/json-typedef-go"
)

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
		Foo string `json:"foo"`
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
				t.Errorf("TypeSchema() = %s, want %s", gotstr, wantstr)
			}
		})
	}
}
