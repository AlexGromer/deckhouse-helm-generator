package synth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func noEnv(string) (string, bool) { return "", false }

func appNamed(apps []App, name string) *App {
	for i := range apps {
		if apps[i].Name == name {
			return &apps[i]
		}
	}
	return nil
}

func TestFromCompose(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"docker-compose.yml": `
services:
  api:
    image: registry.example.com/shop/api:${API_TAG:-1.0}
    ports:
      - "80:8080"
      - 127.0.0.1:9090:9090/tcp
      - target: 9443
        published: 443
    expose: ["8081"]
    environment:
      LOG_LEVEL: info
      DB_PASSWORD: devpass
      FROM_HOST:
    env_file: api.env
    command: ["--port", "8080"]
    entrypoint: /app/run --mode "prod server"
    user: "1000:1000"
    volumes:
      - data:/var/lib/api
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./static:/srv/static
      - /tmp/cache
    healthcheck:
      test: curl -f http://localhost:8080/health
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 1m
    deploy:
      replicas: 2
      resources:
        limits: {cpus: "0.5", memory: 512M}
        reservations: {memory: 128m}
    depends_on: [db]
    restart: always
  db:
    image: postgres:16
    environment:
      - POSTGRES_PASSWORD=${DB_PASS:?set it}
      - POSTGRES_DB=shop
    healthcheck:
      test: ["CMD", "pg_isready"]
      disable: false
  worker_queue:
    build: ./worker
volumes:
  data: {}
`,
		"api.env":    "# comment\nexport FEATURE_X=\"on\"\n",
		"nginx.conf": "events {}\n",
		".env":       "API_TAG=2.3\n",
	})
	if err := os.Mkdir(filepath.Join(dir, "static"), 0o755); err != nil {
		t.Fatal(err)
	}

	var notes Notes
	apps, err := FromCompose([]string{filepath.Join(dir, "docker-compose.yml")}, noEnv, &notes)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 3 {
		t.Fatalf("got %d apps", len(apps))
	}
	api := appNamed(apps, "api")
	if api == nil || api.Image != "registry.example.com/shop/api:2.3" {
		t.Fatalf("api = %+v", api)
	}
	var ports []int64
	for _, p := range api.Ports {
		ports = append(ports, p.Port)
	}
	if len(ports) != 4 || ports[0] != 8080 || ports[1] != 9090 || ports[2] != 9443 || ports[3] != 8081 {
		t.Errorf("ports = %v", ports)
	}
	env := map[string]string{}
	for _, e := range api.Env {
		env[e.Name] = e.Value
	}
	if env["LOG_LEVEL"] != "info" || env["FEATURE_X"] != "on" || len(api.SecretEnv) != 1 || api.SecretEnv[0].Value != "devpass" {
		t.Errorf("env = %v secret = %v", env, api.SecretEnv)
	}
	if strings.Join(api.Command, "|") != "/app/run|--mode|prod server" || strings.Join(api.Args, "|") != "--port|8080" {
		t.Errorf("command = %q args = %q", api.Command, api.Args)
	}
	if api.RunAsUser == nil || *api.RunAsUser != 1000 {
		t.Error("user not applied")
	}
	kinds := map[VolumeKind]int{}
	for _, v := range api.Volumes {
		kinds[v.Kind]++
	}
	if kinds[VolumePVC] != 1 || kinds[VolumeConfigFile] != 1 || kinds[VolumeEmptyDir] != 2 {
		t.Errorf("volumes = %+v", api.Volumes)
	}
	if p := api.Liveness; p == nil || strings.Join(p.Exec, " ") != "/bin/sh -c curl -f http://localhost:8080/health" ||
		p.PeriodSeconds != 30 || p.TimeoutSeconds != 5 || p.FailureThreshold != 3 || p.InitialDelaySeconds != 60 {
		t.Errorf("liveness = %+v", api.Liveness)
	}
	if *api.Replicas != 2 || api.Resources.Limits["memory"] != "512Mi" || api.Resources.Limits["cpu"] != "0.5" || api.Resources.Requests["memory"] != "128Mi" {
		t.Errorf("deploy = %v %+v", *api.Replicas, api.Resources)
	}

	db := appNamed(apps, "db")
	if db.Liveness == nil || strings.Join(db.Liveness.Exec, " ") != "pg_isready" {
		t.Errorf("db probe = %+v", db.Liveness)
	}
	worker := appNamed(apps, "worker-queue")
	if worker == nil || worker.Image != "worker-queue:latest" {
		t.Errorf("worker = %+v", worker)
	}

	all := strings.Join(notes.Items(), "\n")
	for _, want := range []string{
		"FROM_HOST takes its value from the host", "DB_PASSWORD went to Secret", "host port 80 maps to 8080",
		"named volume \"data\" is PVC api-data", "bind mount ./static", "depends_on is not enforced",
		"not carried over: restart", "DB_PASS is required", "renamed to \"worker-queue\"", "built from source",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}
}

