package codegen

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/LOKE/pkg/lokerpc"
	jtd "github.com/jsontypedef/json-typedef-go"
)

func GenTypescriptType(schema jtd.Schema) string {
	var t string

	switch schema.Form() {
	case jtd.FormRef:
		t += capitalize(*schema.Ref)
	case jtd.FormType:
		switch schema.Type {
		case jtd.TypeString,
			jtd.TypeTimestamp:
			t += "string"
		case jtd.TypeInt8,
			jtd.TypeInt16,
			jtd.TypeInt32,
			jtd.TypeUint8,
			jtd.TypeUint16,
			jtd.TypeUint32,
			jtd.TypeFloat32,
			jtd.TypeFloat64:
			t += "number"
		case jtd.TypeBoolean:
			t += "boolean"
		}
	case jtd.FormElements:
		if schema.Elements.Nullable {
			t += "(" + GenTypescriptType(*schema.Elements) + ")[]"
		} else {
			t += GenTypescriptType(*schema.Elements) + "[]"
		}
	case jtd.FormValues:
		t += "Record<string, " + GenTypescriptType(*schema.Values) + ">"
	case jtd.FormProperties:
		t += "{\n"
		for _, k := range sortedKeys(schema.Properties) {
			t += "  " + quoteFieldNames(k) + ": " + GenTypescriptType(schema.Properties[k]) + ";\n"
		}
		for _, k := range sortedKeys(schema.OptionalProperties) {
			t += "  " + quoteFieldNames(k) + "?: " + GenTypescriptType(schema.OptionalProperties[k]) + ";\n"
		}
		t += "}"
	case jtd.FormDiscriminator:
		for _, k := range sortedKeys(schema.Mapping) {
			s := schema.Mapping[k]
			s.Properties = map[string]jtd.Schema{}
			for kk, v := range schema.Mapping[k].Properties {
				s.Properties[kk] = v
			}
			s.Properties[schema.Discriminator] = jtd.Schema{Enum: []string{k}}

			t += "\n| " + GenTypescriptType(s)
		}
	case jtd.FormEnum:
		for i, v := range schema.Enum {
			if i > 0 {
				t += " | "
			}
			b, err := json.Marshal(v)
			if err != nil {
				panic(err)
			}
			t += string(b)
		}
	case jtd.FormEmpty:
		// not sure if this is the best thing, but it'll work I guess
		t += "any"
	}

	if schema.Nullable {
		t += " | null"
	}

	return t
}

func GenTypescriptClient(w io.Writer, meta lokerpc.Meta) error {
	defOrder := normalise(&meta)

	b := bufio.NewWriter(w)

	b.WriteString("import { RPCContextClient } from \"@loke/http-rpc-client\";\n")
	b.WriteString("import { Context } from \"@loke/context\";\n")

	for _, k := range defOrder {
		b.WriteString("\n")
		fmt.Fprintf(b, "export type %s = %s;\n", capitalize(k), GenTypescriptType(meta.Definitions[k]))
	}

	b.WriteString("\n")
	tsDocComment(b, meta.Help, "")
	b.WriteString("export class " + pascalCase(meta.ServiceName) + "Service extends RPCContextClient {\n")
	b.WriteString("  constructor(baseUrl: string) {\n")
	b.WriteString("    super(baseUrl, \"" + meta.ServiceName + "\")\n")
	b.WriteString("  }\n")

	for _, v := range meta.Interfaces {
		reqType := "any"
		if v.RequestTypeDef != nil {
			reqType = GenTypescriptType(*v.RequestTypeDef)
		}

		resType := "any"
		if v.ResponseTypeDef != nil {
			if v.ResponseTypeDef.Metadata["void"] == true {
				resType = "void"
			} else {
				resType = GenTypescriptType(*v.ResponseTypeDef)
			}
		}

		tsDocComment(b, v.Help, "  ")
		b.WriteString("  " + v.MethodName + "(ctx: Context, req: " + reqType + "): Promise<" + resType + "> {\n")
		b.WriteString("    return this.request(ctx, \"" + v.MethodName + "\", req);\n  }\n")
	}

	b.WriteString("}\n")

	return b.Flush()
}

func normalise(meta *lokerpc.Meta) []string {
	var defOrder []string

	if meta.Definitions == nil {
		meta.Definitions = map[string]jtd.Schema{}
	}

	defOrder = append(defOrder, sortedKeys(meta.Definitions)...)

	for i, v := range meta.Interfaces {
		if v.RequestTypeDef != nil && v.RequestTypeDef.Ref == nil && v.RequestTypeDef.Form() != jtd.FormEmpty {
			name := capitalize(v.MethodName) + "Request"
			for {
				if _, ok := meta.Definitions[name]; !ok {
					break
				}
				name += "_"
			}
			meta.Definitions[name] = *v.RequestTypeDef
			meta.Interfaces[i].RequestTypeDef = &jtd.Schema{Ref: &name}
			defOrder = append(defOrder, name)
		}

		if v.ResponseTypeDef != nil && v.ResponseTypeDef.Ref == nil && v.ResponseTypeDef.Form() != jtd.FormEmpty {
			name := capitalize(v.MethodName) + "Response"
			for {
				if _, ok := meta.Definitions[name]; !ok {
					break
				}
				name += "_"
			}
			meta.Definitions[name] = *v.ResponseTypeDef
			meta.Interfaces[i].ResponseTypeDef = &jtd.Schema{Ref: &name}
			defOrder = append(defOrder, name)
		}
	}

	return defOrder
}

func tsDocComment(w io.Writer, text string, indent string) {
	lines := strings.Split(text, "\n")

	fmt.Fprintf(w, "%s/**\n", indent)
	for _, l := range lines {
		fmt.Fprintf(w, "%s * %s\n", indent, l)
	}
	fmt.Fprintf(w, "%s */\n", indent)
}
