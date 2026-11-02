package synth

import (
	"regexp"
	"strings"
)

var placeholder = regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)

// ResolvePlaceholders replaces ${NAME:default} by its default; it returns
// the text with replaced values and the names of placeholders without a default.
// Spring, Quarkus (SmallRye Config) and Micronaut share this syntax.
func ResolvePlaceholders(v string) (string, string) {
	return ResolvePlaceholdersWith(v, nil)
}

// ResolvePlaceholdersWith is ResolvePlaceholders with fallback values for
// placeholders that have no default of their own (an environment template
// next to the sources). An inline default wins over a fallback.
func ResolvePlaceholdersWith(v string, fallback map[string]string) (string, string) {
	var missing []string
	out := placeholder.ReplaceAllStringFunc(v, func(m string) string {
		sm := placeholder.FindStringSubmatch(m)
		if strings.Contains(m, ":") {
			return sm[2]
		}
		if fb, ok := fallback[sm[1]]; ok {
			return fb
		}
		missing = append(missing, sm[1])
		return ""
	})
	return out, strings.Join(missing, ", ")
}

// Placeholders returns the names of the ${NAME} placeholders of v that have
// no default of their own, in order of appearance.
func Placeholders(v string) []string {
	var names []string
	for _, sm := range placeholder.FindAllStringSubmatch(v, -1) {
		if !strings.Contains(sm[0], ":") {
			names = append(names, sm[1])
		}
	}
	return names
}
