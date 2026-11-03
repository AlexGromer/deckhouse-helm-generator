package k8s

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// unstableLabels change with the chart or app version; in a pod template they
// restart the pods on every version bump and on adoption of a running
// workload.
var unstableLabels = []string{"helm.sh/chart", "app.kubernetes.io/version", "app.kubernetes.io/managed-by"}

func TestPodTemplateOmitsVersionedLabels(t *testing.T) {
	got := podTemplate("app", 2, "", "Deployment/web")
	for _, key := range unstableLabels {
		if !strings.Contains(got, `"`+key+`"`) {
			t.Errorf("podTemplate does not drop %q from the pod labels:\n%s", key, got)
		}
	}
	if !strings.Contains(got, `omit (include "app.labels" $ | fromYaml)`) {
		t.Errorf("pod labels must come from the chart labels with the versioned ones omitted:\n%s", got)
	}
}

// renderDocs renders the chart with helm and returns the documents by kind.
func renderDocs(t *testing.T, chart string, args ...string) map[string]map[string]interface{} {
	t.Helper()
	helmPath := "helm"
	if p := os.Getenv("DHG_HELM_PATH"); p != "" {
		helmPath = p
	}
	out, err := exec.Command(helmPath, append([]string{"template", "r", chart}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template %v failed: %v\n%s", args, err, out)
	}
	docs := map[string]map[string]interface{}{}
	for _, part := range strings.Split(string(out), "\n---\n") {
		var doc map[string]interface{}
		if err := yaml.Unmarshal([]byte(part), &doc); err != nil || doc == nil {
			continue
		}
		if kind, _ := doc["kind"].(string); kind != "" {
			docs[kind] = doc
		}
	}
	return docs
}

func nestedMap(t *testing.T, m map[string]interface{}, path ...string) map[string]interface{} {
	t.Helper()
	cur := m
	for _, p := range path {
		next, ok := cur[p].(map[string]interface{})
		if !ok {
			t.Fatalf("no map at %v (stuck at %q) in %v", path, p, cur)
		}
		cur = next
	}
	return cur
}

func TestHelmTemplatePodLabelsAreStable(t *testing.T) {
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
      labels: {app: web, tier: front}
    spec:
      containers:
      - name: app
        image: reg:5000/app:v1
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: db
spec:
  serviceName: db
  selector:
    matchLabels: {app: db}
  template:
    metadata:
      labels: {app: db}
    spec:
      containers:
      - name: db
        image: reg:5000/db:v1
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: agent
spec:
  selector:
    matchLabels: {app: agent}
  template:
    metadata:
      labels: {app: agent}
    spec:
      containers:
      - name: agent
        image: reg:5000/agent:v1
---
apiVersion: batch/v1
kind: Job
metadata:
  name: migrate
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: migrate
        image: reg:5000/migrate:v1
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: nightly
spec:
  schedule: "0 1 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
          - name: nightly
            image: reg:5000/nightly:v1
`
	inputDir := filepath.Join(dir, "input")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "w.yaml"), []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	generate := func(out, chartVersion, appVersion string) string {
		t.Helper()
		cmd := exec.Command("go", "run", "../../../cmd/dhg", "generate", "-f", inputDir, "--chart-name", "app",
			"--mode", "universal", "--chart-version", chartVersion, "--app-version", appVersion, "-o", filepath.Join(dir, out))
		if o, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("dhg generate failed: %v\n%s", err, o)
		}
		return filepath.Join(dir, out, "app")
	}
	podTemplates := func(docs map[string]map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"Deployment":  nestedMap(t, docs["Deployment"], "spec", "template"),
			"StatefulSet": nestedMap(t, docs["StatefulSet"], "spec", "template"),
			"DaemonSet":   nestedMap(t, docs["DaemonSet"], "spec", "template"),
			"Job":         nestedMap(t, docs["Job"], "spec", "template"),
			"CronJob":     nestedMap(t, docs["CronJob"], "spec", "jobTemplate", "spec", "template"),
		}
	}

	chart1 := generate("out1", "0.1.0", "1.0.0")
	v1 := renderDocs(t, chart1)
	helmPath := "helm"
	if p := os.Getenv("DHG_HELM_PATH"); p != "" {
		helmPath = p
	}
	// helm renders NOTES.txt only on install, and even --dry-run=client needs a
	// reachable cluster; without one (CI) the NOTES text is covered by the
	// pkg/helm and pkg/generator tests, and the label checks below still run.
	notes, err := exec.Command(helmPath, "install", "rel", chart1, "-n", "prod", "--dry-run=client").CombinedOutput()
	switch {
	case err != nil && strings.Contains(string(notes), "cluster unreachable"):
		t.Logf("no reachable cluster, rendered NOTES not checked: %s", strings.TrimSpace(string(notes)))
	case err != nil:
		t.Fatalf("helm install --dry-run failed: %v\n%s", err, notes)
	default:
		for _, want := range []string{
			"kubectl -n prod rollout status deploy/web --timeout=30m",
			"helm get manifest rel -n prod | kubectl -n prod diff -f -",
		} {
			if !strings.Contains(string(notes), want) {
				t.Errorf("rendered NOTES lacks %q:\n%s", want, notes)
			}
		}
	}
	v2 := renderDocs(t, generate("out2", "0.2.0", "2.0.0"))
	for kind, tmpl := range podTemplates(v1) {
		podLabels := nestedMap(t, tmpl.(map[string]interface{}), "metadata", "labels")
		for _, key := range unstableLabels {
			if _, has := podLabels[key]; has {
				t.Errorf("%s pod template carries %q: %v", kind, key, podLabels)
			}
		}
		if podLabels["app.kubernetes.io/name"] == nil || podLabels["app.kubernetes.io/instance"] == nil {
			t.Errorf("%s pod template lost the stable chart labels: %v", kind, podLabels)
		}
	}
	// The object's own labels keep the version labels.
	for kind, doc := range v1 {
		if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" && kind != "Job" && kind != "CronJob" {
			continue
		}
		objLabels := nestedMap(t, doc, "metadata", "labels")
		for _, key := range unstableLabels {
			if _, has := objLabels[key]; !has {
				t.Errorf("%s metadata.labels lost %q: %v", kind, key, objLabels)
			}
		}
	}
	// The input's pod labels are kept.
	if got := nestedMap(t, v1["Deployment"], "spec", "template", "metadata", "labels"); got["app"] != "web" || got["tier"] != "front" {
		t.Errorf("Deployment pod labels lost the input's labels: %v", got)
	}
	// A new chart and app version leaves every pod template byte for byte.
	if p1, p2 := podTemplates(v1), podTemplates(v2); !reflect.DeepEqual(p1, p2) {
		t.Errorf("pod templates differ between chart versions:\n%v\n%v", p1, p2)
	}
	if l1, l2 := nestedMap(t, v1["Deployment"], "metadata", "labels"), nestedMap(t, v2["Deployment"], "metadata", "labels"); reflect.DeepEqual(l1, l2) {
		t.Errorf("object labels must follow the chart version, got %v", l1)
	}
}
