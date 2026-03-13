package codegen

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/LOKE/pkg/lokerpc"
	jtd "github.com/jsontypedef/json-typedef-go"
)

func GenGoType(schema jtd.Schema) string {
	var t string

	for k, v := range schema.Definitions {
		t += "\n"
		t += "type " + goFieldName(k) + " " + GenGoType(v) + "\n"
	}

	switch schema.Form() {
	case jtd.FormRef:
		t += goFieldName(*schema.Ref)
	case jtd.FormType:
		switch schema.Type {
		case jtd.TypeString:
			t += "string"
		case jtd.TypeTimestamp:
			t += "time.Time"
		case jtd.TypeInt8:
			t += "int8"
		case jtd.TypeInt16:
			t += "int16"
		case jtd.TypeInt32:
			t += "int32"
		case jtd.TypeUint8:
			t += "uint8"
		case jtd.TypeUint16:
			t += "uint16"
		case jtd.TypeUint32:
			t += "uint32"
		case jtd.TypeFloat32:
			t += "float32"
		case jtd.TypeFloat64:
			t += "float64"
		case jtd.TypeBoolean:
			t += "bool"
		}
	case jtd.FormElements:
		t += "[]" + GenGoType(*schema.Elements)
	case jtd.FormValues:
		t += "map[string]" + GenGoType(*schema.Values)
	case jtd.FormProperties:
		t += "struct {\n"
		for _, k := range sortedKeys(schema.Properties) {
			t += "\t" + goFieldName(k) + " " + GenGoType(schema.Properties[k]) + "`json:\"" + k + "\"`\n"
		}
		for _, k := range sortedKeys(schema.OptionalProperties) {
			t += "\t" + goFieldName(k) + " " + GenGoType(schema.OptionalProperties[k]) + "`json:\"" + k + ",omitempty\"`\n"
		}
		t += "}"
	case jtd.FormDiscriminator:
		panic("discriminator not supported")
	case jtd.FormEnum:
		// Could do more here, but this is good enough for now
		t += "string"
	case jtd.FormEmpty:
		// not sure if this is the best thing, but it'll work I guess
		t += "any"
	}

	if schema.Nullable {
		t = "*" + t
	}

	return t
}

