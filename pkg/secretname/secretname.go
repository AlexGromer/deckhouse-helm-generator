// Package secretname decides from the name of a setting whether its value is
// a credential. dhg uses it to put synthesized environment variables into a
// Secret and to redact credentials found in ConfigMaps and container env.
package secretname

import "strings"

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
	// duration, a limit or a kind rather than the credential itself
	// (TOKEN_URL, KEYCLOAK_HOST, TOKEN_TTL, PASSWORD_MIN_LENGTH,
	// TOKEN_VALIDITY_SECONDS, TOKENSERVICE_CLIENTID).
	locatorWords = map[string]bool{
		"URL": true, "URI": true, "HOST": true, "PORT": true, "ENDPOINT": true,
		"PATH": true, "FILE": true, "ENABLED": true,
		"TTL": true, "EXPIRY": true, "EXPIRATION": true, "LENGTH": true, "MIN": true,
		"MAX": true, "COUNT": true, "LIMIT": true, "TIMEOUT": true,
		"SECONDS": true, "SERVICE": true, "GRANTTYPE": true, "CLIENTID": true,
	}
)

// Sensitive is the default heuristic: a name whose parts include PASSWORD,
// SECRET, CREDENTIAL, PRIVATE_KEY, API_KEY or ACCESS_KEY is a credential, and
// so is a name whose last part is TOKEN. A name that ends in URL, HOST, PORT
// or a similar locator, or in TTL, LENGTH, MAX or a similar limit, is not
// (MAX_TOKENS, TOKEN_TTL, ACCESS_TOKEN_VALIDITY_SECONDS). TOKEN in the middle
// of a name is a setting about a token (USE_TOKEN_FOR_FEIGN), not the token.
// Parts are whole words: SECRETARY or MONKEY do not match.
func Sensitive(name string) bool {
	parts := strings.FieldsFunc(strings.ToUpper(name), func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
	if len(parts) == 0 {
		return false
	}
	last := len(parts) - 1
	if locatorWords[parts[last]] {
		return false
	}
	for i, p := range parts {
		if secretWords[p] && (p != "TOKEN" || i == last) {
			return true
		}
		for _, suffix := range secretSuffixes {
			if len(p) > len(suffix) && strings.HasSuffix(p, suffix) && (suffix != "TOKEN" || i == last) {
				return true
			}
		}
		if i+1 < len(parts) && secretPairs[p+"_"+parts[i+1]] {
			return true
		}
	}
	return false
}
