package synth

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// config reads the properties of one framework project for one app.
type config struct {
	app   *App
	props map[string]string
	notes *Notes

	// fallback holds defaults for ${NAME} placeholders that have none of
	// their own, from an environment template next to the sources.
	// fallbackFrom names the files, for a note.
	fallback     map[string]string
	fallbackFrom string
	// known maps the names of variables the platform provides to the source
	// that provides them (see knownIndex).
	known map[string]string
}

// bareBrace matches "{NAME}" without the "$": a typo of a placeholder that
// would otherwise be copied as text.
var bareBrace = regexp.MustCompile(`\{[A-Z][A-Z0-9_]*\}`)

// lookup returns a property with ${NAME:default} placeholders resolved and
// the names of those without a default, which only the runtime can resolve.
func (c *config) lookup(key string) (resolved, unresolved string, ok bool) {
	v, ok := c.props[key]
	if !ok {
		return "", "", false
	}
	resolved, unresolved = ResolvePlaceholdersWith(v, c.fallback)
	if unresolved == "" {
		if typo := bareBrace.FindAllString(resolved, -1); len(typo) > 0 {
			unresolved = strings.Join(typo, ", ")
		}
	}
	return resolved, unresolved, true
}

// get returns a property with ${NAME:default} placeholders resolved and
// reports placeholders that only the runtime can resolve.
func (c *config) get(key string) (string, bool) {
	resolved, unresolved, ok := c.lookup(key)
	if !ok {
		return "", false
	}
	if unresolved != "" {
		c.notes.Addf("%s: %s = %s needs %s at runtime; set it in values", c.app.Name, key, c.props[key], unresolved)
	}
	return resolved, true
}

// str returns a non-empty property or def.
func (c *config) str(key, def string) string {
	if v, ok := c.get(key); ok && v != "" {
		return v
	}
	return def
}

