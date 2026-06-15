package lint

import (
	"go/ast"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const enumTagDoc = `check that enum-typed struct fields carry a validate:"enum" rule

An enum is a named type that declares its permitted values through an
EnumValues() []string method (lokerpc.EnumProvider). A struct field of such a
type must be validated with the "enum" rule so the value is enforced at runtime.`

// EnumTagAnalyzer reports exported struct fields whose type is an enum (a named
// type implementing EnumValues() []string) but whose validate tag lacks the
// "enum" rule. It is the static replacement for lokerpc.AuditEnumValidatorTags.
var EnumTagAnalyzer = &analysis.Analyzer{
	Name:     "enumtag",
	Doc:      enumTagDoc,
	URL:      "https://github.com/LOKE/pkg/tree/master/lint",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runEnumTag,
}

func runEnumTag(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	insp.Preorder([]ast.Node{(*ast.StructType)(nil)}, func(n ast.Node) {
		st := n.(*ast.StructType)
		for _, field := range st.Fields.List {
			// Embedded fields carry no name and can't hold a validate tag.
			if len(field.Names) == 0 {
				continue
			}

			enumName, ok := enumTypeName(pass.TypesInfo.TypeOf(field.Type))
			if !ok || hasEnumRule(validateTag(field.Tag)) {
				continue
			}

			for _, id := range field.Names {
				if !id.IsExported() {
					continue
				}
				pass.Reportf(id.Pos(),
					"field %s is an enum type (%s) but its validate tag is missing the %q rule",
					id.Name, enumName, "enum")
			}
		}
	})

	return nil, nil
}

// enumTypeName reports the name of t's underlying enum type and whether t is an
// enum. Pointers and aliases are unwrapped, so *Currency and a CurrencyAlias are
// both treated as Currency. A type is an enum when it is a named string type
// whose value method set contains EnumValues() []string.
func enumTypeName(t types.Type) (string, bool) {
	if t == nil {
		return "", false
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	if !ok {
		return "", false
	}
	// Restrict to named string types: lokerpc only refs string types as enums,
	// which also excludes interfaces and other types that happen to declare an
	// EnumValues method.
	if basic, ok := named.Underlying().(*types.Basic); !ok || basic.Info()&types.IsString == 0 {
		return "", false
	}
	if !hasEnumValuesMethod(named) {
		return "", false
	}
	return named.Obj().Name(), true
}

// hasEnumValuesMethod reports whether named's value method set contains a method
// EnumValues() []string. Using the value method set (not the pointer's) mirrors
// EnumProvider's value-receiver requirement.
func hasEnumValuesMethod(named *types.Named) bool {
	ms := types.NewMethodSet(named)
	for i := 0; i < ms.Len(); i++ {
		fn, ok := ms.At(i).Obj().(*types.Func)
		if !ok || fn.Name() != "EnumValues" {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok || sig.Params().Len() != 0 || sig.Results().Len() != 1 {
			return false
		}
		slice, ok := sig.Results().At(0).Type().(*types.Slice)
		if !ok {
			return false
		}
		basic, ok := slice.Elem().(*types.Basic)
		return ok && basic.Kind() == types.String
	}
	return false
}

// validateTag returns the value of the `validate` key in a struct field tag, or
// "" when there is no tag or no validate key.
func validateTag(tag *ast.BasicLit) string {
	if tag == nil {
		return ""
	}
	unquoted, err := strconv.Unquote(tag.Value)
	if err != nil {
		return ""
	}
	return reflect.StructTag(unquoted).Get("validate")
}

// hasEnumRule reports whether a validate tag value contains the "enum" rule.
func hasEnumRule(validate string) bool {
	for _, part := range strings.Split(validate, ",") {
		if strings.TrimSpace(part) == "enum" {
			return true
		}
	}
	return false
}
