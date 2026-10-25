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
	var missing []string
	out := placeholder.ReplaceAllStringFunc(v, func(m string) string {
		sm := placeholder.FindStringSubmatch(m)
		if strings.Contains(m, ":") {
			return sm[2]
		}
		missing = append(missing, sm[1])
		return ""
	})
	return out, strings.Join(missing, ", ")
}
