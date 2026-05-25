package strategy

import (
	"slices"
	"strings"
)

func IDsFromString(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func containsID(raw string, id string) bool {
	return slices.Contains(IDsFromString(raw), id)
}

func stringParam(params map[string]any, key string) string {
	raw, ok := params[key].(string)
	if !ok {
		return ""
	}
	return raw
}

func contextProperty(ctxProperties map[string]string, key string) string {
	if ctxProperties == nil {
		return ""
	}
	return ctxProperties[key]
}
