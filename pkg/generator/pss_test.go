package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func makeChartWithNoSecurityContext() *types.GeneratedChart {
	return &types.GeneratedChart{
		Name: "test-chart",
		Templates: map[string]string{
			"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: insecure-app
spec:
  template:
    spec:
      containers:
      - name: app
        image: nginx:latest
`,
			"templates/service.yaml": `apiVersion: v1
kind: Service
metadata:
  name: my-svc
spec:
  ports:
  - port: 80
`,
		},
	}
}

func TestPSS_InjectDefaults(t *testing.T) {
	chart := makeChartWithNoSecurityContext()
	result := InjectPSSDefaults(chart, "restricted")

	deployment := result.Templates["templates/deployment.yaml"]
	requiredFields := []string{
		"runAsNonRoot: true",
		"readOnlyRootFilesystem: true",
		"allowPrivilegeEscalation: false",
		"drop:",
		"- ALL",
	}
	for _, field := range requiredFields {
		if !strings.Contains(deployment, field) {
			t.Errorf("injected deployment missing required field %q", field)
		}
	}

	// Service should be unchanged.
	svc := result.Templates["templates/service.yaml"]
	if strings.Contains(svc, "securityContext") {
		t.Error("service template should not have securityContext injected")
	}
}

func TestPSS_InjectBaseline(t *testing.T) {
	chart := makeChartWithNoSecurityContext()
	result := InjectPSSDefaults(chart, "baseline")

	deployment := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(deployment, "runAsNonRoot: true") {
		t.Error("baseline injection should include runAsNonRoot")
	}
	// Baseline should not require readOnlyRootFilesystem.
	if strings.Contains(deployment, "readOnlyRootFilesystem") {
		t.Error("baseline injection should not include readOnlyRootFilesystem")
	}
}

func TestPSS_CopyOnWrite(t *testing.T) {
	chart := makeChartWithNoSecurityContext()
	origDeployment := chart.Templates["templates/deployment.yaml"]

	_ = InjectPSSDefaults(chart, "restricted")

	if chart.Templates["templates/deployment.yaml"] != origDeployment {
		t.Error("original chart templates were mutated — copy-on-write violated")
	}
}

func TestPSS_SkipsStatefulSetWithExistingContext(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "test-chart",
		Templates: map[string]string{
			"templates/statefulset.yaml": `apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: db
spec:
  template:
    spec:
      containers:
      - name: postgres
        securityContext:
          runAsNonRoot: true
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
          seccompProfile:
            type: RuntimeDefault
`,
		},
	}

	result := InjectPSSDefaults(chart, "restricted")
	// Should not double-inject if all fields present.
	count := strings.Count(result.Templates["templates/statefulset.yaml"], "runAsNonRoot")
	if count != 1 {
		t.Errorf("expected 1 occurrence of runAsNonRoot, got %d", count)
	}
}
