package generator

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestDeckhouseDashboardsFeature(t *testing.T) {
	// 1. ConfigMap source
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
			"data": map[string]interface{}{
				"db.json":  `{"title":"DB Dashboard"}`,
				"app.json": `{"title":"App Dashboard"}`,
			},
		},
	}

	// 2. GrafanaDashboardDefinition source
	gddObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1alpha1",
			"kind":       "GrafanaDashboardDefinition",
			"metadata": map[string]interface{}{
				"name": "legacy-dash",
			},
			"spec": map[string]interface{}{
				"folder":     "Legacy",
				"definition": `{"title":"Legacy Dashboard"}`,
			},
		},
	}

	// 3. Invalid JSON source
	invalidObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "invalid-dashboard",
				"labels": map[string]interface{}{
					"grafana_dashboard": "1",
				},
			},
			"data": map[string]interface{}{
				"invalid.json": `this is not a json`,
			},
		},
	}

	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/dashboard.yaml": "kind: ConfigMap\nmetadata:\n  name: dashboard",
			"templates/legacy.yaml":    "kind: GrafanaDashboardDefinition\nmetadata:\n  name: legacy-dash",
			"templates/invalid.yaml":   "kind: ConfigMap\nmetadata:\n  name: invalid-dashboard",
		},
	}

	graph := types.NewResourceGraph()
	if graph.Resources == nil {
		graph.Resources = make(map[types.ResourceKey]*types.ProcessedResource)
	}

	for _, obj := range []*unstructured.Unstructured{cmObj, gddObj, invalidObj} {
		res := &types.ProcessedResource{
			Original: &types.ExtractedResource{
				Object: obj,
				GVK:    obj.GroupVersionKind(),
			},
			TemplatePath: "templates/" + strings.ToLower(strings.Split(obj.GetName(), "-")[0]) + ".yaml",
		}
		graph.Resources[res.Original.ResourceKey()] = res
	}

	// Apply feature with category parameter
	out, err := ApplyFeatures([]*types.GeneratedChart{chart}, []string{"deckhouse-dashboards"}, map[string]map[string]string{
		"deckhouse-dashboards": {
			"category": "Main",
		},
	}, graph)
	if err != nil {
		t.Fatal(err)
	}

	c := out[0]

	// Check values
	values := valuesOf(t, c)
	dashValuesRaw := values["deckhouseDashboards"]
	if dashValuesRaw == nil {
		t.Fatal("deckhouseDashboards missing in values")
	}

	dashValues := dashValuesRaw.(map[string]interface{})
	if dashValues["enabled"] != true {
		t.Errorf("deckhouseDashboards.enabled expected to be true")
	}

	dashboardsRaw := dashValues["dashboards"].(map[string]interface{})
	if len(dashboardsRaw) != 4 { // dashboard-db, dashboard-app, legacy-dash, invalid-dashboard-invalid
		t.Errorf("expected 4 dashboards, got %d", len(dashboardsRaw))
	}

	appDash := dashboardsRaw["dashboard-app"].(map[string]interface{})
	if appDash["category"] != "Main" || appDash["title"] != "App Dashboard" || appDash["definition"] != `{"title":"App Dashboard"}` {
		t.Errorf("dashboard-app values incorrect: %v", appDash)
	}

	legacyDash := dashboardsRaw["legacy-dash"].(map[string]interface{})
	if legacyDash["category"] != "Legacy" || legacyDash["title"] != "Legacy Dashboard" {
		t.Errorf("legacy-dash values incorrect: %v", legacyDash)
	}

	invalidDash := dashboardsRaw["invalid-dashboard-invalid"].(map[string]interface{})
	if invalidDash["title"] != nil {
		t.Errorf("invalid-dashboard-invalid title should be absent, got %v", invalidDash["title"])
	}

	// Check template contents
	tmpl := c.Templates["templates/deckhouse-dashboards.yaml"]
	if !strings.Contains(tmpl, "kind: ObservabilityDashboard") {
		t.Errorf("templates/deckhouse-dashboards.yaml does not contain ObservabilityDashboard")
	}

	// The legacy template should not be replaced by default
	if strings.Contains(c.Templates["templates/legacy.yaml"], "{{- if not .Values.deckhouseDashboards.enabled }}") {
		t.Errorf("legacy template should not be wrapped by default")
	}
}

func TestDeckhouseDashboardsFeature_ReplaceLegacy(t *testing.T) {
	gddObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1alpha1",
			"kind":       "GrafanaDashboardDefinition",
			"metadata": map[string]interface{}{
				"name": "legacy-dash",
			},
			"spec": map[string]interface{}{
				"definition": `{"title":"Legacy Dashboard"}`,
			},
		},
	}

	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/legacy.yaml": "kind: GrafanaDashboardDefinition\nmetadata:\n  name: legacy-dash",
		},
	}

	graph := types.NewResourceGraph()
	if graph.Resources == nil {
		graph.Resources = make(map[types.ResourceKey]*types.ProcessedResource)
	}

	res := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: gddObj,
			GVK:    gddObj.GroupVersionKind(),
		},
		TemplatePath: "templates/legacy.yaml",
	}
	graph.Resources[res.Original.ResourceKey()] = res

	// Apply feature with replace-legacy=true
	out, err := ApplyFeatures([]*types.GeneratedChart{chart}, []string{"deckhouse-dashboards"}, map[string]map[string]string{
		"deckhouse-dashboards": {
			"replace-legacy": "true",
		},
	}, graph)
	if err != nil {
		t.Fatal(err)
	}

	c := out[0]

	// The legacy template should be wrapped
	legacyTmpl := c.Templates["templates/legacy.yaml"]
	if !strings.Contains(legacyTmpl, "{{- if not .Values.deckhouseDashboards.enabled }}") {
		t.Errorf("legacy template should be wrapped when replace-legacy=true. Got: %s", legacyTmpl)
	}
}
