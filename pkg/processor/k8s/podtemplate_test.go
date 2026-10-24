package k8s

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

func TestPodTemplateParses(t *testing.T) {
	tests := []struct {
		name          string
		chartName     string
		indent        int
		restartPolicy string
		workloadKey   string
	}{
		{"deployment", "app", 2, "", "Deployment/web"},
		{"cronjob", "app", 6, "OnFailure", "CronJob/x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := podTemplate(tt.chartName, tt.indent, tt.restartPolicy, tt.workloadKey)

			tmpl := template.New("test").Option("missingkey=error").Funcs(template.FuncMap{
				"nindent":  func() string { return "" },
				"include":  func() string { return "" },
				"fromYaml": func() string { return "" },
				"merge":    func() string { return "" },
				"dict":     func() string { return "" },
				"default":  func() string { return "" },
				"concat":   func() string { return "" },
				"list":     func() string { return "" },
				"toYaml":   func() string { return "" },
				"quote":    func() string { return "" },
				"tpl":      func() string { return "" },
				"kindIs":   func() string { return "" },
				"get":      func() string { return "" },
				"set":      func() string { return "" },
			})

			_, err := tmpl.Parse(got)
			if err != nil {
				t.Errorf("podTemplate output failed to parse: %v\nOutput was:\n%s", err, got)
			}
		})
	}
}

func TestPodTemplateInjectionKey(t *testing.T) {
	got := podTemplate("app", 2, "", "Deployment/web")
	expected := `index $dhgBlock.inject "Deployment/web"`
	if !strings.Contains(got, expected) {
		t.Errorf("podTemplate missing %q", expected)
	}
}

func TestContainerTemplateConcat(t *testing.T) {
	got := containerTemplate(6)

	checks := []string{
		`concat (.env | default list) ($dhgInj.env | default list)`,
		`concat (.envFrom | default list) ($dhgInj.envFrom | default list)`,
		`concat (.volumeMounts | default list) ($dhgInj.volumeMounts | default list)`,
	}

	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Errorf("containerTemplate missing %q", check)
		}
	}
}

func TestHelmTemplateInjection(t *testing.T) {
	// The test needs to run `dhg generate` and then `helm template` on the output.
	dir := t.TempDir()

	inputManifests := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: orders-api
spec:
  template:
    spec:
      containers:
      - name: app
        image: nginx
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: orders-api
spec:
  template:
    spec:
      containers:
      - name: app
        image: nginx
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: orders-api
spec:
  schedule: "0 0 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: app
            image: nginx
`
	inputDir := filepath.Join(dir, "input")
	_ = os.MkdirAll(inputDir, 0755)
	_ = os.WriteFile(filepath.Join(inputDir, "manifests.yaml"), []byte(inputManifests), 0644)

	outDir := filepath.Join(dir, "out")

	// Compile and run dhg
	args := []string{"run", "../../../cmd/dhg", "generate", "-f", inputDir, "--chart-name", "app", "--mode", "universal", "-o", outDir}
	cmd := exec.Command("go", args...)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dhg generate failed: %v\nOutput: %s", err, string(out))
	}

	valuesEnabled := `
kafka:
  enabled: true
  marker: injected-kafka
  inject:
    Deployment/orders-api:
      volumes: [{name: kafka-certs, emptyDir: {}}]
      containers:
        app:
          env: [{name: KAFKA_ENABLED, value: '{{ .Values.kafka.marker }}'}]
          volumeMounts: [{name: kafka-certs, mountPath: /certs}]
    StatefulSet/orders-api:
      volumes: [{name: kafka-certs, emptyDir: {}}]
      containers:
        app:
          env: [{name: KAFKA_ENABLED, value: '{{ .Values.kafka.marker }}'}]
          volumeMounts: [{name: kafka-certs, mountPath: /certs}]
    CronJob/orders-api:
      volumes: [{name: kafka-certs, emptyDir: {}}]
      containers:
        app:
          env: [{name: KAFKA_ENABLED, value: '{{ .Values.kafka.marker }}'}]
          volumeMounts: [{name: kafka-certs, mountPath: /certs}]
`
	valuesFile := filepath.Join(dir, "values.yaml")
	_ = os.WriteFile(valuesFile, []byte(valuesEnabled), 0644)

	helmPath := "helm"
	if p := os.Getenv("DHG_HELM_PATH"); p != "" {
		helmPath = p
	}

	cmd = exec.Command(helmPath, "template", "test-release", filepath.Join(outDir, "app"), "-f", valuesFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\nOutput: %s", err, string(out))
	}
	rendered := string(out)

	countEnv := strings.Count(rendered, "KAFKA_ENABLED")
	countVol := strings.Count(rendered, "name: kafka-certs")

	// We expect the injection in 3 resources
	if countEnv < 3 {
		t.Errorf("Expected 3 injected KAFKA_ENABLED, got %d", countEnv)
	}
	if countVol < 3 {
		t.Errorf("Expected at least 3 injected volumes, got %d", countVol)
	}

	// Test disabled
	valuesDisabled := `
kafka:
  enabled: false
  marker: injected-kafka
  inject:
    Deployment/orders-api:
      volumes: [{name: kafka-certs, emptyDir: {}}]
      containers:
        app:
          env: [{name: KAFKA_ENABLED, value: '{{ .Values.kafka.marker }}'}]
          volumeMounts: [{name: kafka-certs, mountPath: /certs}]
`
	_ = os.WriteFile(valuesFile, []byte(valuesDisabled), 0644)
	cmd = exec.Command(helmPath, "template", "test-release", filepath.Join(outDir, "app"), "-f", valuesFile)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template disabled failed: %v", err)
	}
	renderedFalse := string(out)
	if strings.Contains(renderedFalse, "KAFKA_ENABLED") {
		t.Errorf("Output should not contain KAFKA_ENABLED when enabled=false")
	}
}
