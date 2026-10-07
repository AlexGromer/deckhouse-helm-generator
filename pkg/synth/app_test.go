package synth

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func find(objs []*unstructured.Unstructured, kind, name string) *unstructured.Unstructured {
	for _, o := range objs {
		if o.GetKind() == kind && o.GetName() == name {
			return o
		}
	}
	return nil
}

func TestDNSName(t *testing.T) {
	for in, want := range map[string]string{
		"Orders_Service":        "orders-service",
		"--a..b--":              "a-b",
		"":                      "app",
		strings.Repeat("x", 70): strings.Repeat("x", 63),
	} {
		if got := DNSName(in); got != want {
			t.Errorf("DNSName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAppHelpers(t *testing.T) {
	var a App
	a.AddPort(8080, "tcp")
	a.AddPort(8080, "TCP")
	a.AddPort(9090, "")
	a.AddPort(53, "udp")
	if len(a.Ports) != 3 || a.Ports[0].Name != "tcp-8080" || a.Ports[1].Name != "tcp-9090" || a.Ports[2].Name != "udp-53" {
		t.Errorf("ports = %+v", a.Ports)
	}
	a.AddNamedPort("http", 8080, "TCP")
	a.AddNamedPort("http", 7070, "TCP")
	if a.Ports[0].Name != "http" || a.Ports[3].Name != "tcp-7070" {
		t.Errorf("named ports = %+v (a known name renames the port; a taken name falls back)", a.Ports)
	}

	a.SetEnv("LOG_LEVEL", "info")
	a.SetEnv("DB_PASSWORD", "x")
	a.SetEnv("LOG_LEVEL", "debug")
	if len(a.Env) != 1 || a.Env[0].Value != "debug" || len(a.SecretEnv) != 1 || a.SecretEnv[0].Name != "DB_PASSWORD" {
		t.Errorf("env = %+v secret = %+v", a.Env, a.SecretEnv)
	}

	if !a.SetUser("1000:2000") || *a.RunAsUser != 1000 || *a.RunAsGroup != 2000 {
		t.Errorf("SetUser numeric failed")
	}
	var b App
	if b.SetUser("app") || b.RunAsUser != nil {
		t.Error("a named user must not set runAsUser")
	}
}

func TestManifests(t *testing.T) {
	replicas := int64(2)
	app := App{
		Name:      "Orders",
		Image:     "registry.example.com/orders:1.0",
		Args:      []string{"--verbose"},
		Env:       []EnvVar{{Name: "LOG_LEVEL", Value: "info"}},
		SecretEnv: []EnvVar{{Name: "DB_PASSWORD", Value: ""}},
		Volumes: []Volume{
			{Name: "data", MountPath: "/var/lib/orders", Kind: VolumePVC},
			{Name: "tmp", MountPath: "/tmp", Kind: VolumeEmptyDir},
			{Name: "conf", MountPath: "/etc/orders/app.yml", Kind: VolumeConfigFile, FileName: "app.yml", Content: "a: 1\n"},
		},
		Liveness:  &Probe{HTTPPath: "/actuator/health/liveness", Port: 8080, PeriodSeconds: 10},
		Readiness: &Probe{Exec: []string{"sh", "-c", "true"}},
		Replicas:  &replicas,
		Resources: &Resources{Limits: map[string]string{"memory": "512Mi"}},
	}
	app.AddPort(8080, "TCP")
	app.SetUser("1001")

	objs := Manifests(app)
	for _, want := range [][2]string{
		{"Deployment", "orders"}, {"Service", "orders"}, {"ConfigMap", "orders-env"},
		{"Secret", "orders-env"}, {"ConfigMap", "orders-files"}, {"PersistentVolumeClaim", "orders-data"},
	} {
		if find(objs, want[0], want[1]) == nil {
			t.Errorf("missing %s/%s", want[0], want[1])
		}
	}
	for _, o := range objs {
		// Objects must survive deep copies (int64 numbers only) and YAML.
		if _, err := yaml.Marshal(o.DeepCopy().Object); err != nil {
			t.Errorf("%s/%s: %v", o.GetKind(), o.GetName(), err)
		}
	}

	d := find(objs, "Deployment", "orders")
	if got, _, _ := unstructured.NestedInt64(d.Object, "spec", "replicas"); got != 2 {
		t.Errorf("replicas = %d", got)
	}
	sc, _, _ := unstructured.NestedMap(d.Object, "spec", "template", "spec", "securityContext")
	if sc["runAsUser"] != int64(1001) || sc["runAsNonRoot"] != true {
		t.Errorf("securityContext = %v", sc)
	}
	containers, _, _ := unstructured.NestedSlice(d.Object, "spec", "template", "spec", "containers")
	c := containers[0].(map[string]interface{})
	if len(c["envFrom"].([]interface{})) != 2 || len(c["volumeMounts"].([]interface{})) != 3 {
		t.Errorf("container = %v", c)
	}
	if p, _, _ := unstructured.NestedString(c, "livenessProbe", "httpGet", "path"); p != "/actuator/health/liveness" {
		t.Errorf("liveness path = %q", p)
	}
	sel, _, _ := unstructured.NestedStringMap(find(objs, "Service", "orders").Object, "spec", "selector")
	if sel[nameLabel] != "orders" {
		t.Errorf("service selector = %v", sel)
	}
}

func TestManifestsMinimal(t *testing.T) {
	objs := Manifests(App{Name: "worker", Image: "worker:1"})
	if len(objs) != 1 || objs[0].GetKind() != "Deployment" {
		t.Fatalf("a portless app without env is a lone Deployment, got %d objects", len(objs))
	}
	if _, found, _ := unstructured.NestedMap(objs[0].Object, "spec", "template", "spec", "securityContext"); found {
		t.Error("no securityContext without a user")
	}
}

func TestReport(t *testing.T) {
	objs := Manifests(App{Name: "api", Image: "api:1", Ports: []Port{{Name: "http", Port: 80, Protocol: "TCP"}}})
	r := Report([]string{"image api:1"}, objs, []string{"set DB_PASSWORD"})
	for _, want := range []string{"image api:1", "| Deployment | api |", "| Service | api |", "Resource requests and limits", "- set DB_PASSWORD"} {
		if !strings.Contains(r, want) {
			t.Errorf("report lacks %q:\n%s", want, r)
		}
	}
}

func TestManifestsContainerDetails(t *testing.T) {
	uid, gid := int64(1000), int64(2000)
	app := App{
		Name: "tool", Image: "tool:1", Command: []string{"/bin/tool"}, Args: []string{"run"}, WorkingDir: "/work",
		RunAsUser: &uid, RunAsGroup: &gid, Labels: map[string]string{"tier": "batch"},
		Resources: &Resources{Requests: map[string]string{"cpu": "100m"}},
		Volumes: []Volume{
			{Name: "a", MountPath: "/etc/a.conf", Kind: VolumeConfigFile, FileName: "app.conf", Content: "x", ReadOnly: true},
			{Name: "b", MountPath: "/etc/b.conf", Kind: VolumeConfigFile, FileName: "app.conf", Content: "y"},
			{Name: "c", MountPath: "/etc/c", Kind: VolumeConfigFile, FileName: "...", Content: "z"},
		},
	}
	dep := find(Manifests(app), "Deployment", "tool")
	c, _, _ := unstructured.NestedSlice(dep.Object, "spec", "template", "spec", "containers")
	container := c[0].(map[string]interface{})
	if container["workingDir"] != "/work" || container["command"].([]interface{})[0] != "/bin/tool" {
		t.Errorf("container = %v", container)
	}
	if v, _, _ := unstructured.NestedString(container, "resources", "requests", "cpu"); v != "100m" {
		t.Errorf("requests = %v", container["resources"])
	}
	mounts := container["volumeMounts"].([]interface{})
	var keys []string
	for _, m := range mounts {
		keys = append(keys, m.(map[string]interface{})["subPath"].(string))
	}
	if strings.Join(keys, " ") != "app.conf app.conf-2 file" || mounts[0].(map[string]interface{})["readOnly"] != true {
		t.Errorf("mounts = %v", mounts)
	}
	if g, _, _ := unstructured.NestedInt64(dep.Object, "spec", "template", "spec", "securityContext", "runAsGroup"); g != 2000 {
		t.Error("runAsGroup not set")
	}
	if l, _, _ := unstructured.NestedString(dep.Object, "spec", "template", "metadata", "labels", "tier"); l != "batch" {
		t.Error("pod labels not set")
	}
	if sel, _, _ := unstructured.NestedStringMap(dep.Object, "spec", "selector", "matchLabels"); sel["tier"] != "" {
		t.Error("extra labels must not enter the selector")
	}
}
