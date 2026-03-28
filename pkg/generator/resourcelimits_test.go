package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func makeChartWithContainerNoResources() *types.GeneratedChart {
	return &types.GeneratedChart{
		Name: "test-chart",
		Templates: map[string]string{
			"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
spec:
  template:
    spec:
      containers:
      - name: app
        image: nginx:latest
`,
		},
	}
}

func makeChartWithExistingResources() *types.GeneratedChart {
	return &types.GeneratedChart{
		Name: "test-chart",
		Templates: map[string]string{
			"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
spec:
  template:
    spec:
      containers:
      - name: app
        image: nginx:latest
        resources:
          requests:
            cpu: "200m"
            memory: "256Mi"
          limits:
            cpu: "1"
            memory: "1Gi"
`,
		},
	}
}

func TestResourceLimits_WebProfile(t *testing.T) {
	chart := makeChartWithContainerNoResources()
	result := InjectResourceLimits(chart, WorkloadWeb)

	deployment := result.Templates["templates/deployment.yaml"]
	requiredFields := []string{
		"resources:",
		"requests:",
		"limits:",
		"cpu:",
		"memory:",
	}
	for _, field := range requiredFields {
		if !strings.Contains(deployment, field) {
			t.Errorf("web profile missing %q in deployment", field)
		}
	}
	// Check web-specific values.
	if !strings.Contains(deployment, "100m") {
		t.Error("web profile should have cpu request 100m")
	}
	if !strings.Contains(deployment, "128Mi") {
		t.Error("web profile should have memory request 128Mi")
	}
}

func TestResourceLimits_DatabaseProfile(t *testing.T) {
	chart := makeChartWithContainerNoResources()
	result := InjectResourceLimits(chart, WorkloadDatabase)

	deployment := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(deployment, "500m") {
		t.Error("database profile should have cpu request 500m")
	}
	if !strings.Contains(deployment, "512Mi") {
		t.Error("database profile should have memory request 512Mi")
	}
	if !strings.Contains(deployment, "4Gi") {
		t.Error("database profile should have memory limit 4Gi")
	}
}

func TestResourceLimits_WorkerProfile(t *testing.T) {
	chart := makeChartWithContainerNoResources()
	result := InjectResourceLimits(chart, WorkloadWorker)

	deployment := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(deployment, "250m") {
		t.Error("worker profile should have cpu request 250m")
	}
	if !strings.Contains(deployment, "256Mi") {
		t.Error("worker profile should have memory request 256Mi")
	}
	if !strings.Contains(deployment, "1Gi") {
		t.Error("worker profile should have memory limit 1Gi")
	}
}

func TestResourceLimits_SkipsExistingLimits(t *testing.T) {
	chart := makeChartWithExistingResources()
	origDeployment := chart.Templates["templates/deployment.yaml"]

	result := InjectResourceLimits(chart, WorkloadWeb)

	// Template with existing resources should be preserved as-is.
	if result.Templates["templates/deployment.yaml"] != origDeployment {
		t.Error("containers with existing resources should not be modified")
	}
}

func TestResourceLimits_CopyOnWrite(t *testing.T) {
	chart := makeChartWithContainerNoResources()
	origDeployment := chart.Templates["templates/deployment.yaml"]

	_ = InjectResourceLimits(chart, WorkloadWeb)

	if chart.Templates["templates/deployment.yaml"] != origDeployment {
		t.Error("original chart was mutated — copy-on-write violated")
	}
}

func TestResourceLimits_CacheProfile(t *testing.T) {
	chart := makeChartWithContainerNoResources()
	result := InjectResourceLimits(chart, WorkloadCache)

	deployment := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(deployment, "64Mi") {
		t.Error("cache profile should have memory request 64Mi")
	}
	if !strings.Contains(deployment, "256Mi") {
		t.Error("cache profile should have memory limit 256Mi")
	}
}
