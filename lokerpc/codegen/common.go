package codegen

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	jtd "github.com/jsontypedef/json-typedef-go"
)

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func schemaDescription(schema jtd.Schema) string {
	description, _ := schema.Metadata["description"].(string)
	return description
}

func schemaEnumNames(schema jtd.Schema) []string {
	raw, ok := schema.Metadata["enumNames"]
	if !ok {
		return nil
	}

	value := reflect.ValueOf(raw)
	if value.Kind() != reflect.Slice {
		return nil
	}
	names := make([]string, 0, value.Len())
	for index := 0; index < value.Len(); index++ {
		name, ok := value.Index(index).Interface().(string)
		if !ok {
			return nil
		}
		names = append(names, name)
	}
	if len(names) != len(schema.Enum) {
		return nil
	}
	return names
}

func pascalCase(kebabCase string) string {
	var sb strings.Builder
	for _, s := range strings.Split(kebabCase, "-") {
		sb.WriteString(capitalize(s))
	}
	return sb.String()
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var notRequireQuotes = regexp.MustCompile(`(?i)^[a-z_$][a-z0-9_$]*$`)

func quoteFieldNames(s string) string {
	if notRequireQuotes.MatchString(s) {
		return s
	}
	return fmt.Sprintf("%q", s)
}
