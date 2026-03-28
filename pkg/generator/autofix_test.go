package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

const testDeploymentTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: app
        image: nginx:1.25
        ports:
        - containerPort: 8080`

const testStatefulSetTemplate = `apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: test-db
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: db
        image: postgres:16
        ports:
        - containerPort: 5432`

const testServiceTemplate = `apiVersion: v1
kind: Service
metadata:
  name: test-svc
spec:
  ports:
  - port: 80`

func newTestChart(templates map[string]string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:      "test-chart",
		Templates: templates,
	}
}

func TestInjectSecurityContext(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
		"templates/service.yaml":    testServiceTemplate,
	})

	result, count := InjectSecurityContext(chart)

	if count != 1 {
		t.Errorf("expected 1 injection, got %d", count)
	}

	content := result.Templates["templates/deployment.yaml"]
	for _, field := range []string{"runAsNonRoot: true", "readOnlyRootFilesystem: true", "allowPrivilegeEscalation: false", "drop:", "- ALL"} {
		if !strings.Contains(content, field) {
			t.Errorf("missing field %q in injected template", field)
		}
	}

	// Service should be unchanged.
	if result.Templates["templates/service.yaml"] != testServiceTemplate {
		t.Error("service template should not be modified")
	}
}

func TestInjectSecurityContext_Idempotent(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	result, _ := InjectSecurityContext(chart)
	result2, count := InjectSecurityContext(result)

	if count != 0 {
		t.Errorf("expected 0 injections on second pass, got %d", count)
	}

	if result.Templates["templates/deployment.yaml"] != result2.Templates["templates/deployment.yaml"] {
		t.Error("second injection should not change the template")
	}
}

func TestInjectSecurityContext_CopyOnWrite(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	result, _ := InjectSecurityContext(chart)

	if chart.Templates["templates/deployment.yaml"] != testDeploymentTemplate {
		t.Error("original chart should not be mutated")
	}
	if result.Templates["templates/deployment.yaml"] == testDeploymentTemplate {
		t.Error("result should differ from original")
	}
}

func TestInjectResourceDefaults(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
		"templates/service.yaml":    testServiceTemplate,
	})

	result, count := InjectResourceDefaults(chart, WorkloadWeb)

	if count != 1 {
		t.Errorf("expected 1 injection, got %d", count)
	}

	content := result.Templates["templates/deployment.yaml"]
	for _, field := range []string{"resources:", "cpu:", "memory:", "requests:", "limits:"} {
		if !strings.Contains(content, field) {
			t.Errorf("missing field %q in injected template", field)
		}
	}
}

func TestInjectResourceDefaults_SkipExisting(t *testing.T) {
	tmpl := testDeploymentTemplate + "\n        resources:\n          requests:\n            cpu: 100m"
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": tmpl,
	})

	_, count := InjectResourceDefaults(chart, WorkloadWeb)
	if count != 0 {
		t.Errorf("expected 0 injections when resources exist, got %d", count)
	}
}

func TestInjectResourceDefaults_UnknownWorkload(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	result, count := InjectResourceDefaults(chart, "unknown")
	if count != 1 {
		t.Errorf("expected 1 injection with fallback profile, got %d", count)
	}

	content := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(content, "resources:") {
		t.Error("should inject default web profile for unknown workload type")
	}
}

func TestInjectHealthProbes_HTTP(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	result, count := InjectHealthProbes(chart)

	if count != 1 {
		t.Errorf("expected 1 injection, got %d", count)
	}

	content := result.Templates["templates/deployment.yaml"]
	for _, field := range []string{"livenessProbe:", "readinessProbe:", "startupProbe:", "/healthz", "8080"} {
		if !strings.Contains(content, field) {
			t.Errorf("missing field %q in probes", field)
		}
	}
}

func TestInjectHealthProbes_TCP(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/statefulset.yaml": testStatefulSetTemplate,
	})

	result, count := InjectHealthProbes(chart)

	if count != 1 {
		t.Errorf("expected 1 injection, got %d", count)
	}

	content := result.Templates["templates/statefulset.yaml"]
	if !strings.Contains(content, "tcpSocket:") {
		t.Error("expected TCP probe for non-HTTP port 5432")
	}
	if strings.Contains(content, "httpGet:") {
		t.Error("should not use httpGet for non-HTTP port")
	}
}

func TestInjectHealthProbes_Idempotent(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	result, _ := InjectHealthProbes(chart)
	_, count := InjectHealthProbes(result)

	if count != 0 {
		t.Errorf("expected 0 injections on second pass, got %d", count)
	}
}

func TestInjectPDB(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml":  testDeploymentTemplate, // replicas: 3
		"templates/statefulset.yaml": testStatefulSetTemplate, // replicas: 2
	})

	result, count := InjectPDB(chart)

	if count != 2 {
		t.Errorf("expected 2 PDBs generated, got %d", count)
	}

	pdb1 := result.Templates["templates/deployment-pdb.yaml"]
	if !strings.Contains(pdb1, "PodDisruptionBudget") {
		t.Error("expected PDB for deployment")
	}
	// replicas=3 → "50%"
	if !strings.Contains(pdb1, `"50%"`) {
		t.Errorf("expected 50%% minAvailable for 3 replicas, got:\n%s", pdb1)
	}

	pdb2 := result.Templates["templates/statefulset-pdb.yaml"]
	if !strings.Contains(pdb2, "PodDisruptionBudget") {
		t.Error("expected PDB for statefulset")
	}
	// replicas=2 → minAvailable: 1
	if !strings.Contains(pdb2, "minAvailable: 1") {
		t.Errorf("expected minAvailable: 1 for 2 replicas, got:\n%s", pdb2)
	}
}

func TestInjectPDB_SkipLowReplicas(t *testing.T) {
	tmpl := strings.Replace(testDeploymentTemplate, "replicas: 3", "replicas: 1", 1)
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": tmpl,
	})

	_, count := InjectPDB(chart)
	if count != 0 {
		t.Errorf("expected 0 PDBs for 1 replica, got %d", count)
	}
}

func TestInjectPDB_SkipNonWorkload(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/service.yaml": testServiceTemplate,
	})

	_, count := InjectPDB(chart)
	if count != 0 {
		t.Errorf("expected 0 PDBs for service, got %d", count)
	}
}

func TestInjectPSSRestricted(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	result, count := InjectPSSRestricted(chart)

	if count != 1 {
		t.Errorf("expected 1 PSS injection, got %d", count)
	}

	content := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(content, "seccompProfile") {
		t.Error("missing seccompProfile in PSS restricted injection")
	}
	if !strings.Contains(content, "RuntimeDefault") {
		t.Error("missing RuntimeDefault seccomp type")
	}
}

func TestInjectGracefulShutdown(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml":  testDeploymentTemplate,
		"templates/statefulset.yaml": testStatefulSetTemplate,
	})

	result, count := InjectGracefulShutdown(chart)

	if count != 2 {
		t.Errorf("expected 2 injections, got %d", count)
	}

	deploy := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(deploy, "preStop:") {
		t.Error("missing preStop in deployment")
	}
	if !strings.Contains(deploy, "terminationGracePeriodSeconds:") {
		t.Error("missing terminationGracePeriodSeconds in deployment")
	}
	// Deployment grace=30, preStop sleep=10
	if !strings.Contains(deploy, `sleep 10`) {
		t.Error("expected sleep 10 for deployment (30/3)")
	}

	sts := result.Templates["templates/statefulset.yaml"]
	// StatefulSet grace=60, preStop sleep=20
	if !strings.Contains(sts, `sleep 20`) {
		t.Error("expected sleep 20 for statefulset (60/3)")
	}
	if !strings.Contains(sts, "terminationGracePeriodSeconds: 60") {
		t.Error("expected terminationGracePeriodSeconds: 60 for statefulset")
	}
}

func TestInjectGracefulShutdown_Idempotent(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	result, _ := InjectGracefulShutdown(chart)
	_, count := InjectGracefulShutdown(result)

	if count != 0 {
		t.Errorf("expected 0 injections on second pass, got %d", count)
	}
}

func TestApplyAllFixes(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml":  testDeploymentTemplate,
		"templates/statefulset.yaml": testStatefulSetTemplate,
		"templates/service.yaml":     testServiceTemplate,
	})

	result, fixes := ApplyAllFixes(chart, WorkloadWeb)

	if fixes.SecurityContextInjected == 0 {
		t.Error("expected security context injections")
	}
	if fixes.ResourcesInjected == 0 {
		t.Error("expected resource injections")
	}
	if fixes.PDBsGenerated == 0 {
		t.Error("expected PDB generation")
	}

	// Original should be untouched.
	if chart.Templates["templates/deployment.yaml"] != testDeploymentTemplate {
		t.Error("original chart should not be mutated")
	}

	// Result should have more templates (PDBs).
	if len(result.Templates) <= len(chart.Templates) {
		t.Error("result should have additional PDB templates")
	}
}

func TestDetectContainerPort(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"explicit port", "  containerPort: 3000", "3000"},
		{"no port", "  image: nginx", "8080"},
		{"port 80", "  containerPort: 80", "80"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectContainerPort(tt.content)
			if got != tt.want {
				t.Errorf("detectContainerPort() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectWorkloadKind(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"deployment", "kind: Deployment", "Deployment"},
		{"statefulset", "kind: StatefulSet", "StatefulSet"},
		{"daemonset", "kind: DaemonSet", "DaemonSet"},
		{"unknown", "kind: Service", "Deployment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectWorkloadKind(tt.content)
			if got != tt.want {
				t.Errorf("detectWorkloadKind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInjectHealthProbes_Port80_HTTP(t *testing.T) {
	tmpl := strings.Replace(testDeploymentTemplate, "containerPort: 8080", "containerPort: 80", 1)
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": tmpl,
	})

	result, count := InjectHealthProbes(chart)

	if count != 1 {
		t.Errorf("expected 1 injection, got %d", count)
	}

	content := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(content, "httpGet:") {
		t.Error("expected HTTP probe for port 80")
	}
	if !strings.Contains(content, "/healthz") {
		t.Error("expected /healthz path")
	}
}

func TestInjectHealthProbes_Port3000_HTTP(t *testing.T) {
	tmpl := strings.Replace(testDeploymentTemplate, "containerPort: 8080", "containerPort: 3000", 1)
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": tmpl,
	})

	result, _ := InjectHealthProbes(chart)
	content := result.Templates["templates/deployment.yaml"]

	if !strings.Contains(content, "httpGet:") {
		t.Error("expected HTTP probe for port 3000")
	}
}
