package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test 1: Chart with only Deployment+Service — no wrapping
// ============================================================

func TestFeatureFlags_ChartWithOnlyDeploymentService_NoWrapping(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\nspec:\n  replicas: 1",
		"templates/service.yaml":    "apiVersion: v1\nkind: Service\nmetadata:\n  name: myapp\nspec:\n  selector:\n    app: myapp",
	})

	config := DefaultFeatureFlagConfig()
	result := InjectFeatureFlags(chart, config)

	// No template should be wrapped with feature flag guards
	for path, content := range result.Templates {
		if strings.Contains(content, "{{- if .Values.features.") {
			t.Errorf("template %q should not be wrapped with feature flags (workload kind), got:\n%s", path, content)
		}
	}

	// Values should not contain a features section
	if strings.Contains(result.ValuesYAML, "features:") {
		t.Error("expected no 'features:' section in values.yaml when chart has only Deployment+Service")
	}
}

// ============================================================
// Test 2: Chart with ServiceMonitor — monitoring flag wraps it
// ============================================================

func TestFeatureFlags_ChartWithServiceMonitor_MonitoringFlag(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/servicemonitor.yaml": "apiVersion: monitoring.coreos.com/v1\nkind: ServiceMonitor\nmetadata:\n  name: myapp\nspec:\n  selector:\n    matchLabels:\n      app: myapp",
	})

	config := DefaultFeatureFlagConfig()
	result := InjectFeatureFlags(chart, config)

	content, ok := result.Templates["templates/servicemonitor.yaml"]
	if !ok {
		t.Fatal("templates/servicemonitor.yaml missing from result")
	}

	if !strings.Contains(content, "{{- if .Values.features.monitoring }}") {
		t.Error("ServiceMonitor template must start with '{{- if .Values.features.monitoring }}'")
	}
	if !strings.Contains(content, "{{- end }}") {
		t.Error("ServiceMonitor template must end with '{{- end }}'")
	}

	// Original content must be preserved between the guards
	if !strings.Contains(content, "kind: ServiceMonitor") {
		t.Error("original ServiceMonitor YAML must be preserved inside the guards")
	}
}

// ============================================================
// Test 3: Chart with HPA + VPA — autoscaling flag wraps both
// ============================================================

func TestFeatureFlags_ChartWithHPAAndVPA_AutoscalingFlag(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/hpa.yaml": "apiVersion: autoscaling/v2\nkind: HorizontalPodAutoscaler\nmetadata:\n  name: myapp\nspec:\n  scaleTargetRef:\n    kind: Deployment\n    name: myapp",
		"templates/vpa.yaml": "apiVersion: autoscaling.k8s.io/v1\nkind: VerticalPodAutoscaler\nmetadata:\n  name: myapp\nspec:\n  targetRef:\n    kind: Deployment\n    name: myapp",
	})

	config := DefaultFeatureFlagConfig()
	result := InjectFeatureFlags(chart, config)

	for _, path := range []string{"templates/hpa.yaml", "templates/vpa.yaml"} {
		content, ok := result.Templates[path]
		if !ok {
			t.Fatalf("%s missing from result", path)
		}
		if !strings.Contains(content, "{{- if .Values.features.autoscaling }}") {
			t.Errorf("%s must be wrapped with '{{- if .Values.features.autoscaling }}'", path)
		}
		if !strings.Contains(content, "{{- end }}") {
			t.Errorf("%s must end with '{{- end }}'", path)
		}
	}
}

// ============================================================
// Test 4: Chart with NetworkPolicy + PDB — security flag wraps both
// ============================================================

func TestFeatureFlags_ChartWithNetworkPolicyPDB_SecurityFlag(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/netpol.yaml": "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: myapp\nspec:\n  podSelector:\n    matchLabels:\n      app: myapp",
		"templates/pdb.yaml":    "apiVersion: policy/v1\nkind: PodDisruptionBudget\nmetadata:\n  name: myapp\nspec:\n  minAvailable: 1\n  selector:\n    matchLabels:\n      app: myapp",
	})

	config := DefaultFeatureFlagConfig()
	result := InjectFeatureFlags(chart, config)

	for _, path := range []string{"templates/netpol.yaml", "templates/pdb.yaml"} {
		content, ok := result.Templates[path]
		if !ok {
			t.Fatalf("%s missing from result", path)
		}
		if !strings.Contains(content, "{{- if .Values.features.security }}") {
			t.Errorf("%s must be wrapped with '{{- if .Values.features.security }}'", path)
		}
		if !strings.Contains(content, "{{- end }}") {
			t.Errorf("%s must end with '{{- end }}'", path)
		}
	}
}

