package codegen

import (
	"fmt"
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

func quoteSpaces(s string) string {
	if strings.Contains(s, " ") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
