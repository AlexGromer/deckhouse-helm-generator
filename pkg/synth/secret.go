package synth

import (
	"fmt"
	"regexp"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/secretname"
)

// SecretRules decide which environment variables go to a Secret. The zero
// value (and nil) is the built-in heuristic; NewSecretRules replaces it with
// patterns from a platform profile.
type SecretRules struct {
	patterns []*regexp.Regexp
}

// NewSecretRules compiles patterns (case-insensitive regular expressions
// matched anywhere in the variable name). They replace the built-in
// heuristic completely. No patterns gives the built-in heuristic.
func NewSecretRules(patterns []string) (*SecretRules, error) {
	rules := &SecretRules{}
	for _, p := range patterns {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return nil, fmt.Errorf("secret pattern %q: %w", p, err)
		}
		rules.patterns = append(rules.patterns, re)
	}
	return rules, nil
}

// Match reports whether a variable of that name belongs in a Secret.
func (r *SecretRules) Match(name string) bool {
	if r == nil || len(r.patterns) == 0 {
		return secretname.Sensitive(name)
	}
	for _, re := range r.patterns {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}
