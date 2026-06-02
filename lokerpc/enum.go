package lokerpc

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// EnumSet declares the values of a string enum type T in one place and registers
// them so schema generation and validation can resolve T's values by reflection
// — the type itself needs no method. Declare one set per type at package scope,
// then declare each value with Add; adding a value is a single line and nothing
// ever drifts out of sync:
//
//	type Currency string
//
//	var currencies = lokerpc.NewEnumSet[Currency]()
//
//	var (
//	    CurrencyAUD = currencies.Add("AUD")
//	    CurrencyNZD = currencies.Add("NZD")
//	    CurrencyUSD = currencies.Add("USD")
//	)
//
// The values become package vars rather than consts (a method call can't
// initialise a const), but are used identically in switches and map keys.
type EnumSet[T ~string] struct {
	mu     sync.RWMutex
	values []string
}

// NewEnumSet creates an EnumSet for the string enum type T and registers it so
// TypeSchema and EnumValuesFor can resolve T's values without T implementing any
// interface. Call it once per type at package scope.
func NewEnumSet[T ~string]() *EnumSet[T] {
	s := &EnumSet[T]{}
	registerEnum(reflect.TypeFor[T](), s.Values)
	return s
}

// Add records v as a member of the set and returns it, so a named value and its
// membership are declared in a single expression. Values are reported by Values
// in the order they were added.
func (s *EnumSet[T]) Add(v T) T {
	s.mu.Lock()
	s.values = append(s.values, string(v))
	s.mu.Unlock()
	return v
}

// Values returns the recorded values, in declaration order, as a fresh slice.
func (s *EnumSet[T]) Values() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.values...)
}

// Enum converts typed string constants into a []string. Use it when you keep
// your values as consts and prefer the EnumProvider interface over an EnumSet:
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

// EnumProvider is an optional alternative to NewEnumSet: a named string type can
// declare its own values by implementing this interface. Use a value receiver so
// reflect.Zero(t).Interface().(EnumProvider) works during schema generation; a
// pointer receiver fails that assertion silently.
type EnumProvider interface {
	EnumValues() []string
}

// enumProviderType is the reflect.Type of EnumProvider.
var enumProviderType = reflect.TypeOf((*EnumProvider)(nil)).Elem()

// enumRegistry maps a registered enum type to an accessor for its current
// values. It is written when an EnumSet is constructed (package init in normal
// use) and read during schema generation and validation.
var (
	enumRegistryMu sync.RWMutex
	enumRegistry   = map[reflect.Type]func() []string{}
)

func registerEnum(t reflect.Type, values func() []string) {
	enumRegistryMu.Lock()
	enumRegistry[t] = values
	enumRegistryMu.Unlock()
}

// EnumValuesFor returns the enum values for a named string type, resolving them
// from the EnumSet registry (preferred — no method required) or from an
// EnumProvider implementation. ok is false if t is not a known enum. Custom
// validators use this to enforce enum values at runtime.
func EnumValuesFor(t reflect.Type) (values []string, ok bool) {
	enumRegistryMu.RLock()
	fn, found := enumRegistry[t]
	enumRegistryMu.RUnlock()
	if found {
		return fn(), true
	}
	if t.Implements(enumProviderType) {
		return reflect.Zero(t).Interface().(EnumProvider).EnumValues(), true
	}
	return nil, false
}

// isEnumType reports whether t is a known enum (registered or EnumProvider)
// without materialising its values.
func isEnumType(t reflect.Type) bool {
	enumRegistryMu.RLock()
	_, found := enumRegistry[t]
	enumRegistryMu.RUnlock()
	return found || t.Implements(enumProviderType)
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
