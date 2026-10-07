package golden

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry/registrytest"
)

var synthModes = []string{"universal", "separate", "library", "umbrella"}

// renderAll renders every non-library chart under out.
func renderAll(t *testing.T, helm, out string) []object {
	t.Helper()
	var objs []object
	for _, chart := range findCharts(t, out) {
		data, err := os.ReadFile(filepath.Join(chart, "Chart.yaml"))
		if err != nil || bytes.Contains(data, []byte("type: library")) {
			continue
		}
		stream, err := run(helm, "template", "golden", chart)
		if err != nil {
			t.Fatalf("helm template %s: %v\n%s", chart, err, stream)
		}
		objs = append(objs, parseStream(stream)...)
	}
	return objs
}

func findObject(objs []object, kind, name string) object {
	for _, o := range objs {
		if o.kind() == kind && o.name() == name {
			return o
		}
	}
	return nil
}

func firstContainer(o object) map[string]interface{} {
	c, _ := list(get(map[string]interface{}(o), "spec", "template", "spec", "containers"))[0].(map[string]interface{})
	return c
}

// synthesize generates charts from a synthesizing source in every mode,
// checks them with Helm and hands the rendered objects to check.
func synthesize(t *testing.T, args []string, check func(t *testing.T, objs []object)) {
	helm := requireHelm(t)
	for _, mode := range synthModes {
		t.Run(mode, func(t *testing.T) {
			out := t.TempDir()
			runOK(t, dhgBin, append([]string{"generate", "-o", out, "--chart-name", "app", "--mode", mode}, args...)...)
			if !fileExists(filepath.Join(out, "SYNTHESIS.md")) {
				t.Error("SYNTHESIS.md not written")
			}
			if _, complete := checkCharts(t, helm, out); !complete {
				return
			}
			check(t, renderAll(t, helm, out))
		})
	}
}

func TestSynthesizeCompose(t *testing.T) {
	compose := filepath.Join(repoRoot(), "tests", "golden", "testdata", "synth", "compose", "docker-compose.yml")
	synthesize(t, []string{"-s", "compose", "-f", compose}, func(t *testing.T, objs []object) {
		for _, want := range [][2]string{
			{"Deployment", "api"}, {"Deployment", "db"}, {"Deployment", "worker"}, {"Service", "api"}, {"Service", "db"},
			{"PersistentVolumeClaim", "api-uploads"}, {"PersistentVolumeClaim", "db-pgdata"}, {"ConfigMap", "api-files"},
		} {
			if findObject(objs, want[0], want[1]) == nil {
				t.Errorf("%s/%s not rendered", want[0], want[1])
			}
		}
		api := findObject(objs, "Deployment", "api")
		if api == nil {
			return
		}
		c := firstContainer(api)
		if c["image"] != "registry.example.com/shop/api:1.4.2" {
			t.Errorf("api image = %v (.env interpolation)", c["image"])
		}
		if scalar(get(map[string]interface{}(api), "spec", "replicas")) != "2" {
			t.Errorf("api replicas = %v", get(map[string]interface{}(api), "spec", "replicas"))
		}
		if scalar(get(c, "resources", "limits", "memory")) != "512Mi" {
			t.Errorf("api resources = %v", c["resources"])
		}
		if !strings.Contains(scalar(get(c, "livenessProbe", "exec", "command")), "healthz") {
			t.Errorf("api liveness = %v", c["livenessProbe"])
		}
		if svc := findObject(objs, "Service", "db"); svc != nil {
			port, _ := list(get(map[string]interface{}(svc), "spec", "ports"))[0].(map[string]interface{})
			if scalar(port["port"]) != "5432" || port["name"] != "tcp-5432" {
				t.Errorf("db service port = %v", port)
			}
		}
		if w := findObject(objs, "Deployment", "worker"); w != nil {
			if scalar(firstContainer(w)["args"]) != scalar([]interface{}{"--queue", "orders"}) {
				t.Errorf("worker args = %v", firstContainer(w)["args"])
			}
		}
	})
}

func TestSynthesizeSpringSource(t *testing.T) {
	project := filepath.Join(repoRoot(), "tests", "golden", "testdata", "synth", "orders-service")
	synthesize(t, []string{"-s", "source", "-f", project, "--image", "registry.example.com/orders:2.1.0"}, func(t *testing.T, objs []object) {
		d := findObject(objs, "Deployment", "orders")
		if d == nil {
			t.Fatal("Deployment/orders not rendered")
		}
		c := firstContainer(d)
		if get(c, "readinessProbe", "httpGet", "path") != "/actuator/health/readiness" || scalar(get(c, "livenessProbe", "httpGet", "port")) != "8080" {
			t.Errorf("probes = %v / %v", c["livenessProbe"], c["readinessProbe"])
		}
		if scalar(get(map[string]interface{}(d), "spec", "template", "spec", "securityContext", "runAsUser")) != "10001" {
			t.Error("runAsUser from the Dockerfile not rendered")
		}
		cm := findObject(objs, "ConfigMap", "orders-env")
		if cm == nil || get(map[string]interface{}(cm), "data", "SPRING_KAFKA_BOOTSTRAP_SERVERS") != "kafka:9092" {
			t.Errorf("ConfigMap orders-env = %v", cm)
		}
		if s := findObject(objs, "Secret", "orders-env"); s == nil {
			t.Error("Secret orders-env not rendered")
		}
	})
}

func TestSynthesizeImage(t *testing.T) {
	fake := registrytest.New()
	defer fake.Close()
	fake.Token = "t0k"
	fake.Push("team/orders", "2.1.0", map[string]interface{}{
		"config": map[string]interface{}{
			"User":         "10001",
			"ExposedPorts": map[string]interface{}{"8080/tcp": map[string]interface{}{}},
			"Labels":       map[string]interface{}{"org.opencontainers.image.title": "orders-api"},
			"Volumes":      map[string]interface{}{"/tmp": map[string]interface{}{}},
			"Healthcheck":  map[string]interface{}{"Test": []interface{}{"CMD", "/app/healthcheck"}, "Interval": 10e9},
		},
	}, "linux/amd64", "linux/arm64")
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	image := fake.Host() + "/team/orders:2.1.0"
	synthesize(t, []string{"-s", "image", "--image", image, "--insecure-registry"}, func(t *testing.T, objs []object) {
		d := findObject(objs, "Deployment", "orders-api")
		if d == nil {
			t.Fatal("Deployment/orders-api not rendered")
		}
		c := firstContainer(d)
		if c["image"] != image {
			t.Errorf("image = %v", c["image"])
		}
		if scalar(get(c, "livenessProbe", "exec", "command")) != scalar([]interface{}{"/app/healthcheck"}) ||
			scalar(get(c, "livenessProbe", "periodSeconds")) != "10" {
			t.Errorf("liveness = %v", c["livenessProbe"])
		}
		if get(map[string]interface{}(d), "spec", "template", "spec", "securityContext", "runAsNonRoot") != true {
			t.Error("runAsNonRoot not rendered")
		}
		if findObject(objs, "Service", "orders-api") == nil {
			t.Error("Service/orders-api not rendered")
		}
	})
}
