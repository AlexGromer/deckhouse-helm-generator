package synth

import (
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

	f := imageFacts(cfg)
	applyContainer(&app, f, notes)
	if len(app.Ports) == 0 {
		notes.Addf("%s: the image exposes no port, so no Service was created; add ports in values if the app listens on one", app.Name)
	}
	if c.Healthcheck == nil {
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
