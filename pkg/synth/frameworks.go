package synth

import (
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
}

// get returns a property with ${NAME:default} placeholders resolved and
// reports placeholders that only the runtime can resolve.
func (c *config) get(key string) (string, bool) {
	v, ok := c.props[key]
	if !ok {
		return "", false
	}
	resolved, unresolved := resolvePlaceholders(v)
	if unresolved != "" {
		c.notes.Addf("%s: %s = %s needs %s at runtime; set it in values", c.app.Name, key, v, unresolved)
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

// env copies a property into an environment variable of the app.
func (c *config) env(key string) {
	if v, ok := c.get(key); ok {
		c.app.SetEnv(envName(key), v)
	}
}

// secret adds an empty Secret key for a credential the configuration sets:
// the value itself never goes into the chart.
func (c *config) secret(key string) {
	if _, ok := c.props[key]; ok {
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

func httpProbes(app *App, port int64, liveness, readiness string) {
	app.Liveness = &Probe{HTTPPath: liveness, Port: port, PeriodSeconds: 10, FailureThreshold: 3}
	app.Readiness = &Probe{HTTPPath: readiness, Port: port, PeriodSeconds: 10, FailureThreshold: 3}
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
