package synth

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"
)

// SpringProject is what dhg reads from a Spring Boot project.
type SpringProject struct {
	Name     string // spring.application.name, artifactId or rootProject.name
	Version  string
	Actuator bool
	// Properties are the flattened properties of application.yml/.properties
	// (documents without a profile; .properties win over .yml).
	Properties map[string]string
}

// FromSource describes the app built from a project directory: its
// Dockerfile and, for Spring Boot projects, the build file and
// application configuration. image is the app's image; empty derives
// "<name>:<version|latest>".
func FromSource(dir, image string, notes *Notes) (App, error) {
	var df *Dockerfile
	if data, err := os.ReadFile(filepath.Join(dir, "Dockerfile")); err == nil {
		d := ParseDockerfile(string(data))
		df = &d
	}
	spring, err := DetectSpring(dir)
	if err != nil {
		return App{}, err
	}
	if df == nil && spring == nil {
		return App{}, fmt.Errorf("%s: neither a Dockerfile nor a Spring Boot project (pom.xml or build.gradle with spring-boot)", dir)
	}

	name := filepath.Base(filepath.Clean(dir))
	version := ""
	if spring != nil {
		name, version = spring.Name, spring.Version
	}
	app := App{Name: DNSName(name)}
	app.Image = image
	if app.Image == "" {
		tag := version
		if tag == "" {
			tag = "latest"
		}
		app.Image = app.Name + ":" + tag
		notes.Addf("%s: no --image given; image set to %q, set the pushed image in values", app.Name, app.Image)
	}

	if df != nil {
		for _, p := range df.Expose {
			number, proto, _ := strings.Cut(p, "/")
			if n, err := strconv.ParseInt(number, 10, 64); err == nil && n > 0 && n < 65536 {
				app.AddPort(n, proto)
			}
		}
		for _, p := range df.Unresolved {
			notes.Addf("%s: EXPOSE %s uses a variable without a default; add the port in values", app.Name, p)
		}
		applyUser(&app, df.User, "Dockerfile", notes)
		for i, v := range df.Volumes {
			app.Volumes = append(app.Volumes, Volume{Name: "volume-" + strconv.Itoa(i+1), MountPath: v, Kind: VolumeEmptyDir})
			notes.Addf("%s: Dockerfile VOLUME %s is an emptyDir (lost on restart); make it persistent if it holds data", app.Name, v)
		}
		if p := healthcheckProbe(df.Healthcheck, df.Interval, df.Timeout, df.StartPeriod, df.Retries); p != nil {
			app.Liveness, app.Readiness = p, copyProbe(p)
		}
	} else {
		notes.Addf("%s: no Dockerfile, so the container user is unknown; set runAsUser/runAsNonRoot in values", app.Name)
	}

	if spring != nil {
		applySpring(&app, spring, notes)
	}
	if app.Liveness == nil {
		notes.Addf("%s: no health endpoint found (no actuator, no HEALTHCHECK); add liveness and readiness probes", app.Name)
	}
	if len(app.Ports) == 0 {
		notes.Addf("%s: no port found, so no Service was created", app.Name)
	}
	return app, nil
}

// applySpring adds what the Spring Boot configuration tells: the HTTP port,
// actuator probes and the connection settings of external services.
func applySpring(app *App, p *SpringProject, notes *Notes) {
	prop := func(key string) (string, bool) {
		v, ok := p.Properties[key]
		if !ok {
			return "", false
		}
		resolved, unresolved := resolvePlaceholders(v)
		if unresolved != "" {
			notes.Addf("%s: %s = %s needs %s at runtime; set it in values", app.Name, key, v, unresolved)
		}
		return resolved, true
	}

	port := int64(8080)
	if v, ok := prop("server.port"); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			port = n
		}
	}
	app.AddNamedPort("http", port, "TCP")

	if p.Actuator {
		mgmtPort := port
		if v, ok := prop("management.server.port"); ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				mgmtPort = n
				app.AddNamedPort("management", n, "TCP")
			}
		}
		base := "/actuator"
		if v, ok := prop("management.endpoints.web.base-path"); ok {
			base = "/" + strings.Trim(v, "/")
			if base == "/" {
				base = ""
			}
		}
		if ctx, ok := prop("server.servlet.context-path"); ok && mgmtPort == port {
			if ctx = strings.Trim(ctx, "/"); ctx != "" {
				base = "/" + ctx + base
			}
		}
		app.Liveness = &Probe{HTTPPath: base + "/health/liveness", Port: mgmtPort, PeriodSeconds: 10, FailureThreshold: 3}
		app.Readiness = &Probe{HTTPPath: base + "/health/readiness", Port: mgmtPort, PeriodSeconds: 10, FailureThreshold: 3}
	} else {
		notes.Addf("%s: Spring Boot without spring-boot-starter-actuator; add it to get /actuator/health/liveness and /readiness probes", app.Name)
	}

	for _, m := range []struct{ property, env string }{
		{"spring.datasource.url", "SPRING_DATASOURCE_URL"},
		{"spring.datasource.username", "SPRING_DATASOURCE_USERNAME"},
		{"spring.kafka.bootstrap-servers", "SPRING_KAFKA_BOOTSTRAP_SERVERS"},
		{"spring.security.oauth2.resourceserver.jwt.issuer-uri", "SPRING_SECURITY_OAUTH2_RESOURCESERVER_JWT_ISSUER_URI"},
	} {
		if v, ok := prop(m.property); ok {
			app.SetEnv(m.env, v)
		}
	}
	if _, ok := p.Properties["spring.datasource.password"]; ok {
		app.SetEnv("SPRING_DATASOURCE_PASSWORD", "")
		notes.Addf("%s: SPRING_DATASOURCE_PASSWORD is empty in Secret %s-env; set it (or use --with external-secrets)", app.Name, app.Name)
	}
	if _, ok := p.Properties["spring.security.oauth2.resourceserver.jwt.issuer-uri"]; ok {
		notes.Addf("%s: OAuth2 issuer (Keycloak) comes from SPRING_SECURITY_OAUTH2_RESOURCESERVER_JWT_ISSUER_URI; point it at the realm of each environment", app.Name)
	}
}