// ============================================================
// Test 5: Chart with PVC — storage flag wraps it
// ============================================================

func TestFeatureFlags_ChartWithPVC_StorageFlag(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/pvc.yaml": "apiVersion: v1\nkind: PersistentVolumeClaim\nmetadata:\n  name: myapp-data\nspec:\n  accessModes:\n    - ReadWriteOnce\n  resources:\n    requests:\n      storage: 10Gi",
	})

	config := DefaultFeatureFlagConfig()
	result := InjectFeatureFlags(chart, config)

	content, ok := result.Templates["templates/pvc.yaml"]
	if !ok {
		t.Fatal("templates/pvc.yaml missing from result")
	}
	if !strings.Contains(content, "{{- if .Values.features.storage }}") {
		t.Error("PVC template must be wrapped with '{{- if .Values.features.storage }}'")
	}
	if !strings.Contains(content, "{{- end }}") {
		t.Error("PVC template must end with '{{- end }}'")
	}
	if !strings.Contains(content, "kind: PersistentVolumeClaim") {
		t.Error("original PVC YAML must be preserved inside the guards")
	}
}

// ============================================================
// Test 6: Chart with RBAC resources — rbac flag wraps Role and
// RoleBinding but NOT ServiceAccount (never-wrap kind)
// ============================================================

func TestFeatureFlags_ChartWithRBACResources_RBACFlag(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/role.yaml": "apiVersion: rbac.authorization.k8s.io/v1\nkind: Role\nmetadata:\n  name: myapp\nrules:\n  - apiGroups: [\"\"]\n    resources: [\"pods\"]\n    verbs: [\"get\", \"list\"]",
		"templates/rolebinding.yaml": "apiVersion: rbac.authorization.k8s.io/v1\nkind: RoleBinding\nmetadata:\n  name: myapp\nroleRef:\n  kind: Role\n  name: myapp\nsubjects:\n  - kind: ServiceAccount\n    name: myapp",
		"templates/serviceaccount.yaml": "apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: myapp",
	})

	config := DefaultFeatureFlagConfig()
	result := InjectFeatureFlags(chart, config)

	// Role must be wrapped
	roleContent, ok := result.Templates["templates/role.yaml"]
	if !ok {
		t.Fatal("templates/role.yaml missing from result")
	}
	if !strings.Contains(roleContent, "{{- if .Values.features.rbac }}") {
		t.Error("Role template must be wrapped with '{{- if .Values.features.rbac }}'")
	}

	// RoleBinding must be wrapped
	rbContent, ok := result.Templates["templates/rolebinding.yaml"]
	if !ok {
		t.Fatal("templates/rolebinding.yaml missing from result")
	}
	if !strings.Contains(rbContent, "{{- if .Values.features.rbac }}") {
		t.Error("RoleBinding template must be wrapped with '{{- if .Values.features.rbac }}'")
	}

	// ServiceAccount must NOT be wrapped (it is in the never-wrap list)
	saContent, ok := result.Templates["templates/serviceaccount.yaml"]
	if !ok {
		t.Fatal("templates/serviceaccount.yaml missing from result")
	}
	if strings.Contains(saContent, "{{- if .Values.features.") {
		t.Error("ServiceAccount must NOT be wrapped with feature flags (it is in the never-wrap list)")
	}
}

// ============================================================
// Test 7: All categories present — six flags appear in values
// ============================================================

