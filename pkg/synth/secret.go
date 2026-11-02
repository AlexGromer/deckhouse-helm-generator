package synth

import (
	"fmt"
	"regexp"
	"strings"
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
		return builtinSensitive(name)
	}
	for _, re := range r.patterns {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

var (
	// secretWords are parts of a name that mark a credential on their own.
	secretWords = map[string]bool{
		"PASSWORD": true, "PASSWORDS": true, "PASSWD": true, "SECRET": true, "SECRETS": true,
		"TOKEN": true, "CREDENTIAL": true, "CREDENTIALS": true,
		"PRIVATEKEY": true, "APIKEY": true, "ACCESSKEY": true,
	}
	// secretPairs are two parts that mark a credential together.
	secretPairs = map[string]bool{"PRIVATE_KEY": true, "API_KEY": true, "ACCESS_KEY": true}
	// secretSuffixes catch names written without separators (DBPASSWORD).
	secretSuffixes = []string{"PASSWORD", "PASSWD", "SECRET", "TOKEN"}
	// locatorWords at the end of a name say it holds an address, a switch, a
	// duration or a limit rather than the credential itself (TOKEN_URL,
	// KEYCLOAK_HOST, TOKEN_TTL, PASSWORD_MIN_LENGTH).
	locatorWords = map[string]bool{
		"URL": true, "URI": true, "HOST": true, "PORT": true, "ENDPOINT": true,
		"PATH": true, "FILE": true, "ENABLED": true,
		"TTL": true, "EXPIRY": true, "EXPIRATION": true, "LENGTH": true, "MIN": true,
		"MAX": true, "COUNT": true, "LIMIT": true, "TIMEOUT": true,
	}
)

// builtinSensitive is the default heuristic: a name whose parts include
// PASSWORD, SECRET, TOKEN, CREDENTIAL, PRIVATE_KEY, API_KEY or ACCESS_KEY is
// a credential, unless it ends in URL, HOST, PORT or a similar locator, or in
// TTL, LENGTH, MAX or a similar limit (MAX_TOKENS, TOKEN_TTL). Parts are whole
// words: SECRETARY or MONKEY do not match.
func builtinSensitive(name string) bool {
	parts := strings.FieldsFunc(strings.ToUpper(name), func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
	if len(parts) == 0 {
		return false
	}
	if locatorWords[parts[len(parts)-1]] {
		return false
	}
	for i, p := range parts {
		if secretWords[p] {
			return true
		}
		for _, suffix := range secretSuffixes {
			if len(p) > len(suffix) && strings.HasSuffix(p, suffix) {
				return true
			}
		}
		if i+1 < len(parts) && secretPairs[p+"_"+parts[i+1]] {
			return true
		}
	}
	return false
}
