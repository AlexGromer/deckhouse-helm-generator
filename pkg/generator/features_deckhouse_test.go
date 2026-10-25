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

func TestApplyPSSRestrictedFeature(t *testing.T) {
	// 1. Chart without workloads - should return unchanged
	chartNoWorkloads := &types.GeneratedChart{
		ValuesYAML: "foo: bar\n",
	}
	graphNoWorkloads := &types.ResourceGraph{}

	out, err := applyPSSRestrictedFeature(chartNoWorkloads, FeatureContext{Graph: graphNoWorkloads})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ValuesYAML != chartNoWorkloads.ValuesYAML {
		t.Error("chart without workloads was modified")
	}

	// 2. Chart with workload
	chartWithWorkloads := &types.GeneratedChart{
		ValuesYAML: "foo: bar\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "some content",
		},
	}

	// Create a dummy workload resource
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "test-app",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{"name": "c1"},
						},
					},
				},
			},
		},
	}
	graphWithWorkloads := &types.ResourceGraph{
		Resources: map[types.ResourceKey]*types.ProcessedResource{
			{GVK: deployment.GroupVersionKind(), Name: deployment.GetName()}: {
				Original: &types.ExtractedResource{
					Object: deployment,
				},
				ServiceName:  "testApp",
				TemplatePath: "templates/deployment.yaml",
			},
		},
	}

	out2, err := applyPSSRestrictedFeature(chartWithWorkloads, FeatureContext{Graph: graphWithWorkloads})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out2.ValuesYAML, "securityContextDefaults:") {
		t.Error("ValuesYAML missing securityContextDefaults")
	}
	if !strings.Contains(out2.ValuesYAML, "allowPrivilegeEscalation: false") {
		t.Error("ValuesYAML missing container defaults")
	}
	if !strings.Contains(out2.ValuesYAML, "RuntimeDefault") {
		t.Error("ValuesYAML missing pod defaults")
	}

	// 3. Repeat application on the modified chart should error
	_, err = applyPSSRestrictedFeature(out2, FeatureContext{Graph: graphWithWorkloads})
	if err == nil {
		t.Error("expected error when global.securityContextDefaults already exists, got nil")
	} else if !strings.Contains(err.Error(), "pss-restricted: global.securityContextDefaults is already set") {
		t.Errorf("expected specific already set error, got: %v", err)
	}
}