func TestFeatureFlags_AllCategories_SixFlagsInValues(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/servicemonitor.yaml": "apiVersion: monitoring.coreos.com/v1\nkind: ServiceMonitor\nmetadata:\n  name: myapp",
		"templates/ingress.yaml":        "apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: myapp\nspec:\n  rules: []",
		"templates/hpa.yaml":            "apiVersion: autoscaling/v2\nkind: HorizontalPodAutoscaler\nmetadata:\n  name: myapp",
		"templates/netpol.yaml":         "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: myapp",
		"templates/pvc.yaml":            "apiVersion: v1\nkind: PersistentVolumeClaim\nmetadata:\n  name: myapp-data",
		"templates/role.yaml":           "apiVersion: rbac.authorization.k8s.io/v1\nkind: Role\nmetadata:\n  name: myapp",
	})

	config := DefaultFeatureFlagConfig()
	result := InjectFeatureFlags(chart, config)

	expectedFlags := []string{
		"monitoring",
		"ingress",
		"autoscaling",
		"security",
		"storage",
		"rbac",
	}

	for _, flag := range expectedFlags {
		if !strings.Contains(result.ValuesYAML, flag+":") {
			t.Errorf("expected 'features.%s: true' in values.yaml, but flag '%s:' not found", flag, flag)
		}
	}

	// All six flags should be true by default
	featureCount := strings.Count(result.ValuesYAML, "true")
	if featureCount < 6 {
		t.Errorf("expected at least 6 'true' values in features section, got %d", featureCount)
	}

	// The features section must be present
	if !strings.Contains(result.ValuesYAML, "features:") {
		t.Error("expected 'features:' section in values.yaml")
	}
}

// ============================================================
// Test 8: Custom config overrides default kind→category mapping
// ============================================================

func TestFeatureFlags_CustomConfig_OverridesDefault(t *testing.T) {
	// Map ConfigMap to monitoring (non-default behaviour)
	customConfig := &FeatureFlagConfig{
		Categories: map[FeatureCategory]bool{
			FeatureMonitoring: true,
		},
		KindToCategory: map[string]FeatureCategory{
			"ConfigMap": FeatureMonitoring,
		},
	}

	chart := makeChart("myapp", map[string]string{
		"templates/configmap.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: myapp-config\ndata:\n  key: value",
	})

	result := InjectFeatureFlags(chart, customConfig)

	content, ok := result.Templates["templates/configmap.yaml"]
	if !ok {
		t.Fatal("templates/configmap.yaml missing from result")
	}
	if !strings.Contains(content, "{{- if .Values.features.monitoring }}") {
		t.Error("ConfigMap must be wrapped with '{{- if .Values.features.monitoring }}' when custom config maps it there")
	}
	if !strings.Contains(content, "{{- end }}") {
		t.Error("wrapped ConfigMap template must end with '{{- end }}'")
	}
}

// ============================================================
// Test 9: Empty chart (no templates) — no-op
// ============================================================

func TestFeatureFlags_EmptyChart_NoOp(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:      "empty",
		ChartYAML: "apiVersion: v2\nname: empty\nversion: 0.1.0\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates: map[string]string{},
	}

	config := DefaultFeatureFlagConfig()
	result := InjectFeatureFlags(chart, config)

	if len(result.Templates) != 0 {
		t.Errorf("expected 0 templates for empty chart, got %d", len(result.Templates))
	}

	if strings.Contains(result.ValuesYAML, "features:") {
		t.Error("expected no 'features:' section in values.yaml for empty chart")
	}
}

// ============================================================
// Test 10: Wrapping preserves original multi-line YAML content
// ============================================================

func TestFeatureFlags_WrapPreservesYAML(t *testing.T) {
	originalContent := "apiVersion: monitoring.coreos.com/v1\nkind: ServiceMonitor\nmetadata:\n  name: myapp\n  labels:\n    app: myapp\nspec:\n  selector:\n    matchLabels:\n      app: myapp\n  endpoints:\n    - port: http\n      interval: 30s"

	chart := makeChart("myapp", map[string]string{
		"templates/servicemonitor.yaml": originalContent,
	})

	config := DefaultFeatureFlagConfig()
	result := InjectFeatureFlags(chart, config)

	wrapped, ok := result.Templates["templates/servicemonitor.yaml"]
	if !ok {
		t.Fatal("templates/servicemonitor.yaml missing from result")
	}

	// Every line of the original content must appear in the wrapped output
	for _, line := range strings.Split(originalContent, "\n") {
		if line == "" {
			continue
		}
		if !strings.Contains(wrapped, line) {
			t.Errorf("original line %q not found in wrapped template", line)
		}
	}

	// Guard must appear before and after the original content
	guardIdx := strings.Index(wrapped, "{{- if .Values.features.monitoring }}")
	endIdx := strings.Index(wrapped, "{{- end }}")
	contentIdx := strings.Index(wrapped, "kind: ServiceMonitor")

	if guardIdx == -1 {
		t.Fatal("opening guard '{{- if .Values.features.monitoring }}' not found")
	}
	if endIdx == -1 {
		t.Fatal("closing guard '{{- end }}' not found")
	}
	if contentIdx == -1 {
		t.Fatal("'kind: ServiceMonitor' not found in wrapped output")
	}

	if guardIdx >= contentIdx {
		t.Error("opening guard must appear before the original content")
	}
	if endIdx <= contentIdx {
		t.Error("closing '{{- end }}' must appear after the original content")
	}
}

