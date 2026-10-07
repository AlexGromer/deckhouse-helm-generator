package synth

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

// Framework is a JVM framework whose configuration dhg reads.
type Framework string

// Supported frameworks.
const (
	Spring    Framework = "Spring Boot"
	Quarkus   Framework = "Quarkus"
	Micronaut Framework = "Micronaut"
)

// framework describes how to recognise a framework in the build file.
type framework struct {
	kind Framework
	// markers are build file substrings that identify the framework.
	markers []string
	// health is the dependency that serves the health endpoints.
	health string
	// nameKey is the property holding the application name.
	nameKey string
	// profileEnv selects the active profile at runtime.
	profileEnv string
}

// Quarkus and Micronaut come first: their builds may pull Spring
// compatibility artifacts whose names contain "spring-boot".
var frameworks = []framework{
	{Quarkus, []string{"io.quarkus"}, "quarkus-smallrye-health", "quarkus.application.name", "QUARKUS_PROFILE"},
	{Micronaut, []string{"io.micronaut"}, "micronaut-management", "micronaut.application.name", "MICRONAUT_ENVIRONMENTS"},
	{Spring, []string{"org.springframework.boot", "spring-boot"}, "spring-boot-starter-actuator", "spring.application.name", "SPRING_PROFILES_ACTIVE"},
}

// Project is what dhg reads from a JVM project: build file and
// src/main/resources/application*.{yml,yaml,properties}.
type Project struct {
	Framework Framework
	Name      string // application name property, artifactId or rootProject.name
	Version   string
	// Health reports the framework's health dependency (Spring Boot
	// Actuator, SmallRye Health, Micronaut Management).
	Health bool
	// Properties are the flattened properties in effect without a profile.
	Properties map[string]string
	// Profiles holds the properties each profile adds or overrides.
	Profiles map[string]map[string]string

	fw framework
}

