package lokerpc

import (
	"fmt"
	"reflect"
	"strings"
)

// EnumProvider is implemented by a named string type to declare its permitted
// values in one place, so schema generation and validation can resolve them by
// reflection. Use a value receiver so reflect.Zero(t).Interface().(EnumProvider)
// works during schema generation; a pointer receiver fails that assertion
// silently.
//
//	type Currency string
//
//	const (
//	    CurrencyAUD Currency = "AUD"
//	    CurrencyNZD Currency = "NZD"
//	    CurrencyUSD Currency = "USD"
//	)
//
//	func (Currency) EnumValues() []string {
//	    return lokerpc.Enum(CurrencyAUD, CurrencyNZD, CurrencyUSD)
//	}
type EnumProvider interface {
	EnumValues() []string
}

// enumProviderType is the reflect.Type of EnumProvider.
var enumProviderType = reflect.TypeOf((*EnumProvider)(nil)).Elem()

// Enum converts typed string constants into a []string for an EnumValues
// implementation, so a type's values and their declarations never drift apart:
//
//	func (Currency) EnumValues() []string {
//	    return lokerpc.Enum(CurrencyAUD, CurrencyNZD, CurrencyUSD)
//	}
func Enum[T ~string](values ...T) []string {
	result := make([]string, len(values))
	for i, v := range values {
		result[i] = string(v)
	}
	return result
}

// EnumValuesFor returns the enum values for a named string type that implements
// EnumProvider. A pointer to such a type is dereferenced first. ok is false if t
// is not a known enum. Custom validators use this to enforce enum values at
// runtime.
func EnumValuesFor(t reflect.Type) (values []string, ok bool) {
	// Deref pointers: *Currency's method set includes Currency's value-receiver
	// EnumValues, but reflect.Zero(*Currency) is a nil pointer that would panic
	// when EnumValues dereferences it.
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Implements(enumProviderType) {
		return reflect.Zero(t).Interface().(EnumProvider).EnumValues(), true
	}
	return nil, false
}

// isEnumType reports whether t is a known enum (implements EnumProvider) without
// materialising its values.
func isEnumType(t reflect.Type) bool {
	return t.Implements(enumProviderType)
}

// AuditEnumValidatorTags walks t's exported fields recursively and returns one
// error per field whose type (after dereferencing pointers) is a known enum but
// whose validate tag is missing the "enum" rule. Call it from a test over your
// request types to guarantee enum fields are validated at runtime.
func AuditEnumValidatorTags(t reflect.Type) []error {
	return auditEnumTagsInto(t, map[reflect.Type]bool{})
}

func auditEnumTagsInto(t reflect.Type, visited map[reflect.Type]bool) []error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if visited[t] {
		return nil
	}
	visited[t] = true
	if t.Kind() != reflect.Struct {
		return nil
	}

	var errs []error
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}

		if isEnumType(ft) && !hasEnumRule(f.Tag.Get("validate")) {
			errs = append(errs, fmt.Errorf(
				"%s.%s: %s is an enum type but validate tag lacks \"enum\" (got %q)",
				t.Name(), f.Name, ft.Name(), f.Tag.Get("validate"),
			))
		}

		errs = append(errs, auditEnumTagsInto(ft, visited)...)
	}
	return errs
}

func hasEnumRule(validateTag string) bool {
	for _, part := range strings.Split(validateTag, ",") {
		if strings.TrimSpace(part) == "enum" {
			return true
		}
	}
	return false
}
