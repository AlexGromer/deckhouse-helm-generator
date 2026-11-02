package synth

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry"
)

// SourceProject is a project directory: its Dockerfile and, for Spring
// Boot, Quarkus and Micronaut, the build file and configuration.
type SourceProject struct {
	Dir        string
	Dockerfile *Dockerfile
	Project    *Project
	// ImageConfig, when set, is the configuration of the built image. It
	// replaces the Dockerfile as the source of ports, user, volumes and
	// health check: the image also carries what its base image sets.
	ImageConfig *registry.ImageConfig
}

// ReadSource reads a project directory. It fails when the directory holds
// neither a Dockerfile nor a supported framework project.
//
// The root of a Maven or Gradle multi-module build has no Dockerfile of its
// own: the application is the one module that has a Dockerfile or a
// framework project. ReadSource reads that module and says so in notes; with
// several such modules it fails and lists them.
func ReadSource(dir string, notes *Notes) (*SourceProject, error) {
	s := &SourceProject{Dir: dir}
	if data, err := os.ReadFile(filepath.Join(dir, "Dockerfile")); err == nil {
		d := ParseDockerfile(string(data))
		s.Dockerfile = &d
	}
	p, err := DetectProject(dir, notes)
	if err != nil {
		return nil, err
	}
	s.Project = p
	// A root with a framework project of its own (a Gradle app with library
	// subprojects) is the application; a Maven parent pom is not.
	if s.Dockerfile == nil && (p == nil || isParentPom(dir)) {
		apps := appModules(dir, 0)
		switch {
		case len(apps) > 1 && p != nil:
			// The root is a project itself: keep it, and say how to pick a module.
			notes.Addf("%s: several modules could be the application (%s); using the root project, pass one of them to choose, e.g. -f %s", filepath.Base(filepath.Clean(dir)), strings.Join(apps, ", "), filepath.Join(dir, apps[0]))
		case len(apps) > 1:
			return nil, fmt.Errorf("%s: several modules could be the application (%s); pass one of them, e.g. -f %s", dir, strings.Join(apps, ", "), filepath.Join(dir, apps[0]))
		case len(apps) == 1:
			notes.Addf("%s: no Dockerfile at the build root; using module %s", filepath.Base(filepath.Clean(dir)), apps[0])
			return ReadSource(filepath.Join(dir, apps[0]), notes)
		}
	}
	if s.Dockerfile == nil && p == nil {
		return nil, fmt.Errorf("%s: neither a Dockerfile nor a Spring Boot, Quarkus or Micronaut project (pom.xml or build.gradle)", dir)
	}
	return s, nil
}

// maxModuleDepth bounds the search through nested modules.
const maxModuleDepth = 3

var (
	gradleInclude = regexp.MustCompile(`(?m)^\s*include\b\s*\(?([^\n)]*)`)
	quoted        = regexp.MustCompile(`["']([^"']+)["']`)
)

// modules lists the module directories of the Maven or Gradle build in dir,
// relative to it.
func modules(dir string) []string {
	var out []string
	if data, err := os.ReadFile(filepath.Join(dir, "pom.xml")); err == nil {
		var m struct {
			Modules []string `xml:"modules>module"`
		}
		if xml.Unmarshal(data, &m) == nil {
			for _, name := range m.Modules {
				out = append(out, strings.TrimSpace(name))
			}
		}
	}
	for _, f := range []string{"settings.gradle", "settings.gradle.kts"} {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		for _, inc := range gradleInclude.FindAllSubmatch(data, -1) {
			for _, q := range quoted.FindAllSubmatch(inc[1], -1) {
				// ":libs:common" is the directory libs/common.
				out = append(out, strings.ReplaceAll(strings.Trim(string(q[1]), ":"), ":", "/"))
			}
		}
	}
	return out
}

// isParentPom reports whether dir holds a Maven pom with packaging "pom".
func isParentPom(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "pom.xml"))
	if err != nil {
		return false
	}
	var m struct {
		Packaging string `xml:"packaging"`
	}
	return xml.Unmarshal(data, &m) == nil && strings.TrimSpace(m.Packaging) == "pom"
}

// Build-file markers of an executable application: the framework packaging
// plugin. A starter dependency is no signal, a library module has one too.
var (
	pomAppPlugin    = regexp.MustCompile(`spring-boot-maven-plugin|quarkus-maven-plugin|micronaut-maven-plugin`)
	gradleAppPlugin = regexp.MustCompile(`["'](org\.springframework\.boot|io\.quarkus|io\.micronaut\.application)["']`)
)

// isAppModule reports whether the framework project in dir is an executable
// application: a packaging plugin in its build file or an application
// configuration file.
func isAppModule(dir string) bool {
	if data, err := os.ReadFile(filepath.Join(dir, "pom.xml")); err == nil && pomAppPlugin.Match(data) {
		return true
	}
	for _, f := range []string{"build.gradle", "build.gradle.kts"} {
		if data, err := os.ReadFile(filepath.Join(dir, f)); err == nil && gradleAppPlugin.Match(data) {
			return true
		}
	}
	for _, name := range []string{"application.yml", "application.yaml", "application.properties"} {
		if _, err := os.Stat(filepath.Join(dir, "src", "main", "resources", name)); err == nil {
			return true
		}
	}
	return false
}