func GenGoClient(w io.Writer, meta lokerpc.Meta) error {
	defOrder := normalise(&meta)

	b := bufio.NewWriter(w)

	fmt.Fprintf(b, "package %s\n", strings.ToLower(strings.ReplaceAll(meta.ServiceName, "-", "")))
	b.WriteString("\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n")
	if metaUsesTimestamp(meta) {
		b.WriteString("\t\"time\"\n")
	}
	b.WriteString("\n")
	b.WriteString("\t\"github.com/LOKE/pkg/lokerpc\"\n")
	b.WriteString(")\n")

	for _, k := range defOrder {
		b.WriteString("\n")
		fmt.Fprintf(b, "type %s %s;\n", goFieldName(k), GenGoType(meta.Definitions[k]))
	}

	// Service interface
	b.WriteString("\n")
	// goDocComment(b, meta.Help, "")
	b.WriteString("type " + goFieldName(meta.ServiceName) + "Service interface {\n")
	for _, v := range meta.Interfaces {
		reqType := "any"
		if v.RequestTypeDef != nil {
			reqType = GenGoType(*v.RequestTypeDef)
		}

		resType := "any"
		if v.ResponseTypeDef != nil {
			if v.ResponseTypeDef.Metadata["void"] == true {
				resType = "struct{}"
			} else {
				resType = GenGoType(*v.ResponseTypeDef)

				if !strings.HasPrefix(resType, "[]") && !strings.HasPrefix(resType, "map[") && !strings.HasPrefix(resType, "*") {
					resType = "*" + resType
				}
			}
		}

		// goDocComment(b, v.Help, "\t")
		fmt.Fprintf(b, "\t%s(context.Context, %s) (%s, error)\n", goFieldName(v.MethodName), reqType, resType)
	}
	b.WriteString("}\n")

	// Service client implementation
	b.WriteString("\n")
	// goDocComment(b, meta.Help, "")
	b.WriteString("type " + goFieldName(meta.ServiceName) + "RPCClient struct{\nlokerpc.Client}\n\n")
	for _, v := range meta.Interfaces {
		reqType := "any"
		if v.RequestTypeDef != nil {
			reqType = GenGoType(*v.RequestTypeDef)
		}

		resType := "any"
		if v.ResponseTypeDef != nil {
			if v.ResponseTypeDef.Metadata["void"] == true {
				resType = "struct{}"
			} else {
				resType = GenGoType(*v.ResponseTypeDef)

				if !strings.HasPrefix(resType, "[]") && !strings.HasPrefix(resType, "map[") && !strings.HasPrefix(resType, "*") {
					resType = "*" + resType
				}
			}
		}

		varType := resType
		if varType != "any" && strings.HasPrefix(varType, "*") {
			varType = varType[1:]
		}

		// goDocComment(b, v.Help, "\t")
		fmt.Fprintf(b, "func (c %sRPCClient) %s(ctx context.Context, req %s) (%s, error) {\n", goFieldName(meta.ServiceName), goFieldName(v.MethodName), reqType, resType)
		fmt.Fprintf(b, "\tvar res %s\n", varType)
		fmt.Fprintf(b, "\terr := c.DoRequest(ctx, \"%s\", req, &res)\n", v.MethodName)
		fmt.Fprintf(b, "\tif err != nil {\n")
		fmt.Fprintf(b, "\t\treturn nil, err\n")
		fmt.Fprintf(b, "\t}\n")
		if resType == "any" {
			fmt.Fprintf(b, "\treturn res, nil\n")
		} else if strings.HasPrefix(resType, "*") {
			fmt.Fprintf(b, "\treturn &res, nil\n")
		} else {
			fmt.Fprintf(b, "\treturn res, nil\n")
		}
		fmt.Fprintf(b, "}\n")
	}

	return b.Flush()
}

func schemaUsesTimestamp(schema jtd.Schema) bool {
	if schema.Type == jtd.TypeTimestamp {
		return true
	}
	for _, v := range schema.Definitions {
		if schemaUsesTimestamp(v) {
			return true
		}
	}
	for _, v := range schema.Properties {
		if schemaUsesTimestamp(v) {
			return true
		}
	}
	for _, v := range schema.OptionalProperties {
		if schemaUsesTimestamp(v) {
			return true
		}
	}
	if schema.Elements != nil && schemaUsesTimestamp(*schema.Elements) {
		return true
	}
	if schema.Values != nil && schemaUsesTimestamp(*schema.Values) {
		return true
	}
	return false
}

func metaUsesTimestamp(meta lokerpc.Meta) bool {
	for _, v := range meta.Definitions {
		if schemaUsesTimestamp(v) {
			return true
		}
	}
	for _, iface := range meta.Interfaces {
		if iface.RequestTypeDef != nil && schemaUsesTimestamp(*iface.RequestTypeDef) {
			return true
		}
		if iface.ResponseTypeDef != nil && schemaUsesTimestamp(*iface.ResponseTypeDef) {
			return true
		}
	}
	return false
}

// Regexp that matches word boundaries,
// e.g.
// "customer_id" -> "CustomerID"
// "order-item" -> "OrderItem"
// "customer address" -> "CustomerAddress"
var fieldRe = regexp.MustCompile(`[_\-\s]+([a-zA-Z0-9])`)

var invalidCharRe = regexp.MustCompile(`[^a-zA-Z0-9_]`)

var idRe = regexp.MustCompile(`Id$`)

func goFieldName(name string) string {
	// Capitalize the first letter
	name = strings.Title(name)
	// Replace word boundaries
	name = fieldRe.ReplaceAllStringFunc(name, func(s string) string {
		return strings.ToUpper(string(s[len(s)-1]))
	})

	// Remove invalid characters
	name = invalidCharRe.ReplaceAllString(name, "")

	// Special case: change "Id" to "ID"
	name = idRe.ReplaceAllString(name, "ID")

	return name
}
