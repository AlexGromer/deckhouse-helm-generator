package processor

import (
	"fmt"
	"strings"
)

// SpecOverlay returns template lines rendering a "spec:" block from the
// complete input spec kept at <dot>.spec, with each listed key that is
// present at <dot> replacing the spec field of the same name.
//
// Processors expose a few spec fields as top-level values for convenience;
// rendering only those would silently drop every other field of the input.
// The overlay keeps both: the whole spec is reproduced, and the convenience
// keys still win when set. It uses set rather than mergeOverwrite, which
// would ignore overrides holding zero values such as false or 0.
//
// dot is the template expression holding the values map, e.g. "." inside a
// with block or "$svc.moduleConfig".
func SpecOverlay(dot string, keys ...string) string {
	return SpecOverlayVars(dot, keys...) + "spec:\n  {{- toYaml $dhgSpec | nindent 2 }}\n"
}

// SpecOverlayVars returns the part of SpecOverlay that builds $dhgSpec
// without rendering it, for templates that adjust the spec further.
func SpecOverlayVars(dot string, keys ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "{{- $dhgValues := %s }}\n", dot)
	b.WriteString("{{- $dhgSpec := deepCopy ($dhgValues.spec | default dict) }}\n")
	if len(keys) > 0 {
		quoted := make([]string, len(keys))
		for i, k := range keys {
			quoted[i] = fmt.Sprintf("%q", k)
		}
		fmt.Fprintf(&b, "{{- range $k := list %s }}\n", strings.Join(quoted, " "))
		b.WriteString("{{- if hasKey $dhgValues $k }}{{- $_ := set $dhgSpec $k (index $dhgValues $k) }}{{- end }}\n")
		b.WriteString("{{- end }}\n")
	}
	return b.String()
}
