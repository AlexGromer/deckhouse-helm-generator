package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// GrafanaDashboardProcessor processes ConfigMaps with label grafana_dashboard: "1".
// These are Grafana sidecar-provisioned dashboards.
// Priority is higher than ConfigMapProcessor to intercept dashboard ConfigMaps.
type GrafanaDashboardProcessor struct {
	processor.BaseProcessor
}

// NewGrafanaDashboardProcessor creates a new GrafanaDashboard processor.
func NewGrafanaDashboardProcessor() *GrafanaDashboardProcessor {
	return &GrafanaDashboardProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"grafanadashboard",
			110, // Higher than ConfigMapProcessor (80) to intercept first
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
		),
	}
}

// Process processes a ConfigMap if it has the grafana_dashboard label.
// Returns nil result (Processed=false) for non-dashboard ConfigMaps.
func (p *GrafanaDashboardProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ConfigMap object is nil")
	}

	// Only handle ConfigMaps with grafana_dashboard label
	if !isGrafanaDashboard(obj) {
		return &processor.Result{Processed: false}, nil
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values := p.extractValues(obj)
	template := p.generateTemplate(ctx, serviceName, name)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/grafana-dashboard-%s.yaml", name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.grafanaDashboard", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"type":      "grafana_dashboard",
		},
	}, nil
}

func isGrafanaDashboard(obj *unstructured.Unstructured) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	return labels["grafana_dashboard"] == "1"
}

func (p *GrafanaDashboardProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Extract dashboard JSON data
	if data, ok, _ := unstructured.NestedMap(obj.Object, "data"); ok && len(data) > 0 {
		values["dashboards"] = data
	}

	return values
}

func (p *GrafanaDashboardProcessor) generateTemplate(ctx processor.Context, serviceName, name string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.grafanaDashboard }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    grafana_dashboard: "1"
data:
  {{- range $key, $value := .dashboards }}
  {{ $key }}: |
    {{ $value | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, sanitized, name, ctx.ChartName)
}
