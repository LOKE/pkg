package codegen

import (
	"bufio"
	"encoding/json"
	"io"

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
		t += GenTypescriptType(*schema.Elements) + "[]"
	case jtd.FormValues:
		t += "Record<string, " + GenTypescriptType(*schema.Values) + ">"
	case jtd.FormProperties:
		t += "{\n"
		for k, v := range schema.Properties {
			t += "  " + k + ": " + GenTypescriptType(v) + ";\n"
		}
		for k, v := range schema.OptionalProperties {
			t += "  " + k + "?: " + GenTypescriptType(v) + ";\n"
		}
		t += "}"
	case jtd.FormDiscriminator:
		panic("discriminator not supported")
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
	b := bufio.NewWriter(w)

	b.WriteString("import { RPCClient } from \"@loke/http-rpc-client\";\n")

	for k, v := range meta.Definitions {
		b.WriteString("\n")
		b.WriteString("export type " + capitalize(k) + " = " + GenTypescriptType(v) + ";\n")
	}

	for _, v := range meta.Interfaces {
		if v.RequestTypeDef != nil {
			b.WriteString("\n")
			b.WriteString("export type " + capitalize(v.MethodName) + "Request = " + GenTypescriptType(*v.RequestTypeDef) + ";\n")
		}
		if v.ResponseTypeDef != nil {
			b.WriteString("\n")
			b.WriteString("export type " + capitalize(v.MethodName) + "Response = " + GenTypescriptType(*v.ResponseTypeDef) + ";\n")
		}
	}

	b.WriteString("\n")
	b.WriteString("export class " + capitalize(meta.ServiceName) + "Service extends RPCClient {\n")
	b.WriteString("  constructor(baseUrl: string) {\n")
	b.WriteString("    super(baseUrl, \"" + meta.ServiceName + "\")\n")
	b.WriteString("  }\n")

	for _, v := range meta.Interfaces {
		reqType := "any"
		if v.RequestTypeDef != nil {
			reqType = capitalize(v.MethodName) + "Request"

		}

		resType := "any"
		if v.ResponseTypeDef != nil {
			resType = capitalize(v.MethodName) + "Response"
		}

		b.WriteString("  " + v.MethodName + "(req: " + reqType + "): Promise<" + resType + "> {\n")
		b.WriteString("    return this.request(\"" + v.MethodName + "\", req);\n  }\n")
	}

	b.WriteString("}\n")

	return b.Flush()
}