var placeholder = regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)

// resolvePlaceholders replaces ${NAME:default} by its default; it returns
// the names of placeholders without a default.
func resolvePlaceholders(v string) (string, string) {
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

// DetectSpring reads a Spring Boot project in dir; it returns nil when the
// build file does not use Spring Boot.
func DetectSpring(dir string) (*SpringProject, error) {
	p := &SpringProject{Properties: map[string]string{}}
	found := false

	if data, err := os.ReadFile(filepath.Join(dir, "pom.xml")); err == nil && strings.Contains(string(data), "spring-boot") {
		found = true
		var pom struct {
			ArtifactID string `xml:"artifactId"`
			Version    string `xml:"version"`
			Parent     struct {
				Version string `xml:"version"`
			} `xml:"parent"`
			Dependencies []struct {
				ArtifactID string `xml:"artifactId"`
			} `xml:"dependencies>dependency"`
		}
		if err := xml.Unmarshal(data, &pom); err != nil {
			return nil, fmt.Errorf("pom.xml: %w", err)
		}
		p.Name, p.Version = pom.ArtifactID, pom.Version
		if p.Version == "" {
			p.Version = pom.Parent.Version
		}
		for _, d := range pom.Dependencies {
			if d.ArtifactID == "spring-boot-starter-actuator" {
				p.Actuator = true
			}
		}
	}
	for _, f := range []string{"build.gradle", "build.gradle.kts"} {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil || (!strings.Contains(string(data), "org.springframework.boot") && !strings.Contains(string(data), "spring-boot")) {
			continue
		}
		found = true
		p.Actuator = p.Actuator || strings.Contains(string(data), "spring-boot-starter-actuator")
		if m := regexp.MustCompile(`(?m)^\s*version\s*=\s*["']([^"']+)["']`).FindStringSubmatch(string(data)); m != nil && p.Version == "" {
			p.Version = m[1]
		}
		for _, s := range []string{"settings.gradle", "settings.gradle.kts"} {
			if sd, err := os.ReadFile(filepath.Join(dir, s)); err == nil {
				if m := regexp.MustCompile(`rootProject\.name\s*=\s*["']([^"']+)["']`).FindStringSubmatch(string(sd)); m != nil && p.Name == "" {
					p.Name = m[1]
				}
			}
		}
	}
	if !found {
		return nil, nil
	}

	resources := filepath.Join(dir, "src", "main", "resources")
	for _, f := range []string{"application.yml", "application.yaml"} {
		if data, err := os.ReadFile(filepath.Join(resources, f)); err == nil {
			if err := readSpringYAML(string(data), p.Properties); err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(resources, "application.properties")); err == nil {
		readProperties(string(data), p.Properties)
	}
	if v := p.Properties["spring.application.name"]; v != "" {
		if resolved, missing := resolvePlaceholders(v); missing == "" {
			p.Name = resolved
		}
	}
	if p.Name == "" {
		p.Name = filepath.Base(filepath.Clean(dir))
	}
	if strings.Contains(p.Version, "${") {
		p.Version = ""
	}
	return p, nil
}

// readSpringYAML flattens the documents of application.yml that do not
// activate on a profile.
func readSpringYAML(content string, into map[string]string) error {
	for _, doc := range regexp.MustCompile(`(?m)^---\s*$`).Split(content, -1) {
		var m map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &m); err != nil {
			return err
		}
		flat := map[string]string{}
		flatten("", m, flat)
		if flat["spring.config.activate.on-profile"] != "" || flat["spring.profiles"] != "" {
			continue
		}
		for k, v := range flat {
			into[k] = v
		}
	}
	return nil
}

func flatten(prefix string, v interface{}, into map[string]string) {
	switch t := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flatten(key, t[k], into)
		}
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			parts = append(parts, scalar(x))
		}
		into[prefix] = strings.Join(parts, ",")
	default:
		into[prefix] = scalar(t)
	}
}

// readProperties reads key=value (or key: value) lines.
func readProperties(content string, into map[string]string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		i := strings.IndexAny(line, "=:")
		if i < 0 {
			continue
		}
		into[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
	}
}