// DetectProject reads a Spring Boot, Quarkus or Micronaut project in dir; it
// returns nil when the build file uses none of them.
func DetectProject(dir string, notes *Notes) (*Project, error) {
	pom, _ := os.ReadFile(filepath.Join(dir, "pom.xml"))
	var gradle []byte
	for _, f := range []string{"build.gradle", "build.gradle.kts"} {
		if data, err := os.ReadFile(filepath.Join(dir, f)); err == nil {
			gradle = data
			break
		}
	}
	build := string(pom) + "\n" + string(gradle)
	p := &Project{Properties: map[string]string{}, Profiles: map[string]map[string]string{}}
	for _, fw := range frameworks {
		for _, m := range fw.markers {
			if strings.Contains(build, m) {
				p.fw = fw
				break
			}
		}
		if p.fw.kind != "" {
			break
		}
	}
	if p.fw.kind == "" {
		return nil, nil
	}
	p.Framework = p.fw.kind
	p.Health = strings.Contains(build, p.fw.health)

	if len(pom) > 0 {
		var m struct {
			ArtifactID string `xml:"artifactId"`
			Version    string `xml:"version"`
			Parent     struct {
				Version string `xml:"version"`
			} `xml:"parent"`
		}
		if err := xml.Unmarshal(pom, &m); err != nil {
			return nil, fmt.Errorf("pom.xml: %w", err)
		}
		// Maven inherits the version from the parent when the project has none.
		p.Name, p.Version = m.ArtifactID, m.Version
		if p.Version == "" {
			p.Version = m.Parent.Version
		}
	}
	if len(gradle) > 0 {
		if m := regexp.MustCompile(`(?m)^\s*version\s*=\s*["']([^"']+)["']`).FindSubmatch(gradle); m != nil && p.Version == "" {
			p.Version = string(m[1])
		}
		for _, s := range []string{"settings.gradle", "settings.gradle.kts"} {
			if sd, err := os.ReadFile(filepath.Join(dir, s)); err == nil {
				if m := regexp.MustCompile(`rootProject\.name\s*=\s*["']([^"']+)["']`).FindSubmatch(sd); m != nil && p.Name == "" {
					p.Name = string(m[1])
				}
			}
		}
	}

	resources := filepath.Join(dir, "src", "main", "resources")
	var err error
	switch p.Framework {
	case Spring:
		err = p.loadSpring(resources, notes)
	case Quarkus:
		err = p.loadQuarkus(resources, notes)
	case Micronaut:
		err = p.loadMicronaut(resources, notes)
	}
	if err != nil {
		return nil, err
	}

	if v := p.Properties[p.fw.nameKey]; v != "" {
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

// ProfileNames returns the profiles in sorted order.
func (p *Project) ProfileNames() []string {
	names := make([]string, 0, len(p.Profiles))
	for n := range p.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// properties returns the properties in effect with profile active.
func (p *Project) properties(profile string) map[string]string {
	out := make(map[string]string, len(p.Properties))
	for k, v := range p.Properties {
		out[k] = v
	}
	for k, v := range p.Profiles[profile] {
		out[k] = v
	}
	return out
}

func (p *Project) addProfile(name string, props map[string]string) {
	if p.Profiles[name] == nil {
		p.Profiles[name] = map[string]string{}
	}
	for k, v := range props {
		p.Profiles[name][k] = v
	}
}

// loadSpring follows Spring Boot: application.properties overrides
// application.yml; a document with spring.config.activate.on-profile (or
// the legacy spring.profiles) belongs to that profile, as do the
// application-<profile>.* files.
func (p *Project) loadSpring(dir string, notes *Notes) error {
	for _, f := range []string{"application.yml", "application.yaml", "application.properties"} {
		docs, err := readConfig(filepath.Join(dir, f))
		if err != nil {
			return err
		}
		for _, doc := range docs {
			on := doc["spring.config.activate.on-profile"]
			if on == "" {
				on = doc["spring.profiles"]
			}
			delete(doc, "spring.config.activate.on-profile")
			delete(doc, "spring.profiles")
			if on == "" {
				mergeInto(p.Properties, doc)
				continue
			}
			if strings.ContainsAny(on, "&|!()") {
				notes.Addf("%s: profile expression %q in %s is not mapped to a values file", p.Name, on, f)
				continue
			}
			for _, name := range strings.Split(on, ",") {
				p.addProfile(strings.TrimSpace(name), doc)
			}
		}
	}
	return p.loadProfileFiles(dir)
}

// loadQuarkus follows Quarkus: application.yaml (ordinal 255) overrides
// application.properties (250); "%<profile>." keys belong to a profile.
// Outside dev mode and tests the active profile is prod, so %prod keys
// apply to the container; %dev and %test do not.
func (p *Project) loadQuarkus(dir string, notes *Notes) error {
	flat := map[string]string{}
	for _, f := range []string{"application.properties", "application.yml", "application.yaml"} {
		docs, err := readConfig(filepath.Join(dir, f))
		if err != nil {
			return err
		}
		for _, doc := range docs {
			mergeInto(flat, doc)
		}
	}
	if err := p.loadProfileFiles(dir); err != nil {
		return err
	}
	prod := p.Profiles["prod"]
	delete(p.Profiles, "prod")
	skipped := map[string]bool{}
	for k, v := range flat {
		if !strings.HasPrefix(k, "%") {
			p.Properties[k] = v
			continue
		}
		names, key, ok := strings.Cut(k[1:], ".")
		if !ok {
			continue
		}
		for _, name := range strings.Split(names, ",") {
			switch name {
			case "prod":
				if prod == nil {
					prod = map[string]string{}
				}
				prod[key] = v
			case "dev", "test":
				skipped[name] = true
			default:
				p.addProfile(name, map[string]string{key: v})
			}
		}
	}
	mergeInto(p.Properties, prod)
	if len(skipped) > 0 {
		notes.Addf("%s: %%dev and %%test properties apply to Quarkus dev mode and tests, not to the container; they are not mapped", p.Name)
	}
	return nil
}

// loadMicronaut reads application.* and the application-<environment>.*
// files of Micronaut environments. Micronaut documents no order between a
// .yml and a .properties file, so keys set differently in both are
// reported.
func (p *Project) loadMicronaut(dir string, notes *Notes) error {
	seen := map[string]string{}
	for _, f := range []string{"application.yml", "application.yaml", "application.properties"} {
		docs, err := readConfig(filepath.Join(dir, f))
		if err != nil {
			return err
		}
		for _, doc := range docs {
			for k, v := range doc {
				if prev, ok := p.Properties[k]; ok && prev != v && seen[k] != f {
					notes.Addf("%s: %s is %q in %s and %q in %s; check which one Micronaut uses", p.Name, k, prev, seen[k], v, f)
				}
				p.Properties[k], seen[k] = v, f
			}
		}
	}
	return p.loadProfileFiles(dir)
}

var profileFile = regexp.MustCompile(`^application-([A-Za-z0-9_.-]+)\.(yml|yaml|properties)$`)

// loadProfileFiles reads application-<profile>.{yml,yaml,properties}.
func (p *Project) loadProfileFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		m := profileFile.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() {
			continue
		}
		docs, err := readConfig(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		for _, doc := range docs {
			p.addProfile(m[1], doc)
		}
	}
	return nil
}

var (
	yamlDocSeparator  = regexp.MustCompile(`(?m)^---\s*$`)
	propsDocSeparator = regexp.MustCompile(`(?m)^[#!]---\s*$`)
)

// readConfig returns the flattened documents of a .yml/.yaml or
// .properties file; a missing file has none. Properties files may hold
// several documents separated by "#---" (Spring Boot 2.4+).
func readConfig(path string) ([]map[string]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var docs []map[string]string
	if strings.HasSuffix(path, ".properties") {
		for _, doc := range propsDocSeparator.Split(string(data), -1) {
			flat := map[string]string{}
			readProperties(doc, flat)
			docs = append(docs, flat)
		}
		return docs, nil
	}
	// YAML 1.1 like SnakeYAML, which Spring Boot uses.
	for _, doc := range yamlDocSeparator.Split(string(data), -1) {
		var m map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &m); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		flat := map[string]string{}
		flatten("", m, flat)
		docs = append(docs, flat)
	}
	return docs, nil
}

func mergeInto(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
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
	case nil:
		if prefix != "" {
			into[prefix] = ""
		}
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