// ============================================================
// Test 11: DefaultFeatureFlagConfig has all expected kind→category mappings
// ============================================================

func TestFeatureFlags_DefaultConfig_HasAllMappings(t *testing.T) {
	config := DefaultFeatureFlagConfig()

	if config == nil {
		t.Fatal("DefaultFeatureFlagConfig() must not return nil")
	}
	if config.KindToCategory == nil {
		t.Fatal("KindToCategory map must not be nil")
	}

	expectedMappings := map[string]FeatureCategory{
		// monitoring
		"ServiceMonitor":    FeatureMonitoring,
		"PodMonitor":        FeatureMonitoring,
		"PrometheusRule":    FeatureMonitoring,
		"GrafanaDashboard":  FeatureMonitoring,
		// ingress
		"Ingress":    FeatureIngress,
		"HTTPRoute":  FeatureIngress,
		"Gateway":    FeatureIngress,
		"GRPCRoute":  FeatureIngress,
		// autoscaling
		"HorizontalPodAutoscaler": FeatureAutoscaling,
		"VerticalPodAutoscaler":   FeatureAutoscaling,
		"ScaledObject":            FeatureAutoscaling,
		"TriggerAuthentication":   FeatureAutoscaling,
		// security
		"NetworkPolicy":        FeatureSecurity,
		"PodDisruptionBudget":  FeatureSecurity,
		// storage
		"PersistentVolumeClaim": FeatureStorage,
		// rbac
		"Role":               FeatureRBAC,
		"ClusterRole":        FeatureRBAC,
		"RoleBinding":        FeatureRBAC,
		"ClusterRoleBinding": FeatureRBAC,
	}

	for kind, wantCategory := range expectedMappings {
		gotCategory, ok := config.KindToCategory[kind]
		if !ok {
			t.Errorf("DefaultFeatureFlagConfig missing mapping for kind %q", kind)
			continue
		}
		if gotCategory != wantCategory {
			t.Errorf("kind %q: expected category %q, got %q", kind, wantCategory, gotCategory)
		}
	}

	// Verify all six categories are present in the Categories map
	for _, cat := range []FeatureCategory{
		FeatureMonitoring, FeatureIngress, FeatureAutoscaling,
		FeatureSecurity, FeatureStorage, FeatureRBAC,
	} {
		if _, ok := config.Categories[cat]; !ok {
			t.Errorf("Categories map missing entry for category %q", cat)
		}
	}
}

// ============================================================
// Test 12: generateFeatureValues returns all categories as true by default
// ============================================================

func TestFeatureFlags_GenerateFeatureValues_DefaultTrue(t *testing.T) {
	config := DefaultFeatureFlagConfig()
	vals := generateFeatureValues(config)

	if vals == nil {
		t.Fatal("generateFeatureValues must not return nil")
	}

	expectedCategories := []FeatureCategory{
		FeatureMonitoring,
		FeatureIngress,
		FeatureAutoscaling,
		FeatureSecurity,
		FeatureStorage,
		FeatureRBAC,
	}

	for _, cat := range expectedCategories {
		v, ok := vals[string(cat)]
		if !ok {
			t.Errorf("generateFeatureValues missing entry for category %q", cat)
			continue
		}
		boolVal, ok := v.(bool)
		if !ok {
			t.Errorf("category %q value must be bool, got %T", cat, v)
			continue
		}
		if !boolVal {
			t.Errorf("category %q must default to true, got false", cat)
		}
	}

	if len(vals) != len(expectedCategories) {
		t.Errorf("expected %d entries in feature values, got %d", len(expectedCategories), len(vals))
	}
}