// appModules returns the modules below dir (relative paths, in build order)
// that are an application: a Dockerfile, or a framework project with an
// executable-app signal (see isAppModule). A module that has application
// modules itself is searched, not counted.
func appModules(dir string, depth int) []string {
	if depth >= maxModuleDepth {
		return nil
	}
	var apps []string
	seen := map[string]bool{}
	for _, m := range modules(dir) {
		sub := filepath.Join(dir, filepath.FromSlash(m))
		if m == "" || seen[m] || !filepath.IsLocal(filepath.FromSlash(m)) {
			continue
		}
		seen[m] = true
		if info, err := os.Stat(sub); err != nil || !info.IsDir() {
			continue
		}
		if nested := appModules(sub, depth+1); len(nested) > 0 {
			for _, n := range nested {
				apps = append(apps, filepath.ToSlash(filepath.Join(m, n)))
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(sub, "Dockerfile")); err == nil {
			apps = append(apps, m)
			continue
		}
		// A module that cannot be read is no candidate; the notes of this probe are dropped.
		if p, err := DetectProject(sub, &Notes{}); err == nil && p != nil && isAppModule(sub) {
			apps = append(apps, m)
		}
	}
	return apps
}

// Name is the application name: from the framework configuration or the
// build file, else the directory.
func (s *SourceProject) Name() string {
	if s.Project != nil {
		return DNSName(s.Project.Name)
	}
	return DNSName(filepath.Base(filepath.Clean(s.Dir)))
}

// Profiles lists the configuration profiles (Spring profiles, Quarkus
// profiles, Micronaut environments) in sorted order.
func (s *SourceProject) Profiles() []string {
	if s.Project == nil {
		return nil
	}
	return s.Project.ProfileNames()
}

// FromSource describes the app built from a project directory with its
// default configuration. image is the app's image; empty derives
// "<name>:<version|latest>".
func FromSource(dir, image string, notes *Notes) (App, error) {
	s, err := ReadSource(dir, notes)
	if err != nil {
		return App{}, err
	}
	return s.App(image, "", notes), nil
}

// App describes the app with profile active ("" for the default
// configuration).
func (s *SourceProject) App(image, profile string, notes *Notes) App {
	app := App{Name: s.Name(), Image: image}
	if app.Image == "" {
		tag := "latest"
		if s.Project != nil && s.Project.Version != "" {
			tag = s.Project.Version
		}
		app.Image = app.Name + ":" + tag
		notes.Addf("%s: no --image given; image set to %q, set the pushed image in values", app.Name, app.Image)
	}

	switch {
	case s.ImageConfig != nil:
		f := imageFacts(s.ImageConfig)
		applyContainer(&app, f, notes)
		if s.Dockerfile != nil {
			d := dockerfileFacts(s.Dockerfile)
			if d.portSet() != f.portSet() || d.user != f.user {
				notes.Addf("%s: the image config (ports %q, user %q) differs from the Dockerfile (ports %q, user %q); the chart follows the image", app.Name, f.portSet(), f.user, d.portSet(), d.user)
			}
		}
	case s.Dockerfile != nil:
		applyContainer(&app, dockerfileFacts(s.Dockerfile), notes)
	default:
		notes.Addf("%s: no Dockerfile, so the container user is unknown; set runAsUser/runAsNonRoot in values", app.Name)
	}

	if p := s.Project; p != nil {
		c := &config{app: &app, props: p.properties(profile), notes: notes}
		switch p.Framework {
		case Spring:
			applySpring(c, p)
		case Quarkus:
			applyQuarkus(c, p)
		case Micronaut:
			applyMicronaut(c, p)
		}
		c.flush()

		if profile != "" {
			app.SetEnv(p.fw.profileEnv, profile)
		}

		kafkaFacts, kafkaNotes := ExtractKafkaFacts(c.props, "file ")
		for _, note := range kafkaNotes {
			notes.Addf("%s", note)
		}
		// In a full implementation we would store kafkaFacts somewhere,
		// but per requirements here we just call it and generate notes for now
		_ = kafkaFacts

		// EXPOSE only documents a port; the framework says where the app listens.
		var exposed []string
		for _, port := range app.Ports {
			if strings.HasPrefix(port.Name, "tcp-") {
				exposed = append(exposed, strconv.FormatInt(port.Port, 10))
			}
		}
		if len(exposed) > 0 {
			notes.Addf("%s: the container exposes %s, which the %s configuration does not use; the chart keeps the port, drop it if nothing listens there", app.Name, strings.Join(exposed, ", "), p.Framework)
		}
	}
	if app.Liveness == nil {
		notes.Addf("%s: no health endpoint found (no health dependency, no HEALTHCHECK); add liveness and readiness probes", app.Name)
	}
	if len(app.Ports) == 0 {
		notes.Addf("%s: no port found, so no Service was created", app.Name)
	}
	return app
}
