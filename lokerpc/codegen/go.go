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

	for _, k := range sortedKeys(schema.Definitions) {
		t += "\n"
		t += "type " + goFieldName(k) + " " + GenGoType(schema.Definitions[k], imports) + "\n"
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
	// isNullable is true when the response schema itself is nullable (as opposed
	// to resType merely being auto-wrapped in "*" for API convenience).
	isNullable bool
}

// schemaIsNullable reports whether schema is nullable. It checks the schema's
// own Nullable flag first, then falls back to the referenced definition's
// Nullable flag — normalise() hoists inline schemas (Nullable included) into
// named definitions and replaces the original with a bare ref, so a hoisted
// response's nullability only survives on the definition it now points to.
func schemaIsNullable(schema jtd.Schema, defs map[string]jtd.Schema) bool {
	if schema.Nullable {
		return true
	}
	if schema.Ref != nil {
		return defs[*schema.Ref].Nullable
	}
	return false
}

// resolveMethodTypes determines the Go request and response types for an endpoint,
// including whether the method has a void return type.
func resolveMethodTypes(v lokerpc.EndpointMeta, defs map[string]jtd.Schema, hoisted map[string]bool, imports map[string]struct{}) resolvedMethod {
	reqType := "any"
	if v.RequestTypeDef != nil {
		reqType = GenGoType(*v.RequestTypeDef, imports)
	}

	resType := "any"
	isVoid := false
	isNullable := false
	if v.ResponseTypeDef != nil {
		if v.ResponseTypeDef.Metadata["void"] == true {
			isVoid = true
			resType = ""
		} else {
			resType = GenGoType(*v.ResponseTypeDef, imports)
			isNullable = schemaIsNullable(*v.ResponseTypeDef, defs)

			// A ref to a pre-existing (non-hoisted) definition whose own schema is
			// nullable already renders as a pointer type (see the "type X
			// *Underlying" definitions emitted below), so resType already denotes
			// a pointer even without a literal "*" here — don't stack another one
			// on top. Hoisted definitions never render this way (see the
			// definitions loop in GenGoClient), so they always need the "*" added
			// here at the usage site instead.
			alreadyPointer := strings.HasPrefix(resType, "*")
			if !alreadyPointer && v.ResponseTypeDef.Ref != nil && !hoisted[*v.ResponseTypeDef.Ref] {
				refType := GenGoType(defs[*v.ResponseTypeDef.Ref], imports)
				alreadyPointer = strings.HasPrefix(refType, "*")
			}

			if !alreadyPointer && !strings.HasPrefix(resType, "[]") && !strings.HasPrefix(resType, "map[") {
				resType = "*" + resType
			}
		}
	}

	return resolvedMethod{reqType: reqType, resType: resType, isVoid: isVoid, isNullable: isNullable}
}

func GenGoClient(w io.Writer, meta lokerpc.Meta) error {
	defOrder, hoisted := normalise(&meta)

	imports := map[string]struct{}{
		"context": {},
	}

	var b bytes.Buffer

	for _, k := range defOrder {
		b.WriteString("\n")
		def := meta.Definitions[k]
		if hoisted[k] {
			// Hoisted definitions are an implementation detail of normalise()
			// giving an anonymous inline schema a Go name — they aren't a
			// schema author's deliberate nullable type, so don't bake a "*"
			// into the type itself. Nullability instead shows up as "*Name"
			// at each usage site (see resolveMethodTypes).
			def.Nullable = false
		}
		fmt.Fprintf(&b, "type %s %s;\n", goFieldName(k), GenGoType(def, imports))
	}

	// Service interface
	b.WriteString("\n")
	// goDocComment(b, meta.Help, "")
	b.WriteString("type " + goFieldName(meta.ServiceName) + "Service interface {\n")
	for _, v := range meta.Interfaces {
		m := resolveMethodTypes(v, meta.Definitions, hoisted, imports)

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
		m := resolveMethodTypes(v, meta.Definitions, hoisted, imports)

		if m.isVoid {
			fmt.Fprintf(&b, "func (c %sRPCClient) %s(ctx context.Context, req %s) error {\n", goFieldName(meta.ServiceName), goFieldName(v.MethodName), m.reqType)
			fmt.Fprintf(&b, "\treturn c.DoRequest(ctx, \"%s\", req, nil)\n", v.MethodName)
			fmt.Fprintf(&b, "}\n")
		} else {
			varType := m.resType
			if !m.isNullable && varType != "any" && strings.HasPrefix(varType, "*") {
				varType = varType[1:]
			}

			fmt.Fprintf(&b, "func (c %sRPCClient) %s(ctx context.Context, req %s) (%s, error) {\n", goFieldName(meta.ServiceName), goFieldName(v.MethodName), m.reqType, m.resType)
			fmt.Fprintf(&b, "\tvar res %s\n", varType)
			fmt.Fprintf(&b, "\terr := c.DoRequest(ctx, \"%s\", req, &res)\n", v.MethodName)
			fmt.Fprintf(&b, "\tif err != nil {\n")
			fmt.Fprintf(&b, "\t\treturn nil, err\n")
			fmt.Fprintf(&b, "\t}\n")
			if m.resType == "any" || m.isNullable {
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
