package codegen

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
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
