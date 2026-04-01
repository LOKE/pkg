package codegen

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/LOKE/pkg/lokerpc"
	jtd "github.com/jsontypedef/json-typedef-go"
)

func GenGoType(schema jtd.Schema, imports map[string]struct{}) string {
	var t string

	for k, v := range schema.Definitions {
		t += "\n"
		t += "type " + goFieldName(k) + " " + GenGoType(v, imports) + "\n"
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
			imports["time"] = struct{}{}
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
		t += "[]" + GenGoType(*schema.Elements, imports)
	case jtd.FormValues:
		t += "map[string]" + GenGoType(*schema.Values, imports)
	case jtd.FormProperties:
		t += "struct {\n"
		for _, k := range sortedKeys(schema.Properties) {
			t += "\t" + goFieldName(k) + " " + GenGoType(schema.Properties[k], imports) + "`json:\"" + k + "\"`\n"
		}
		for _, k := range sortedKeys(schema.OptionalProperties) {
			t += "\t" + goFieldName(k) + " " + GenGoType(schema.OptionalProperties[k], imports) + "`json:\"" + k + ",omitempty\"`\n"
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

type resolvedMethod struct {
	reqType string
	resType string
	isVoid  bool
}

// resolveMethodTypes determines the Go request and response types for an endpoint,
// including whether the method has a void return type.
func resolveMethodTypes(v lokerpc.EndpointMeta, imports map[string]struct{}) resolvedMethod {
	reqType := "any"
	if v.RequestTypeDef != nil {
		reqType = GenGoType(*v.RequestTypeDef, imports)
	}

	resType := "any"
	isVoid := false
	if v.ResponseTypeDef != nil {
		if v.ResponseTypeDef.Metadata["void"] == true {
			isVoid = true
			resType = ""
		} else {
			resType = GenGoType(*v.ResponseTypeDef, imports)

			if !strings.HasPrefix(resType, "[]") && !strings.HasPrefix(resType, "map[") && !strings.HasPrefix(resType, "*") {
				resType = "*" + resType
			}
		}
	}

	return resolvedMethod{reqType: reqType, resType: resType, isVoid: isVoid}
}

func GenGoClient(w io.Writer, meta lokerpc.Meta) error {
	defOrder := normalise(&meta)

	imports := map[string]struct{}{
		"context": {},
	}

	var b bytes.Buffer

	for _, k := range defOrder {
		b.WriteString("\n")
		fmt.Fprintf(&b, "type %s %s;\n", goFieldName(k), GenGoType(meta.Definitions[k], imports))
	}

	// Service interface
	b.WriteString("\n")
	// goDocComment(b, meta.Help, "")
	b.WriteString("type " + goFieldName(meta.ServiceName) + "Service interface {\n")
	for _, v := range meta.Interfaces {
		m := resolveMethodTypes(v, imports)

		// goDocComment(b, v.Help, "\t")
		if m.isVoid {
			fmt.Fprintf(&b, "\t%s(context.Context, %s) error\n", goFieldName(v.MethodName), m.reqType)
		} else {
			fmt.Fprintf(&b, "\t%s(context.Context, %s) (%s, error)\n", goFieldName(v.MethodName), m.reqType, m.resType)
		}
	}
	b.WriteString("}\n")

	// Service client implementation
	b.WriteString("\n")
	// goDocComment(b, meta.Help, "")
	b.WriteString("type " + goFieldName(meta.ServiceName) + "RPCClient struct{\nlokerpc.Client}\n\n")
	for _, v := range meta.Interfaces {
		m := resolveMethodTypes(v, imports)

		if m.isVoid {
			fmt.Fprintf(&b, "func (c %sRPCClient) %s(ctx context.Context, req %s) error {\n", goFieldName(meta.ServiceName), goFieldName(v.MethodName), m.reqType)
			fmt.Fprintf(&b, "\treturn c.DoRequest(ctx, \"%s\", req, nil)\n", v.MethodName)
			fmt.Fprintf(&b, "}\n")
		} else {
			varType := m.resType
			if varType != "any" && strings.HasPrefix(varType, "*") {
				varType = varType[1:]
			}

			fmt.Fprintf(&b, "func (c %sRPCClient) %s(ctx context.Context, req %s) (%s, error) {\n", goFieldName(meta.ServiceName), goFieldName(v.MethodName), m.reqType, m.resType)
			fmt.Fprintf(&b, "\tvar res %s\n", varType)
			fmt.Fprintf(&b, "\terr := c.DoRequest(ctx, \"%s\", req, &res)\n", v.MethodName)
			fmt.Fprintf(&b, "\tif err != nil {\n")
			fmt.Fprintf(&b, "\t\treturn nil, err\n")
			fmt.Fprintf(&b, "\t}\n")
			if m.resType == "any" {
				fmt.Fprintf(&b, "\treturn res, nil\n")
			} else if strings.HasPrefix(m.resType, "*") {
				fmt.Fprintf(&b, "\treturn &res, nil\n")
			} else {
				fmt.Fprintf(&b, "\treturn res, nil\n")
			}
			fmt.Fprintf(&b, "}\n")
		}
	}

	// Write header
	fmt.Fprintf(w, "package %s\n", strings.ToLower(strings.ReplaceAll(meta.ServiceName, "-", "")))
	fmt.Fprintf(w, "\nimport (\n")

	for _, im := range sortedKeys(imports) {
		fmt.Fprintf(w, "\t\"%s\"\n", im)
	}
	fmt.Fprintf(w, "\n\t\"github.com/LOKE/pkg/lokerpc\"\n")
	fmt.Fprintf(w, ")\n\n")

	_, err := io.Copy(w, &b)

	return err
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
