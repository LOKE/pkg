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

// resolveRef follows schema through ref definitions to the schema it denotes.
func resolveRef(schema jtd.Schema, defs map[string]jtd.Schema) jtd.Schema {
	seen := map[string]bool{}
	for schema.Ref != nil {
		if seen[*schema.Ref] {
			return schema
		}
		seen[*schema.Ref] = true
		def, ok := defs[*schema.Ref]
		if !ok {
			return schema
		}
		schema = def
	}
	return schema
}

// goGen carries the definitions in scope, the imports the output needs, and
// the declarations hoisted out of inline positions (union envelopes and their
// variants), which need a name to hang methods off.
type goGen struct {
	defs    map[string]jtd.Schema
	imports map[string]struct{}
	decls   *bytes.Buffer
}

// optionalField renders an optional property's type and its JSON omit option.
// Scalars and objects are pointers so absence is distinguishable from a zero
// value; collections rely on a nil slice/map and use omitzero.
func optionalField(schema jtd.Schema, name string, g goGen) (goType, omit string) {
	t := genGoType(schema, name, false, g)

	if schema.Nullable {
		return t, "omitempty"
	}

	// A ref renders as a named type, so the underlying definition decides how
	// absence is expressed.
	resolved := resolveRef(schema, g.defs)
	if resolved.Nullable {
		return t, "omitempty"
	}

	switch resolved.Form() {
	case jtd.FormElements, jtd.FormValues:
		return t, "omitzero"
	case jtd.FormProperties, jtd.FormDiscriminator:
		// Structs render as a value type, so an absent object and one whose
		// fields all happen to be zero decode identically. Consumers end up
		// guessing from a field ("id != \"\""), which is wrong whenever the
		// zero value is legitimate. A pointer makes absence unambiguous.
		return "*" + t, "omitempty"
	case jtd.FormType, jtd.FormEnum:
		// Every scalar has a zero value a payload can legitimately carry, so a
		// value type cannot answer "was this field sent?". Enums are strings,
		// so absence reads as "". Timestamps are the worst of them: omitzero
		// only affects marshalling, so an absent timestamp still decodes to
		// time.Time{} and reads as a real instant somewhere in year 1.
		return "*" + t, "omitempty"
	}

	return t, "omitempty"
}

func GenGoType(schema jtd.Schema, imports map[string]struct{}) string {
	var decls bytes.Buffer
	t := genGoType(schema, "", true, goGen{defs: schema.Definitions, imports: imports, decls: &decls})
	return decls.String() + t
}

