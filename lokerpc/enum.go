package lokerpc

import "reflect"

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
//
// The enumtag analyzer (github.com/LOKE/pkg/lint) statically checks that struct
// fields of an EnumProvider type carry a validate:"enum" rule.
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
