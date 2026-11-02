package k8s

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// podSpecObj returns a Deployment whose pod spec is the containers plus extra.
func podSpecObj(extra map[string]interface{}) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"containers": []interface{}{
			map[string]interface{}{"name": "app", "image": "app:v1"},
		},
	}
	for k, v := range extra {
		spec[k] = v
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "web", "namespace": "ns"},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{"spec": spec},
		},
	}}
}

func TestExtractPodTemplateValues_PodSpecScalarsAndObjects(t *testing.T) {
	hostAliases := []interface{}{map[string]interface{}{
		"ip": "10.0.0.1", "hostnames": []interface{}{"db.local"},
	}}
	dnsConfig := map[string]interface{}{
		"nameservers": []interface{}{"1.1.1.1"},
	}
	want := map[string]interface{}{
		"hostname":                      "web-host",
		"subdomain":                     "web-sub",
		"terminationGracePeriodSeconds": int64(90),
		"priorityClassName":             "high",
		"hostAliases":                   hostAliases,
		"dnsPolicy":                     "None",
		"dnsConfig":                     dnsConfig,
		"runtimeClassName":              "gvisor",
		"shareProcessNamespace":         true,
		"enableServiceLinks":            false,
	}
	obj := podSpecObj(want)
	values := map[string]interface{}{}
	extractPodTemplateValues(obj, values, "spec", "template")

	for k, w := range want {
		got, ok := values[k]
		if !ok {
			t.Errorf("values lack %q", k)
			continue
		}
		if !reflect.DeepEqual(got, w) {
			t.Errorf("values[%q] = %#v, want %#v", k, got, w)
		}
	}
}

func TestExtractPodTemplateValues_PodSpecDefaultsAreNotCopied(t *testing.T) {
	obj := podSpecObj(map[string]interface{}{
		"dnsPolicy":             "ClusterFirst",
		"enableServiceLinks":    true,
		"shareProcessNamespace": false,
		"hostname":              "",
		"priorityClassName":     "",
		// The API server sets 30 on every pod template.
		"terminationGracePeriodSeconds": int64(30),
	})
	values := map[string]interface{}{}
	extractPodTemplateValues(obj, values, "spec", "template")

	for _, k := range []string{
		"dnsPolicy", "enableServiceLinks", "shareProcessNamespace", "hostname",
		"priorityClassName", "subdomain", "terminationGracePeriodSeconds",
		"hostAliases", "dnsConfig", "runtimeClassName",
	} {
		if v, ok := values[k]; ok {
			t.Errorf("values[%q] = %#v, want it absent", k, v)
		}
	}
}

func TestExtractPodTemplateValues_TerminationGracePeriodZero(t *testing.T) {
	// 0 is a real setting (kill at once), not the unset default.
	obj := podSpecObj(map[string]interface{}{"terminationGracePeriodSeconds": int64(0)})
	values := map[string]interface{}{}
	extractPodTemplateValues(obj, values, "spec", "template")
	if v, ok := values["terminationGracePeriodSeconds"]; !ok || v != int64(0) {
		t.Errorf("terminationGracePeriodSeconds = %#v (set %v), want 0", v, ok)
	}
}

func TestExtractPodTemplateValues_TerminationGracePeriodNonDefault(t *testing.T) {
	for _, in := range []interface{}{int64(60), float64(60)} {
		obj := podSpecObj(map[string]interface{}{"terminationGracePeriodSeconds": in})
		values := map[string]interface{}{}
		extractPodTemplateValues(obj, values, "spec", "template")
		if v, ok := values["terminationGracePeriodSeconds"]; !ok || v != in {
			t.Errorf("terminationGracePeriodSeconds = %#v (set %v), want %#v", v, ok, in)
		}
	}
	// 30 as float64 (JSON) is the default as well.
	obj := podSpecObj(map[string]interface{}{"terminationGracePeriodSeconds": float64(30)})
	values := map[string]interface{}{}
	extractPodTemplateValues(obj, values, "spec", "template")
	if v, ok := values["terminationGracePeriodSeconds"]; ok {
		t.Errorf("terminationGracePeriodSeconds = %#v, want it absent", v)
	}
}