// genGoType renders schema. name is the PascalCase path used to name hoisted
// types; root is true when the caller is declaring name itself.
func genGoType(schema jtd.Schema, name string, root bool, g goGen) string {
	var t string

	if len(schema.Definitions) > 0 {
		merged := make(map[string]jtd.Schema, len(g.defs)+len(schema.Definitions))
		for k, v := range g.defs {
			merged[k] = v
		}
		for k, v := range schema.Definitions {
			merged[k] = v
		}
		g.defs = merged
	}

	for _, k := range sortedKeys(schema.Definitions) {
		t += "\n"
		def := schema.Definitions[k]
		t += "type " + goFieldName(k) + defAssign(def, g.defs) + genGoType(def, goFieldName(k), true, g) + "\n"
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
			g.imports["time"] = struct{}{}
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
		t += "[]" + genGoType(*schema.Elements, name, false, g)
	case jtd.FormValues:
		t += "map[string]" + genGoType(*schema.Values, name, false, g)
	case jtd.FormProperties:
		t += "struct {\n"
		for _, k := range sortedKeys(schema.Properties) {
			t += "\t" + goFieldName(k) + " " + genGoType(schema.Properties[k], name+goFieldName(k), false, g) + "`json:\"" + k + "\"`\n"
		}
		for _, k := range sortedKeys(schema.OptionalProperties) {
			propType, omit := optionalField(schema.OptionalProperties[k], name+goFieldName(k), g)
			t += "\t" + goFieldName(k) + " " + propType + "`json:\"" + k + "," + omit + "\"`\n"
		}
		t += "}"
	case jtd.FormDiscriminator:
		body := union(schema, name, g)
		if root {
			t += body
		} else {
			fmt.Fprintf(g.decls, "\ntype %s %s\n", name, body)
			t += name
		}
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

// defAssign picks the separator for a type definition. Timestamps, including
// those reached through a chain of refs, become aliases so they keep
// time.Time's JSON marshalling.
func defAssign(def jtd.Schema, defs map[string]jtd.Schema) string {
	seen := map[string]bool{}
	for def.Form() == jtd.FormRef && !seen[*def.Ref] {
		seen[*def.Ref] = true
		def = defs[*def.Ref]
	}
	if def.Form() == jtd.FormType && def.Type == jtd.TypeTimestamp {
		return " = "
	}
	return " "
}

// union emits a oneof-style envelope: one pointer field per variant, with
// MarshalJSON/UnmarshalJSON translating to and from the tagged wire form.
func union(schema jtd.Schema, name string, g goGen) string {
	g.imports["encoding/json"] = struct{}{}
	g.imports["fmt"] = struct{}{}

	keys := sortedKeys(schema.Mapping)
	variants := make([]string, len(keys))
	for i, k := range keys {
		variants[i] = name + variantName(k)
		fmt.Fprintf(g.decls, "\ntype %s %s\n", variants[i], genGoType(schema.Mapping[k], variants[i], true, g))
	}

	body := "struct {\n"
	for i := range keys {
		body += "\t" + variantName(keys[i]) + " *" + variants[i] + "\n"
	}
	body += "}"

	tagField := goFieldName(schema.Discriminator)
	m := g.decls
	fmt.Fprintf(m, "\nfunc (v %s) MarshalJSON() ([]byte, error) {\n", name)
	m.WriteString("\tswitch {\n")
	for i, k := range keys {
		fmt.Fprintf(m, "\tcase v.%s != nil:\n", variantName(k))
		fmt.Fprintf(m, "\t\treturn json.Marshal(struct {\n\t\t\t%s string `json:\"%s\"`\n\t\t\t*%s\n\t\t}{%q, v.%s})\n", tagField, schema.Discriminator, variants[i], k, variantName(k))
	}
	m.WriteString("\t}\n")
	fmt.Fprintf(m, "\treturn nil, fmt.Errorf(\"%s: no variant set\")\n", name)
	m.WriteString("}\n")

	fmt.Fprintf(m, "\nfunc (v *%s) UnmarshalJSON(b []byte) error {\n", name)
	fmt.Fprintf(m, "\tvar tag struct {\n\t\t%s string `json:\"%s\"`\n\t}\n", tagField, schema.Discriminator)
	m.WriteString("\tif err := json.Unmarshal(b, &tag); err != nil {\n\t\treturn err\n\t}\n")
	fmt.Fprintf(m, "\t*v = %s{}\n", name)
	fmt.Fprintf(m, "\tswitch tag.%s {\n", tagField)
	for i, k := range keys {
		fmt.Fprintf(m, "\tcase %q:\n", k)
		fmt.Fprintf(m, "\t\tv.%s = &%s{}\n", variantName(k), variants[i])
		fmt.Fprintf(m, "\t\treturn json.Unmarshal(b, v.%s)\n", variantName(k))
	}
	m.WriteString("\t}\n")
	fmt.Fprintf(m, "\treturn fmt.Errorf(\"%s: unknown %s %%q\", tag.%s)\n", name, schema.Discriminator, tagField)
	m.WriteString("}\n")

	return body
}

// SCREAMING_CASE tags would otherwise collapse to USERCREATED.
func variantName(tag string) string {
	if tag == strings.ToUpper(tag) {
		tag = strings.ToLower(tag)
	}
	return goFieldName(tag)
}

type resolvedMethod struct {
	reqType    string
	resType    string
	isVoid     bool
	isNullable bool
}

// schemaIsNullable reports whether schema, or the definition it refs, is nullable.
func schemaIsNullable(schema jtd.Schema, defs map[string]jtd.Schema) bool {
	if schema.Nullable {
		return true
	}
	if schema.Ref != nil {
		return defs[*schema.Ref].Nullable
	}
	return false
}

// refAlreadyPointer reports whether resolvedType already denotes a pointer,
// either directly or via a ref to a pre-existing (non-hoisted) definition
func refAlreadyPointer(schema jtd.Schema, resolvedType string, hoisted map[string]bool, g goGen) bool {
	if strings.HasPrefix(resolvedType, "*") {
		return true
	}
	if schema.Ref != nil && !hoisted[*schema.Ref] {
		return strings.HasPrefix(genGoType(g.defs[*schema.Ref], goFieldName(*schema.Ref), true, g), "*")
	}
	return false
}

// resolveMethodTypes determines the Go request and response types for an endpoint,
// including whether the method has a void return type.
func resolveMethodTypes(v lokerpc.EndpointMeta, hoisted map[string]bool, g goGen) resolvedMethod {
	reqType := "any"
	if v.RequestTypeDef != nil {
		reqType = genGoType(*v.RequestTypeDef, goFieldName(v.MethodName)+"Request", false, g)

		// Unlike responses, requests aren't wrapped in "*" by default — only
		// when the schema is actually nullable.
		if schemaIsNullable(*v.RequestTypeDef, g.defs) && !refAlreadyPointer(*v.RequestTypeDef, reqType, hoisted, g) {
			reqType = "*" + reqType
		}
	}

	resType := "any"
	isVoid := false
	isNullable := false
	if v.ResponseTypeDef != nil {
		if v.ResponseTypeDef.Metadata["void"] == true {
			isVoid = true
			resType = ""
		} else {
			resType = genGoType(*v.ResponseTypeDef, goFieldName(v.MethodName)+"Response", false, g)
			isNullable = schemaIsNullable(*v.ResponseTypeDef, g.defs)

			// A ref to a pre-existing nullable definition already renders as a
			// pointer type, so don't stack another "*" on top.
			alreadyPointer := refAlreadyPointer(*v.ResponseTypeDef, resType, hoisted, g)

			if !alreadyPointer && !strings.HasPrefix(resType, "[]") && !strings.HasPrefix(resType, "map[") {
				resType = "*" + resType
			}
		}
	}

	return resolvedMethod{reqType: reqType, resType: resType, isVoid: isVoid, isNullable: isNullable}
}

func GenGoClient(w io.Writer, meta lokerpc.Meta) error {
	defOrder, hoisted := normalise(&meta)

	var b, decls bytes.Buffer
	g := goGen{defs: meta.Definitions, imports: map[string]struct{}{"context": {}}, decls: &decls}

	for _, k := range defOrder {
		b.WriteString("\n")
		def := meta.Definitions[k]
		if hoisted[k] {
			// Hoisted types shouldn't bake in "*" — nullability shows up as
			// "*Name" at each usage site instead (see resolveMethodTypes).
			def.Nullable = false
		}
		fmt.Fprintf(&b, "type %s%s%s;\n", goFieldName(k), defAssign(def, meta.Definitions), genGoType(def, goFieldName(k), true, g))
	}

	b.Write(decls.Bytes())

	// Service interface
	b.WriteString("\n")
	// goDocComment(b, meta.Help, "")
	b.WriteString("type " + goFieldName(meta.ServiceName) + "Service interface {\n")
	for _, v := range meta.Interfaces {
		m := resolveMethodTypes(v, hoisted, g)

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
		m := resolveMethodTypes(v, hoisted, g)

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

	for _, im := range sortedKeys(g.imports) {
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
