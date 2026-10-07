package synth

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry"
)

func TestFromImage(t *testing.T) {
	ref, _ := registry.ParseReference("registry.example.com/team/orders:2.1.0")
	cfg := &registry.ImageConfig{Config: registry.ContainerConfig{
		User:         "10001:10001",
		ExposedPorts: map[string]struct{}{"8080/tcp": {}, "9090/udp": {}, "bad/tcp": {}},
		Volumes:      map[string]struct{}{"/data": {}},
		Env:          []string{"PATH=/bin", "JAVA_OPTS=-Xmx512m", "APP_MODE=prod"},
		Labels:       map[string]string{"org.opencontainers.image.title": "Orders API"},
		Healthcheck:  &registry.Healthcheck{Test: []string{"CMD-SHELL", "curl -f localhost:8080"}, Interval: 5e9, Retries: 2},
	}}
	var notes Notes
	app := FromImage(ref, cfg, &notes)
	if app.Name != "orders-api" || app.Image != "registry.example.com/team/orders:2.1.0" {
		t.Errorf("name = %s image = %s", app.Name, app.Image)
	}
	if len(app.Ports) != 2 || app.Ports[0].Port != 8080 || app.Ports[1].Protocol != "UDP" {
		t.Errorf("ports = %+v", app.Ports)
	}
	if *app.RunAsUser != 10001 || *app.RunAsGroup != 10001 || len(app.Volumes) != 1 || app.Volumes[0].Kind != VolumeEmptyDir {
		t.Errorf("user/volumes = %v %v %+v", *app.RunAsUser, *app.RunAsGroup, app.Volumes)
	}
	if app.Liveness == nil || strings.Join(app.Liveness.Exec, " ") != "/bin/sh -c curl -f localhost:8080" ||
		app.Liveness.PeriodSeconds != 5 || app.Liveness.FailureThreshold != 2 || app.Readiness == app.Liveness {
		t.Errorf("probes = %+v", app.Liveness)
	}
	if app.Command != nil || app.Env != nil {
		t.Error("the image's entrypoint and env must not be copied")
	}
	all := strings.Join(notes.Items(), "\n")
	for _, want := range []string{`ignored exposed port "bad/tcp"`, "volume /data is an emptyDir", "JAVA_OPTS, APP_MODE"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "PATH") {
		t.Error("PATH is not a configuration candidate")
	}
}

func TestFromImageBareAndUsers(t *testing.T) {
	ref, _ := registry.ParseReference("nginx:1.27")
	for _, tt := range []struct {
		user, note string
		uid        *int64
	}{
		{"", "runs as root", nil},
		{"root", "runs as root", nil},
		{"0:0", "runs as root", ptr(0)},
		{"www-data", `named user "www-data"`, nil},
	} {
		var notes Notes
		app := FromImage(ref, &registry.ImageConfig{Config: registry.ContainerConfig{User: tt.user}}, &notes)
		all := strings.Join(notes.Items(), "\n")
		if app.Name != "nginx" || !strings.Contains(all, tt.note) || !strings.Contains(all, "exposes no port") || !strings.Contains(all, "no HEALTHCHECK") {
			t.Errorf("user %q: name %s notes:\n%s", tt.user, app.Name, all)
		}
		if (tt.uid == nil) != (app.RunAsUser == nil) || (tt.uid != nil && *app.RunAsUser != *tt.uid) {
			t.Errorf("user %q: runAsUser = %v", tt.user, app.RunAsUser)
		}
	}
}

func TestHealthcheckProbe(t *testing.T) {
	if healthcheckProbe(nil, 0, 0, 0, 0) != nil || healthcheckProbe([]string{"NONE"}, 0, 0, 0, 0) != nil || healthcheckProbe([]string{"CMD"}, 0, 0, 0, 0) != nil {
		t.Error("empty, NONE and argument-less checks give no probe")
	}
	p := healthcheckProbe([]string{"pg_isready", "-q"}, 1, 1_500_000_000, 0, 0)
	if strings.Join(p.Exec, " ") != "/bin/sh -c pg_isready -q" || p.PeriodSeconds != 1 || p.TimeoutSeconds != 2 {
		t.Errorf("probe = %+v (durations round up to whole seconds)", p)
	}
}

func ptr(v int64) *int64 { return &v }
