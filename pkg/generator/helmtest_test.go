package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestGenerateHelmTests_BasicDeployment(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\nspec:\n  replicas: 1",
		},
	}

	tests := GenerateHelmTests(chart)

	if len(tests) != 1 {
		t.Fatalf("expected 1 test file, got %d", len(tests))
	}

	content, ok := tests["tests/deployment_test.yaml"]
	if !ok {
		t.Fatal("expected tests/deployment_test.yaml to exist")
	}

	// Verify suite name
	if !strings.Contains(content, "suite: test deployment") {
		t.Error("expected suite name 'test deployment'")
	}

	// Verify template reference
	if !strings.Contains(content, "- templates/deployment.yaml") {
		t.Error("expected template reference to templates/deployment.yaml")
	}

	// Verify basic render test with isKind
	if !strings.Contains(content, "- it: should render") {
		t.Error("expected 'should render' test")
	}
	if !strings.Contains(content, "isKind:") {
		t.Error("expected isKind assertion")
	}
	if !strings.Contains(content, "of: Deployment") {
		t.Error("expected Kind to be Deployment")
	}

	// Should NOT have disabled test (no feature flag)
	if strings.Contains(content, "should not render when disabled") {
		t.Error("should not have disabled test for non-feature-flagged template")
	}
}

func TestGenerateHelmTests_SkipsHelpers(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/_helpers.tpl":    "{{- define \"myapp.name\" -}}\nmyapp\n{{- end -}}",
			"templates/NOTES.txt":       "Thank you for installing myapp.",
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp",
		},
	}

	tests := GenerateHelmTests(chart)

	if len(tests) != 1 {
		t.Fatalf("expected 1 test file (only deployment), got %d", len(tests))
	}

	if _, ok := tests["tests/_helpers_test.yaml"]; ok {
		t.Error("should not generate test for _helpers.tpl")
	}
	if _, ok := tests["tests/NOTES_test.yaml"]; ok {
		t.Error("should not generate test for NOTES.txt")
	}
	if _, ok := tests["tests/deployment_test.yaml"]; !ok {
		t.Error("should generate test for deployment.yaml")
	}
}

func TestGenerateHelmTests_FeatureFlaggedTemplate(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/servicemonitor.yaml": "{{- if .Values.features.monitoring }}\napiVersion: monitoring.coreos.com/v1\nkind: ServiceMonitor\nmetadata:\n  name: myapp\n{{- end }}",
		},
	}

	tests := GenerateHelmTests(chart)

	if len(tests) != 1 {
		t.Fatalf("expected 1 test file, got %d", len(tests))
	}

	content, ok := tests["tests/servicemonitor_test.yaml"]
	if !ok {
		t.Fatal("expected tests/servicemonitor_test.yaml to exist")
	}

	// Verify basic render test
	if !strings.Contains(content, "- it: should render") {
		t.Error("expected 'should render' test")
	}
	if !strings.Contains(content, "of: ServiceMonitor") {
		t.Error("expected Kind to be ServiceMonitor")
	}

	// Verify disabled test exists
	if !strings.Contains(content, "should not render when disabled") {
		t.Error("expected disabled test for feature-flagged template")
	}
	if !strings.Contains(content, "features.monitoring: false") {
		t.Error("expected set features.monitoring: false in disabled test")
	}
	if !strings.Contains(content, "hasDocuments:") {
		t.Error("expected hasDocuments assertion")
	}
	if !strings.Contains(content, "count: 0") {
		t.Error("expected count: 0 in disabled assertion")
	}
}

func TestGenerateHelmTests_EmptyChart(t *testing.T) {
	// Nil chart
	tests := GenerateHelmTests(nil)
	if tests != nil {
		t.Error("expected nil for nil chart")
	}

	// Empty templates
	chart := &types.GeneratedChart{
		Name:      "myapp",
		Templates: map[string]string{},
	}
	tests = GenerateHelmTests(chart)
	if tests != nil {
		t.Errorf("expected nil for empty templates, got %d files", len(tests))
	}

	// Only helpers (all skipped)
	chart = &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/_helpers.tpl": "{{- define \"myapp.name\" -}}myapp{{- end -}}",
			"templates/NOTES.txt":   "Thank you for installing.",
		},
	}
	tests = GenerateHelmTests(chart)
	if tests != nil {
		t.Errorf("expected nil when all templates are skipped, got %d files", len(tests))
	}
}

func TestGenerateHelmTests_MultipleTemplates(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp",
			"templates/service.yaml":    "apiVersion: v1\nkind: Service\nmetadata:\n  name: myapp",
			"templates/_helpers.tpl":    "{{- define \"myapp.name\" -}}myapp{{- end -}}",
		},
	}

	tests := GenerateHelmTests(chart)

	if len(tests) != 2 {
		t.Fatalf("expected 2 test files, got %d", len(tests))
	}

	deployContent := tests["tests/deployment_test.yaml"]
	if !strings.Contains(deployContent, "of: Deployment") {
		t.Error("deployment test should assert Deployment kind")
	}

	svcContent := tests["tests/service_test.yaml"]
	if !strings.Contains(svcContent, "of: Service") {
		t.Error("service test should assert Service kind")
	}
}

func TestExtractKindFromTemplate(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"deployment", "apiVersion: apps/v1\nkind: Deployment\nmetadata:", "Deployment"},
		{"service", "apiVersion: v1\nkind: Service\nmetadata:", "Service"},
		{"no kind", "apiVersion: v1\nmetadata:", ""},
		{"kind in comment", "# kind: Fake\nkind: StatefulSet", "StatefulSet"},
		{"templated prefix", "{{- if .Values.enabled }}\napiVersion: apps/v1\nkind: DaemonSet", "DaemonSet"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractKindFromTemplate(tt.content)
			if got != tt.want {
				t.Errorf("extractKindFromTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractFeatureFlag(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			"monitoring feature flag",
			"{{- if .Values.features.monitoring }}\napiVersion: monitoring.coreos.com/v1\nkind: ServiceMonitor",
			"features.monitoring",
		},
		{
			"ingress enabled",
			"{{- if .Values.ingress.enabled }}\napiVersion: networking.k8s.io/v1\nkind: Ingress",
			"ingress.enabled",
		},
		{
			"no feature flag",
			"apiVersion: apps/v1\nkind: Deployment",
			"",
		},
		{
			"feature flag not at start (deep in template)",
			strings.Repeat("x", 600) + "{{- if .Values.features.late }}",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFeatureFlag(tt.content)
			if got != tt.want {
				t.Errorf("extractFeatureFlag() = %q, want %q", got, tt.want)
			}
		})
	}
}
