package kafkaclient

import (
	"strings"
	"unicode"
)

// NormalizeKey normalizes a property key by converting to lower case and removing '.', '-', '_'.
func NormalizeKey(key string) string {
	var builder strings.Builder
	builder.Grow(len(key))
	for _, ch := range key {
		if ch != '.' && ch != '-' && ch != '_' {
			builder.WriteRune(unicode.ToLower(ch))
		}
	}
	return builder.String()
}

// isEnvForm checks if a key looks like an environment variable (only uppercase, numbers, and underscores).
func isEnvForm(key string) bool {
	hasLetter := false
	for _, ch := range key {
		if unicode.IsLetter(ch) {
			hasLetter = true
			if !unicode.IsUpper(ch) {
				return false
			}
		} else if !unicode.IsDigit(ch) && ch != '_' {
			return false
		}
	}
	return hasLetter
}
