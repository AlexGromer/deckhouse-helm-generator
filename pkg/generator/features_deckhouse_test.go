package generator

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/compliance/pss"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestApplyDeckhouseFeature_InvalidProfile(t *testing.T) {
	chart := &types.GeneratedChart{}
	fc := FeatureContext{Params: map[string]string{"pss-profile": "invalid"}}

	_, err := applyDeckhouseFeature(chart, fc)
	if err == nil || !strings.Contains(err.Error(), "pss-profile must be baseline or restricted") {
		t.Errorf("expected error about invalid profile, got %v", err)
	}
}

func TestBuildDeckhouseReport_Empty(t *testing.T) {
	chart := &types.GeneratedChart{}
	fc := FeatureContext{Graph: types.NewResourceGraph()}

	report, err := buildDeckhouseReport(chart, fc, pss.ProfileRestricted)
	if err != nil {
		t.Fatal(err)
	}

	md := report.Markdown()

	if !strings.Contains(md, "No findings.") {
		t.Errorf("Expected 'No findings.' in empty report, got:\n%s", md)
	}
}

func TestBuildDeckhouseReport_WorkloadsAndMonitoring(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment",
		},
	}

	graph := types.NewResourceGraph()
	if graph.Resources == nil {
		graph.Resources = make(map[types.ResourceKey]*types.ProcessedResource)
	}

	// Add a Deployment that violates restricted profile
	depObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "orders",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "app",
							},
						},
					},
				},
			},
		},
	}

	depRes := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: depObj,
			GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		TemplatePath: "templates/deployment.yaml",
	}

	graph.Resources[depRes.Original.ResourceKey()] = depRes

	// Add a ServiceMonitor missing prometheus label
	smObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "monitoring.coreos.com/v1",
			"kind":       "ServiceMonitor",
			"metadata": map[string]interface{}{
				"name": "orders",
			},
		},
	}

	smRes := &types.ProcessedResource{Original: &types.ExtractedResource{Object: smObj, GVK: schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "ServiceMonitor"}}}
	graph.Resources[smRes.Original.ResourceKey()] = smRes

	fc := FeatureContext{Graph: graph}

	report, err := buildDeckhouseReport(chart, fc, pss.ProfileRestricted)
	if err != nil {
		t.Fatal(err)
	}

	md := report.Markdown()

	if !strings.Contains(md, "security.deckhouse.io/pod-policy: baseline") && !strings.Contains(md, "security.deckhouse.io/pod-policy: privileged") {
		t.Errorf("Expected pod-policy label in output, got:\n%s", md)
	}

	if !strings.Contains(md, "Deployment/orders") {
		t.Errorf("Expected Deployment/orders in output, got:\n%s", md)
	}

	if !strings.Contains(md, "monitor-watcher-enabled") {
		t.Errorf("Expected monitor-watcher-enabled in output, got:\n%s", md)
	}

	if !strings.Contains(md, "missing 'prometheus: main'") {
		t.Errorf("Expected ServiceMonitor warning in output, got:\n%s", md)
	}
}

func TestBuildDeckhouseReport_GrafanaConfigMap(t *testing.T) {
	chart := &types.GeneratedChart{}

	graph := types.NewResourceGraph()
	if graph.Resources == nil {
		graph.Resources = make(map[types.ResourceKey]*types.ProcessedResource)
	}

	cmObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "dashboard",
				"labels": map[string]interface{}{
					"grafana_dashboard": "1",
				},
			},
		},
	}

	cmRes := &types.ProcessedResource{Original: &types.ExtractedResource{Object: cmObj, GVK: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}}}
	graph.Resources[cmRes.Original.ResourceKey()] = cmRes

	fc := FeatureContext{Graph: graph}

	report, err := buildDeckhouseReport(chart, fc, pss.ProfileBaseline)
	if err != nil {
		t.Fatal(err)
	}

	md := report.Markdown()

	if !strings.Contains(md, "ConfigMap/dashboard") || !strings.Contains(md, "Grafana sidecar convention") {
		t.Errorf("Expected ConfigMap dashboard info in output, got:\n%s", md)
	}
}
