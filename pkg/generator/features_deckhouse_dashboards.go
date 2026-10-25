package generator

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func init() {
	RegisterFeature(Feature{
		Name:        "deckhouse-dashboards",
		Description: "Convert ConfigMap/GrafanaDashboardDefinition to ObservabilityDashboard objects",
		Params: map[string]string{
			"category":       "",
			"replace-legacy": "false",
		},
		Apply: applyDeckhouseDashboardsFeature,
	})
}

func applyDeckhouseDashboardsFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	categoryParam := fc.Param("category")
	replaceLegacy := fc.Param("replace-legacy") == "true"

	resources := opsChartResources(chart, fc.Graph)

	type dashData struct {
		category   string
		title      string
		definition string
	}

	dashboards := make(map[string]dashData)
	var legacyTemplates []string

	for _, r := range resources {
		obj := r.Original.Object
		kind := obj.GetKind()

		if kind == "ConfigMap" {
			labels := obj.GetLabels()
			if labels != nil && labels["grafana_dashboard"] == "1" {
				data, _, _ := unstructured.NestedStringMap(obj.Object, "data")

				// sort keys for deterministic order, though map insertion order won't matter if we sort later,
				// but here we just populate a map. We'll sort when creating template/values anyway.
				for k, v := range data {
					name := obj.GetName() + "-" + strings.TrimSuffix(k, ".json")
					name = strings.ReplaceAll(name, ".", "-")
					name = strings.ReplaceAll(name, "_", "-")

					title := ""
					var parsed map[string]interface{}
					if err := json.Unmarshal([]byte(v), &parsed); err == nil {
						if t, ok := parsed["title"].(string); ok {
							title = t
						}
					}

					dashboards[name] = dashData{
						category:   categoryParam,
						title:      title,
						definition: v,
					}
				}
			}
		} else if kind == "GrafanaDashboardDefinition" {
			if replaceLegacy {
				legacyTemplates = append(legacyTemplates, r.TemplatePath)
			}

			defStr, _, _ := unstructured.NestedString(obj.Object, "spec", "definition")
			if defStr == "" {
				continue
			}

			folder, _, _ := unstructured.NestedString(obj.Object, "spec", "folder")
			cat := folder
			if cat == "" {
				cat = categoryParam
			}

			title := ""
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(defStr), &parsed); err == nil {
				if t, ok := parsed["title"].(string); ok {
					title = t
				}
			}

			dashboards[obj.GetName()] = dashData{
				category:   cat,
				title:      title,
				definition: defStr,
			}
		}
	}

	if len(dashboards) == 0 {
		return chart, nil
	}

	out := cloneChart(chart)

	// Create values
	dashMap := make(map[string]interface{})
	for k, v := range dashboards {
		m := map[string]interface{}{
			"definition": v.definition,
		}
		if v.category != "" {
			m["category"] = v.category
		}
		if v.title != "" {
			m["title"] = v.title
		}
		dashMap[k] = m
	}

	if err := addFeatureValues(out, "deckhouseDashboards", map[string]interface{}{
		"enabled":    true,
		"dashboards": dashMap,
	}); err != nil {
		return nil, err
	}

	// Create template
	tmpl := `{{- if .Values.deckhouseDashboards.enabled }}
{{- range $name, $d := .Values.deckhouseDashboards.dashboards }}
---
apiVersion: observability.deckhouse.io/v1alpha1
kind: ObservabilityDashboard
metadata:
  name: {{ $name }}
  namespace: {{ $.Release.Namespace }}
{{- if or $d.category $d.title }}
  annotations:
{{- if $d.category }}
    metadata.deckhouse.io/category: {{ $d.category | quote }}
{{- end }}
{{- if $d.title }}
    metadata.deckhouse.io/title: {{ $d.title | quote }}
{{- end }}
{{- end }}
spec:
  definition: {{ $d.definition | toJson }}
{{- end }}
{{- end }}
`
	if err := addTemplate(out, "templates/deckhouse-dashboards.yaml", tmpl); err != nil {
		return nil, err
	}

	// Replace legacy templates
	for _, tpath := range legacyTemplates {
		content := out.Templates[tpath]
		// To avoid double wrapping if somehow it is already wrapped
		if !strings.Contains(content, "{{- if not .Values.deckhouseDashboards.enabled }}") {
			wrapped := fmt.Sprintf("{{- if not .Values.deckhouseDashboards.enabled }}\n%s\n{{- end }}", strings.TrimSuffix(content, "\n"))
			out.Templates[tpath] = wrapped
		}
	}

	return out, nil
}
