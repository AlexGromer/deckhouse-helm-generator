package connect

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

var (
	secretParamRegex = regexp.MustCompile(`(?i)([?&])(password|pwd|secret|token|apikey|api-key|access_token)=([^&]+)`)
)

func IsLoopback(host string) bool {
	h := strings.ToLower(host)
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if h == "::1" || h == "0.0.0.0" || h == "::" || h == "127.0.0.1" {
		return true
	}
	if strings.HasPrefix(h, "127.") {
		return true
	}
	return false
}

func Redact(raw string) (string, bool) {
	hadSecret := false
	redacted := raw

	parseRaw := raw
	parseRaw = strings.TrimPrefix(parseRaw, "jdbc:")

	if strings.Contains(parseRaw, "://") {
		if u, err := url.Parse(parseRaw); err == nil && u.User != nil {
			if _, hasPwd := u.User.Password(); hasPwd {
				hadSecret = true
				u.User = url.UserPassword(u.User.Username(), "REDACTED")
				if strings.HasPrefix(raw, "jdbc:") {
					redacted = "jdbc:" + u.String()
				} else {
					redacted = u.String()
				}
			}
		}
	}

	if secretParamRegex.MatchString(redacted) {
		hadSecret = true
		redacted = secretParamRegex.ReplaceAllString(redacted, "${1}${2}=REDACTED")
	}

	return redacted, hadSecret
}

func cleanHost(host string) string {
	// Remove templates
	host = regexp.MustCompile(`\{\{[^}]+\}\}`).ReplaceAllString(host, "")
	host = regexp.MustCompile(`\$\{[^}]+\}`).ReplaceAllString(host, "")
	// Clean brackets for IPv6 or specific formatting if necessary
	host = strings.Trim(host, "[]")
	return host
}

func ParseValue(raw string) []types.ConnectionEndpoint {
	var endpoints []types.ConnectionEndpoint

	// In-memory skips
	if strings.HasPrefix(raw, "jdbc:h2:mem:") || strings.HasPrefix(raw, "jdbc:sqlite::memory:") {
		return endpoints
	}

	// JDBC handling
	if strings.HasPrefix(raw, "jdbc:") {
		s := strings.TrimPrefix(raw, "jdbc:")

		if strings.HasPrefix(s, "oracle:thin:@//") {
			parts := strings.Split(strings.TrimPrefix(s, "oracle:thin:@//"), "/")
			hp := strings.Split(parts[0], ":")
			port := 1521
			if len(hp) > 1 {
				port, _ = strconv.Atoi(hp[1])
			}
			db := ""
			if len(parts) > 1 {
				db = strings.Split(parts[1], "?")[0]
			}
			endpoints = append(endpoints, types.ConnectionEndpoint{
				Host: cleanHost(hp[0]), Port: port, Database: db, Scheme: "jdbc",
			})
			return endpoints
		}
		if strings.HasPrefix(s, "oracle:thin:@") {
			// e.g. oracle:thin:@host:1521:sid
			parts := strings.Split(strings.TrimPrefix(s, "oracle:thin:@"), ":")
			if len(parts) >= 2 {
				port, _ := strconv.Atoi(parts[1])
				db := ""
				if len(parts) > 2 {
					db = parts[2]
				}
				endpoints = append(endpoints, types.ConnectionEndpoint{
					Host: cleanHost(parts[0]), Port: port, Database: db, Scheme: "jdbc",
				})
			}
			return endpoints
		}

		// normal jdbc://
		if idx := strings.Index(s, "://"); idx != -1 {
			schemePart := s[:idx]
			rest := s[idx+3:]

			dbPart := ""
			hostPart := rest
			if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
				hostPart = rest[:slashIdx]
				dbPart = strings.Split(rest[slashIdx+1:], "?")[0]
				dbPart = strings.Split(dbPart, ";")[0]
			} else if semiIdx := strings.Index(rest, ";"); semiIdx != -1 {
				hostPart = rest[:semiIdx]
				// Parse databaseName from properties
				for _, prop := range strings.Split(rest[semiIdx+1:], ";") {
					if strings.HasPrefix(strings.ToLower(prop), "databasename=") {
						dbPart = strings.Split(prop, "=")[1]
					}
				}
			}

			for _, h := range strings.Split(hostPart, ",") {
				hp := strings.Split(h, ":")
				port := 0
				if len(hp) > 1 {
					port, _ = strconv.Atoi(hp[1])
				}
				endpoints = append(endpoints, types.ConnectionEndpoint{
					Host: cleanHost(hp[0]), Port: port, Database: dbPart, Scheme: schemePart,
				})
			}
			return endpoints
		}
	}

	// Normal URL or comma separated lists
	if strings.Contains(raw, "://") {
		// Can be comma separated URLs or a single one, but standard URL shouldn't be comma separated
		// MongoDB can be mongodb://h1,h2/db
		u, err := url.Parse(raw)
		if err == nil {
			hosts := strings.Split(u.Host, ",")
			db := strings.TrimPrefix(u.Path, "/")
			for _, h := range hosts {
				hp := strings.Split(h, ":")
				port := 0
				if len(hp) > 1 {
					port, _ = strconv.Atoi(hp[1])
				}
				endpoints = append(endpoints, types.ConnectionEndpoint{
					Host: cleanHost(hp[0]), Port: port, Database: db, Scheme: u.Scheme, Path: u.Path,
				})
			}
			return endpoints
		}
	}

	// Comma separated bare host:port
	if strings.Contains(raw, ",") {
		for _, p := range strings.Split(raw, ",") {
			hp := strings.Split(p, ":")
			port := 0
			if len(hp) > 1 {
				port, _ = strconv.Atoi(hp[1])
			}
			endpoints = append(endpoints, types.ConnectionEndpoint{
				Host: cleanHost(hp[0]), Port: port,
			})
		}
		return endpoints
	}

	// Bare host:port
	hp := strings.Split(raw, ":")
	port := 0
	if len(hp) > 1 {
		port, _ = strconv.Atoi(hp[1])
	}
	endpoints = append(endpoints, types.ConnectionEndpoint{
		Host: cleanHost(hp[0]), Port: port,
	})

	return endpoints
}
