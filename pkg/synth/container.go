package synth

import (
	"sort"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry"
)

// containerFacts is what a Dockerfile or an image configuration tells about
// the container: the same facts, from the build recipe or from its result.
type containerFacts struct {
	from        string   // "Dockerfile" or "image", for notes
	ports       []string // "8080", "8080/tcp", "53/udp"
	unresolved  []string // EXPOSE arguments with variables without a default
	user        string
	volumes     []string
	healthcheck []string // Docker form: CMD …, CMD-SHELL …, NONE
	interval    int64    // nanoseconds
	timeout     int64
	startPeriod int64
	retries     int64
}

func dockerfileFacts(d *Dockerfile) containerFacts {
	return containerFacts{
		from: "Dockerfile", ports: d.Expose, unresolved: d.Unresolved, user: d.User, volumes: d.Volumes,
		healthcheck: d.Healthcheck, interval: d.Interval, timeout: d.Timeout, startPeriod: d.StartPeriod, retries: d.Retries,
	}
}

func imageFacts(cfg *registry.ImageConfig) containerFacts {
	c := cfg.Config
	f := containerFacts{from: "image", user: c.User}
	for p := range c.ExposedPorts {
		f.ports = append(f.ports, p)
	}
	sort.Strings(f.ports)
	for v := range c.Volumes {
		f.volumes = append(f.volumes, v)
	}
	sort.Strings(f.volumes)
	if h := c.Healthcheck; h != nil {
		f.healthcheck, f.interval, f.timeout, f.startPeriod, f.retries = h.Test, h.Interval, h.Timeout, h.StartPeriod, h.Retries
	}
	return f
}

// applyContainer adds the ports, user, volumes and health check probes.
func applyContainer(app *App, f containerFacts, notes *Notes) {
	for _, p := range f.ports {
		number, proto, _ := strings.Cut(p, "/")
		n, err := strconv.ParseInt(number, 10, 64)
		if err != nil || n < 1 || n > 65535 {
			notes.Addf("%s: ignored exposed port %q", app.Name, p)
			continue
		}
		app.AddPort(n, proto)
	}
	for _, p := range f.unresolved {
		notes.Addf("%s: EXPOSE %s uses a variable without a default; add the port in values", app.Name, p)
	}
	applyUser(app, f.user, f.from, notes)
	for i, v := range f.volumes {
		app.Volumes = append(app.Volumes, Volume{Name: "volume-" + strconv.Itoa(i+1), MountPath: v, Kind: VolumeEmptyDir})
		notes.Addf("%s: %s volume %s is an emptyDir (lost on restart); make it persistent if it holds data", app.Name, f.from, v)
	}
	if p := healthcheckProbe(f.healthcheck, f.interval, f.timeout, f.startPeriod, f.retries); p != nil {
		app.Liveness, app.Readiness = p, copyProbe(p)
	}
}

// portSet normalises exposed ports for comparison: "8080" = "8080/tcp".
func (f containerFacts) portSet() string {
	set := make([]string, 0, len(f.ports))
	for _, p := range f.ports {
		if !strings.Contains(p, "/") {
			p += "/tcp"
		}
		set = append(set, strings.ToLower(p))
	}
	sort.Strings(set)
	return strings.Join(set, " ")
}