func TestFromComposeOverrideAndErrors(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"a.yml": "services:\n  web:\n    image: web:1\n    ports: [\"8080\"]\n",
		"b.yml": "services:\n  web:\n    image: web:2\n",
		"c.yml": "services:\n  bad:\n    ports: [\"x\"]\n",
		"d.yml": "services:\n  web:\n    image: web:1\n    ports: [\"not-a-port\"]\n",
		"e.yml": "version: '3'\n",
	})
	var notes Notes
	apps, err := FromCompose([]string{filepath.Join(dir, "a.yml"), filepath.Join(dir, "b.yml")}, noEnv, &notes)
	if err != nil || apps[0].Image != "web:2" || len(apps[0].Ports) != 1 {
		t.Errorf("override: %+v %v", apps, err)
	}
	for _, f := range []string{"c.yml", "d.yml", "e.yml"} {
		if _, err := FromCompose([]string{filepath.Join(dir, f)}, noEnv, &notes); err == nil {
			t.Errorf("%s: expected an error", f)
		}
	}
}

func TestInterpolation(t *testing.T) {
	env := map[string]string{"SET": "x", "EMPTY": ""}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	var notes Notes
	for in, want := range map[string]string{
		"$SET-${SET}":  "x-x",
		"${UNSET:-d}":  "d",
		"${EMPTY:-d}":  "d",
		"${EMPTY-d}":   "",
		"${UNSET-d}":   "d",
		"cost $$5":     "cost $5",
		"${UNSET}":     "",
		"${SET:?must}": "x",
	} {
		if got := interpolate(in, lookup, &notes); got != want {
			t.Errorf("interpolate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestComposeHelpers(t *testing.T) {
	if got := strings.Join(words(`a "b c" 'd e' f`), "|"); got != "a|b c|d e|f" {
		t.Errorf("words = %q", got)
	}
	if duration("1m30s") != int64(90e9) || duration(10.0) != int64(10e9) || duration("bad") != 0 {
		t.Error("duration")
	}
	res := composeResources(map[string]interface{}{"cpus": 1.5, "memory": "2g"})
	if res["cpu"] != "1.5" || res["memory"] != "2Gi" {
		t.Errorf("resources = %v", res)
	}
}

func TestFromComposeLongSyntax(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"compose.yaml": `
services:
  app:
    image: app:1
    ports:
      - "8000-8002:9000-9002/udp"
    expose: ["7000/tcp"]
    env_file:
      - path: missing.env
        required: false
    environment:
      - FROM_SHELL
    volumes:
      - type: bind
        source: ./conf.ini
        target: /etc/app/conf.ini
      - type: volume
        source: state
        target: /state
        read_only: true
      - type: tmpfs
        target: /run
      - type: npipe
        target: /pipe
      - /anonymous
    healthcheck:
      test: ["NONE"]
    depends_on:
      db:
        condition: service_healthy
    user: "0"
  off:
    image: off:1
    healthcheck:
      disable: true
      test: ["CMD", "true"]
`,
		"conf.ini": "[a]\nb=1\n",
	})
	var notes Notes
	apps, err := FromCompose([]string{filepath.Join(dir, "compose.yaml")}, noEnv, &notes)
	if err != nil {
		t.Fatal(err)
	}
	app := appNamed(apps, "app")
	if len(app.Ports) != 4 || app.Ports[0].Port != 9000 || app.Ports[0].Protocol != "UDP" || app.Ports[3].Port != 7000 {
		t.Errorf("ports = %+v", app.Ports)
	}
	byPath := map[string]Volume{}
	for _, v := range app.Volumes {
		byPath[v.MountPath] = v
	}
	if byPath["/etc/app/conf.ini"].Kind != VolumeConfigFile || byPath["/state"].Kind != VolumePVC || !byPath["/state"].ReadOnly ||
		byPath["/run"].Kind != VolumeEmptyDir || byPath["/pipe"].Kind != VolumeEmptyDir || byPath["/anonymous"].Kind != VolumeEmptyDir {
		t.Errorf("volumes = %+v", app.Volumes)
	}
	if app.Liveness != nil || appNamed(apps, "off").Liveness != nil {
		t.Error("NONE and disable must give no probe")
	}
	if app.RunAsUser == nil || *app.RunAsUser != 0 {
		t.Error("uid 0 is still set explicitly")
	}
	all := strings.Join(notes.Items(), "\n")
	for _, want := range []string{"FROM_SHELL takes its value", "npipe volume /pipe", "anonymous volume /anonymous", "depends_on is not enforced", "runs as root"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes lack %q:\n%s", want, all)
		}
	}
}

func TestComposePortErrors(t *testing.T) {
	for _, spec := range []string{"9-1", "1-500", "70000", "x:y"} {
		var a App
		var notes Notes
		if err := composePort(&a, spec, true, &notes); err == nil {
			t.Errorf("port %q: expected an error", spec)
		}
	}
	var a App
	var notes Notes
	if err := composeVolume(&a, 0, map[string]interface{}{"type": "volume"}, ".", &notes); err == nil {
		t.Error("a volume without target must fail")
	}
	if words(42) != nil || list("x")[0] != "x" || scalar(true) != "true" || scalar(3) != "3" {
		t.Error("helpers")
	}
	if res := composeResources("x"); res != nil {
		t.Error("non-map resources")
	}
	if res := composeResources(map[string]interface{}{"memory": "lots"}); res["memory"] != "lots" {
		t.Errorf("unparsable memory is kept: %v", res)
	}
}
