package unleash

import (
	"slices"
	"strings"
)

func idsFromString(s string) []string {
	out := make([]string, 0, strings.Count(s, ",")+1)
	for p := range strings.SplitSeq(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func containsID(raw string, id string) bool {
	return slices.Contains(idsFromString(raw), id)
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

func contextHasID(ctxProperties map[string]string, key string, ids string) bool {
	property := contextProperty(ctxProperties, key)
	if property == "" {
		return false
	}
	return containsID(ids, property)
}
