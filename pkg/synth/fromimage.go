package synth

import (
	"sort"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry"
)

// FromImage describes the app an image runs, from its configuration.
// Entrypoint, command, working directory and environment are not copied:
// the image already carries them. The variable names are reported as
// candidates for values.
func FromImage(ref registry.Reference, cfg *registry.ImageConfig, notes *Notes) App {
	c := cfg.Config
	name := c.Labels["org.opencontainers.image.title"]
	if name == "" {
		name = ref.Repository[strings.LastIndex(ref.Repository, "/")+1:]
	}
	app := App{Name: DNSName(name), Image: ref.Name()}

	ports := make([]string, 0, len(c.ExposedPorts))
	for p := range c.ExposedPorts {
		ports = append(ports, p)
	}
	sort.Strings(ports)
	for _, p := range ports {
		number, proto, _ := strings.Cut(p, "/")
		n, err := strconv.ParseInt(number, 10, 64)
		if err != nil || n < 1 || n > 65535 {
			notes.Addf("%s: ignored exposed port %q", app.Name, p)
			continue
		}
		app.AddPort(n, proto)
	}
	if len(app.Ports) == 0 {
		notes.Addf("%s: the image exposes no port, so no Service was created; add ports in values if the app listens on one", app.Name)
	}

	applyUser(&app, c.User, "image", notes)

	vols := make([]string, 0, len(c.Volumes))
	for v := range c.Volumes {
		vols = append(vols, v)
	}
	sort.Strings(vols)
	for i, v := range vols {
		app.Volumes = append(app.Volumes, Volume{Name: "volume-" + strconv.Itoa(i+1), MountPath: v, Kind: VolumeEmptyDir})
		notes.Addf("%s: image volume %s is an emptyDir (lost on restart); make it persistent if it holds data", app.Name, v)
	}

	if c.Healthcheck != nil {
		if p := healthcheckProbe(c.Healthcheck.Test, c.Healthcheck.Interval, c.Healthcheck.Timeout, c.Healthcheck.StartPeriod, c.Healthcheck.Retries); p != nil {
			app.Liveness, app.Readiness = p, copyProbe(p)
		}
	} else {
		notes.Addf("%s: the image defines no HEALTHCHECK, so the chart has no probes; add liveness and readiness probes", app.Name)
	}

	var env []string
	for _, e := range c.Env {
		k, _, _ := strings.Cut(e, "=")
		if k != "PATH" && k != "HOME" && k != "HOSTNAME" {
			env = append(env, k)
		}
	}
	if len(env) > 0 {
		notes.Addf("%s: the image sets %s; override them through env in values if needed", app.Name, strings.Join(env, ", "))
	}
	return app
}

// applyUser sets the security context from a "uid[:gid]" user and reports
// users it cannot use.
func applyUser(app *App, user, from string, notes *Notes) {
	switch {
	case user == "" || user == "0" || user == "root" || strings.HasPrefix(user, "0:"):
		notes.Addf("%s: the %s runs as root; set runAsNonRoot and a non-root user if the app allows it", app.Name, from)
		if user != "" && user != "root" {
			app.SetUser(user)
		}
	case !app.SetUser(user):
		notes.Addf("%s: the %s runs as named user %q; kubelet cannot check runAsNonRoot for names, set its numeric uid in values", app.Name, from, user)
	}
}

// healthcheckProbe turns a Docker/compose health check into an exec probe.
// Durations are in nanoseconds.
func healthcheckProbe(test []string, interval, timeout, startPeriod, retries int64) *Probe {
	if len(test) == 0 {
		return nil
	}
	var cmd []string
	switch test[0] {
	case "NONE":
		return nil
	case "CMD":
		cmd = test[1:]
	case "CMD-SHELL":
		cmd = []string{"/bin/sh", "-c", strings.Join(test[1:], " ")}
	default:
		cmd = []string{"/bin/sh", "-c", strings.Join(test, " ")}
	}
	if len(cmd) == 0 {
		return nil
	}
	return &Probe{
		Exec:                cmd,
		PeriodSeconds:       seconds(interval),
		TimeoutSeconds:      seconds(timeout),
		InitialDelaySeconds: seconds(startPeriod),
		FailureThreshold:    retries,
	}
}

func seconds(ns int64) int64 {
	if ns <= 0 {
		return 0
	}
	s := (ns + 999_999_999) / 1_000_000_000
	return s
}

func copyProbe(p *Probe) *Probe {
	c := *p
	c.Exec = append([]string(nil), p.Exec...)
	return &c
}