// port returns a fixed port number or def. Port 0 and -1 ask the
// framework for a random port, which a Service cannot target.
func (c *config) port(key string, def int64) int64 {
	v, ok := c.get(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 || n > 65535 {
		c.notes.Addf("%s: %s = %s is not a fixed port; the chart uses %d, set the real port in values", c.app.Name, key, v, def)
		return def
	}
	return n
}

// env copies a property into an environment variable of the app. A value
// that still needs a runtime placeholder is not copied, not even partly
// resolved: an empty or half-built variable would override the property.
// The placeholders themselves become variables (see flush).
func (c *config) env(key string) {
	v, unresolved, ok := c.lookup(key)
	if !ok || unresolved != "" {
		return
	}
	c.app.SetEnv(envName(key), v)
}

// maxListed caps the names a note lists.
const maxListed = 20

func listed(names []string) string {
	if len(names) > maxListed {
		return strings.Join(names[:maxListed], ", ") + fmt.Sprintf(", ... (%d more)", len(names)-maxListed)
	}
	return strings.Join(names, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// flush turns every ${NAME} placeholder of the configuration that has no
// default of its own into a variable of the app: with the value of the
// environment template when there is one (never for a credential), else
// empty. The empty ones are required and listed in one note. A variable
// that already exists is kept.
func (c *config) flush() {
	seen := map[string]bool{}
	var names []string
	var typos []string
	keys := make([]string, 0, len(c.props))
	for k := range c.props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := c.props[k]
		for _, n := range Placeholders(v) {
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
		if resolved, _ := ResolvePlaceholders(v); bareBrace.MatchString(resolved) {
			typos = append(typos, k+" ("+strings.Join(bareBrace.FindAllString(resolved, -1), ", ")+")")
		}
	}
	sort.Strings(names)

	var empty, secrets []string
	fromTemplate := 0
	provided := map[string][]string{}
	for _, n := range names {
		if c.app.hasEnv(n) {
			continue
		}
		if source, ok := c.known[n]; ok {
			provided[source] = append(provided[source], n)
			continue
		}
		value := ""
		if fb, ok := c.fallback[n]; ok {
			value = fb
			fromTemplate++
		}
		c.app.SetEnv(n, value)
		if value == "" {
			empty = append(empty, n)
			if c.app.secrets.Match(n) {
				secrets = append(secrets, n)
			}
		}
	}
	providers := make([]string, 0, len(provided))
	for source := range provided {
		providers = append(providers, source)
	}
	sort.Strings(providers)
	for _, source := range providers {
		verb := "come"
		if len(provided[source]) == 1 {
			verb = "comes"
		}
		c.notes.Addf("%s: %s %s from %s (%s), so the chart does not set them", c.app.Name, plural(len(provided[source]), "variable", "variables"), verb, source, listed(provided[source]))
	}
	if len(empty) > 0 {
		verb := "are"
		if len(empty) == 1 {
			verb = "is"
		}
		extra := ""
		if len(secrets) > 0 {
			extra = fmt.Sprintf(" (in Secret %s-env: %s)", c.app.Name, listed(secrets))
		}
		c.notes.Addf("%s: %s %s empty and required (%s): the configuration has no default for them%s; set them in values",
			c.app.Name, plural(len(empty), "environment variable", "environment variables"), verb, listed(empty), extra)
	}
	if fromTemplate > 0 {
		verb := "take"
		if fromTemplate == 1 {
			verb = "takes"
		}
		c.notes.Addf("%s: %s %s the value from %s; check it for the target environment", c.app.Name, plural(fromTemplate, "environment variable", "environment variables"), verb, c.fallbackFrom)
	}
	if len(typos) > 0 {
		c.notes.Addf("%s: {NAME} without $ (a typo for ${NAME}?) in %s, so the value is not copied: %s", c.app.Name, plural(len(typos), "property", "properties"), listed(typos))
	}
}

// secret adds an empty Secret key for a credential the configuration sets:
// the value itself never goes into the chart. A credential that only a
// placeholder sets is the placeholder's variable (see flush).
func (c *config) secret(key string) {
	if _, unresolved, ok := c.lookup(key); ok && unresolved == "" {
		name := envName(key)
		c.app.SetEnv(name, "")
		c.notes.Addf("%s: %s is empty in Secret %s-env; set it (or use --with external-secrets)", c.app.Name, name, c.app.Name)
	}
}

// issuer copies an OIDC issuer and reminds that it differs per environment.
func (c *config) issuer(key string) {
	if _, ok := c.props[key]; ok {
		c.env(key)
		c.notes.Addf("%s: OAuth2 issuer (Keycloak) comes from %s; point it at the realm of each environment", c.app.Name, envName(key))
	}
}

// envName is the environment variable that sets a property: upper case,
// every other character than a letter or digit replaced by "_". Spring
// Boot (its legacy form), MicroProfile Config (Quarkus) and Micronaut
// all bind it.
func envName(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 'a' + 'A')
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// resolvePath joins p to parent unless p is absolute, the rule Quarkus
// applies to its HTTP paths.
func resolvePath(parent, p string) string {
	if strings.HasPrefix(p, "/") {
		return path.Clean(p)
	}
	return path.Join(parent, p)
}

// httpProbes sets the probes of a JVM app. A JVM app can take minutes to
// start, so liveness and readiness only begin once the startup probe
// (liveness path, 30 x 10 s) has succeeded.
func httpProbes(app *App, port int64, liveness, readiness string) {
	app.Liveness = &Probe{HTTPPath: liveness, Port: port, PeriodSeconds: 10, FailureThreshold: 3}
	app.Readiness = &Probe{HTTPPath: readiness, Port: port, PeriodSeconds: 10, FailureThreshold: 3}
	app.Startup = &Probe{HTTPPath: liveness, Port: port, PeriodSeconds: 10, FailureThreshold: 30}
}

// applySpring adds what the Spring Boot configuration tells: the HTTP port,
// Actuator probes and the connection settings of external services.
func applySpring(c *config, p *Project) {
	port := c.port("server.port", 8080)
	c.app.AddNamedPort("http", port, "TCP")

	if p.Health {
		mgmt := c.port("management.server.port", port)
		if mgmt != port {
			c.app.AddNamedPort("management", mgmt, "TCP")
		}
		base := "/actuator"
		if v, ok := c.get("management.endpoints.web.base-path"); ok {
			base = "/" + strings.Trim(v, "/")
		}
		// The servlet context path does not apply to a separate management port.
		ctx := ""
		if mgmt == port {
			ctx = c.str("server.servlet.context-path", "")
		}
		httpProbes(c.app, mgmt, path.Join("/", ctx, base, "health/liveness"), path.Join("/", ctx, base, "health/readiness"))
	} else {
		c.notes.Addf("%s: Spring Boot without spring-boot-starter-actuator; add it to get /actuator/health/liveness and /readiness probes", c.app.Name)
	}

	for _, key := range []string{"spring.datasource.url", "spring.datasource.username", "spring.kafka.bootstrap-servers"} {
		c.env(key)
	}
	c.secret("spring.datasource.password")
	c.issuer("spring.security.oauth2.resourceserver.jwt.issuer-uri")
}

// applyQuarkus adds the HTTP port, SmallRye Health probes and connection
// settings. Quarkus serves health under the non-application root
// (default "q", relative to quarkus.http.root-path), or under the
// management interface root on its own port (default 9000) when
// quarkus.management.enabled is true.
func applyQuarkus(c *config, p *Project) {
	port := c.port("quarkus.http.port", 8080)
	c.app.AddNamedPort("http", port, "TCP")

	if p.Health {
		probePort := port
		var root string
		if c.str("quarkus.management.enabled", "false") == "true" {
			probePort = c.port("quarkus.management.port", 9000)
			c.app.AddNamedPort("management", probePort, "TCP")
			root = resolvePath("/", c.str("quarkus.management.root-path", "q"))
		} else {
			httpRoot := resolvePath("/", c.str("quarkus.http.root-path", "/"))
			root = resolvePath(httpRoot, c.str("quarkus.http.non-application-root-path", "q"))
		}
		health := resolvePath(root, c.str("quarkus.smallrye-health.root-path", "health"))
		httpProbes(c.app, probePort,
			resolvePath(health, c.str("quarkus.smallrye-health.liveness-path", "live")),
			resolvePath(health, c.str("quarkus.smallrye-health.readiness-path", "ready")))
	} else {
		c.notes.Addf("%s: Quarkus without quarkus-smallrye-health; add it to get /q/health/live and /q/health/ready probes", c.app.Name)
	}

	for _, key := range []string{"quarkus.datasource.jdbc.url", "quarkus.datasource.reactive.url", "quarkus.datasource.username", "kafka.bootstrap.servers"} {
		c.env(key)
	}
	c.secret("quarkus.datasource.password")
	c.secret("quarkus.oidc.credentials.secret")
	c.issuer("quarkus.oidc.auth-server-url")
}

var micronautIssuer = regexp.MustCompile(`^micronaut\.security\.oauth2\.clients\.[^.]+\.openid\.issuer$`)
var micronautClientSecret = regexp.MustCompile(`^micronaut\.security\.oauth2\.clients\.[^.]+\.client-secret$`)

// applyMicronaut adds the HTTP port, Micronaut Management probes
// (/health/liveness and /health/readiness under endpoints.all.path) and
// connection settings.
func applyMicronaut(c *config, p *Project) {
	port := c.port("micronaut.server.port", 8080)
	c.app.AddNamedPort("http", port, "TCP")

	enabled := c.str("endpoints.all.enabled", "true") != "false" && c.str("endpoints.health.enabled", "true") != "false"
	switch {
	case !p.Health:
		c.notes.Addf("%s: Micronaut without micronaut-management; add it to get /health/liveness and /health/readiness probes", c.app.Name)
	case !enabled:
		c.notes.Addf("%s: the health endpoint is disabled (endpoints.*.enabled = false), so the chart has no probes", c.app.Name)
	default:
		probePort := port
		if _, ok := c.props["endpoints.all.port"]; ok {
			probePort = c.port("endpoints.all.port", port)
			if probePort != port {
				c.app.AddNamedPort("management", probePort, "TCP")
				c.notes.Addf("%s: management endpoints listen on port %d; check that the probe paths match the context path there", c.app.Name, probePort)
			}
		}
		base := path.Join("/", c.str("micronaut.server.context-path", ""), c.str("endpoints.all.path", "/"))
		httpProbes(c.app, probePort, path.Join(base, "health/liveness"), path.Join(base, "health/readiness"))
	}

	for _, key := range []string{"datasources.default.url", "datasources.default.username", "kafka.bootstrap.servers"} {
		c.env(key)
	}
	c.secret("datasources.default.password")
	keys := make([]string, 0, len(c.props))
	for k := range c.props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch {
		case micronautIssuer.MatchString(k):
			c.issuer(k)
		case micronautClientSecret.MatchString(k):
			c.secret(k)
		}
	}
}
