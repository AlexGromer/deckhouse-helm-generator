package synth

import (
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
	if s.Dockerfile == nil && p == nil {
		return nil, fmt.Errorf("%s: neither a Dockerfile nor a Spring Boot, Quarkus or Micronaut project (pom.xml or build.gradle)", dir)
	}
	return s, nil
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
		if profile != "" {
			app.SetEnv(p.fw.profileEnv, profile)
		}
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

var placeholder = regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)

// resolvePlaceholders replaces ${NAME:default} by its default; it returns
// the names of placeholders without a default. Spring, Quarkus (SmallRye
// Config) and Micronaut share this syntax.
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