func TestPodTemplate_RendersPodSpecFields(t *testing.T) {
	tmpl := podTemplate("app", 2, "", "Deployment/web")
	for _, want := range []string{
		`hostname: {{ . | quote }}`,
		`subdomain: {{ . | quote }}`,
		`priorityClassName: {{ . | quote }}`,
		`runtimeClassName: {{ . | quote }}`,
		`dnsPolicy: {{ . | quote }}`,
		`hostAliases:`,
		`dnsConfig:`,
		`shareProcessNamespace: {{ . }}`,
		`enableServiceLinks: {{ .enableServiceLinks }}`,
		`terminationGracePeriodSeconds: {{ .terminationGracePeriodSeconds | int64 }}`,
	} {
		if !strings.Contains(tmpl, want) {
			t.Errorf("podTemplate lacks %q:\n%s", want, tmpl)
		}
	}
}

// Spot injection (Deployment, StatefulSet) writes terminationGracePeriodSeconds
// itself when spot is enabled; the pod template must not emit it a second time.
func TestPodTemplate_GracePeriodYieldsToSpotOnlyForSpotKinds(t *testing.T) {
	const guard = `not (and $.Values.spot $.Values.spot.enabled)`
	for _, key := range []string{"Deployment/web", "StatefulSet/web"} {
		if !strings.Contains(podTemplate("app", 2, "", key), guard) {
			t.Errorf("%s: grace period is not guarded against the spot block", key)
		}
	}
	for _, key := range []string{"DaemonSet/web", "Job/web", "CronJob/web"} {
		if strings.Contains(podTemplate("app", 2, "", key), guard) {
			t.Errorf("%s: spot never touches this kind, no guard expected", key)
		}
	}
}

func TestHelmTemplatePodSpecFields(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		if os.Getenv("DHG_REQUIRE_HELM") != "" {
			t.Fatalf("helm not found: %v", err)
		}
		t.Skip("helm not installed")
	}
	dir := t.TempDir()
	input := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  selector:
    matchLabels: {app: web}
  template:
    metadata:
      labels: {app: web}
    spec:
      hostname: web-host
      subdomain: web-sub
      terminationGracePeriodSeconds: 0
      priorityClassName: high
      dnsPolicy: None
      dnsConfig:
        nameservers: ["1.1.1.1"]
      runtimeClassName: gvisor
      shareProcessNamespace: true
      enableServiceLinks: false
      hostAliases:
      - ip: 10.0.0.1
        hostnames: [db.local]
      containers:
      - name: app
        image: nginx
`
	inputDir := filepath.Join(dir, "input")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "m.yaml"), []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	gen := exec.Command("go", "run", "../../../cmd/dhg", "generate", "-f", inputDir, "--chart-name", "app", "--mode", "universal", "-o", outDir)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("dhg generate failed: %v\n%s", err, out)
	}
	helmPath := "helm"
	if p := os.Getenv("DHG_HELM_PATH"); p != "" {
		helmPath = p
	}
	out, err := exec.Command(helmPath, "template", "r", filepath.Join(outDir, "app")).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\n%s", err, out)
	}
	rendered := string(out)
	for _, want := range []string{
		`hostname: "web-host"`,
		`subdomain: "web-sub"`,
		`terminationGracePeriodSeconds: 0`,
		`priorityClassName: "high"`,
		`dnsPolicy: "None"`,
		`runtimeClassName: "gvisor"`,
		`shareProcessNamespace: true`,
		`enableServiceLinks: false`,
		`hostAliases:`,
		`- 1.1.1.1`,
		`- db.local`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered chart lacks %q:\n%s", want, rendered)
		}
	}
}
