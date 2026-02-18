package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ServiceProcessor processes Kubernetes Services.
type ServiceProcessor struct {
	processor.BaseProcessor
}

// NewServiceProcessor creates a new Service processor.
func NewServiceProcessor() *ServiceProcessor {
	return &ServiceProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"service",
			100,
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
		),
	}
}

// Process processes a Service resource.
func (p *ServiceProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot process nil Service")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	// Extract values from the service
	values, deps := p.extractValues(obj)

	// Generate template
	template := p.generateTemplate(ctx, obj, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-service.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.service", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *ServiceProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	// Type
	if svcType, found, _ := unstructured.NestedString(obj.Object, "spec", "type"); found {
		values["type"] = svcType
	} else {
		values["type"] = "ClusterIP"
	}

	// Ports
	if ports, found, _ := unstructured.NestedSlice(obj.Object, "spec", "ports"); found {
		portValues := make([]map[string]interface{}, 0, len(ports))
		for _, p := range ports {
			port, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			pv := make(map[string]interface{})

			if name, ok := port["name"].(string); ok {
				pv["name"] = name
			}
			if portNum, ok := toInt64(port["port"]); ok {
				pv["port"] = portNum
			}
			if targetPort := port["targetPort"]; targetPort != nil {
				// targetPort can be int or string (named port)
				if tp, ok := toInt64(targetPort); ok {
					pv["targetPort"] = tp
				} else {
					pv["targetPort"] = targetPort
				}
			}
			if protocol, ok := port["protocol"].(string); ok {
				pv["protocol"] = protocol
			}
			if nodePort, ok := toInt64(port["nodePort"]); ok {
				pv["nodePort"] = nodePort
			}

			portValues = append(portValues, pv)
		}
		values["ports"] = portValues
	}

	// Selector
	if selector, found, _ := unstructured.NestedStringMap(obj.Object, "spec", "selector"); found {
		values["selector"] = selector

		// Try to detect related Deployment/StatefulSet based on selector
		// This is a simplified heuristic - full detection happens in the analyzer
		if app, ok := selector["app"]; ok {
			deps = append(deps, types.ResourceKey{
				GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
				Namespace: obj.GetNamespace(),
				Name:      app,
			})
		}
		if appName, ok := selector["app.kubernetes.io/name"]; ok {
			deps = append(deps, types.ResourceKey{
				GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
				Namespace: obj.GetNamespace(),
				Name:      appName,
			})
		}
	}

	// ClusterIP
	if clusterIP, found, _ := unstructured.NestedString(obj.Object, "spec", "clusterIP"); found {
		if clusterIP != "" && clusterIP != "None" {
			values["clusterIP"] = clusterIP
		} else if clusterIP == "None" {
			values["clusterIP"] = "None"
		}
	}

	// External traffic policy
	if policy, found, _ := unstructured.NestedString(obj.Object, "spec", "externalTrafficPolicy"); found {
		values["externalTrafficPolicy"] = policy
	}

	// Session affinity
	if affinity, found, _ := unstructured.NestedString(obj.Object, "spec", "sessionAffinity"); found {
		values["sessionAffinity"] = affinity
	}

	// Load balancer settings
	if lbIP, found, _ := unstructured.NestedString(obj.Object, "spec", "loadBalancerIP"); found {
		values["loadBalancerIP"] = lbIP
	}
	if lbSourceRanges, found, _ := unstructured.NestedStringSlice(obj.Object, "spec", "loadBalancerSourceRanges"); found {
		values["loadBalancerSourceRanges"] = lbSourceRanges
	}

	// External IPs
	if externalIPs, found, _ := unstructured.NestedStringSlice(obj.Object, "spec", "externalIPs"); found {
		values["externalIPs"] = externalIPs
	}

	// ExternalName (for ExternalName type services)
	if externalName, found, _ := unstructured.NestedString(obj.Object, "spec", "externalName"); found {
		values["externalName"] = externalName
	}

	// Health check node port (used with LoadBalancer + externalTrafficPolicy: Local)
	if healthCheckNodePort, found, _ := unstructured.NestedInt64(obj.Object, "spec", "healthCheckNodePort"); found {
		values["healthCheckNodePort"] = healthCheckNodePort
	}

	// Annotations (useful for cloud provider settings)
	if annotations := obj.GetAnnotations(); len(annotations) > 0 {
		values["annotations"] = annotations
	}

	return values, deps
}

// toInt64 converts numeric types (int64, float64, int, int32) to int64.
// YAML/JSON parsers may produce different numeric types depending on the source.
func toInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	default:
		return 0, false
	}
}

func (p *ServiceProcessor) generateTemplate(ctx processor.Context, obj *unstructured.Unstructured, serviceName string) string {
	fullnameHelper := fmt.Sprintf("{{ include \"%s.fullname\" $ }}", ctx.ChartName)

	template := fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.service }}
apiVersion: v1
kind: Service
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
  {{- with .annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  type: {{ .type | default "ClusterIP" }}
  {{- if and (eq .type "ClusterIP") .clusterIP }}
  clusterIP: {{ .clusterIP }}
  {{- end }}
  {{- if eq .type "LoadBalancer" }}
  {{- with .loadBalancerIP }}
  loadBalancerIP: {{ . }}
  {{- end }}
  {{- with .loadBalancerSourceRanges }}
  loadBalancerSourceRanges:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- end }}
  {{- with .externalIPs }}
  externalIPs:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .externalTrafficPolicy }}
  externalTrafficPolicy: {{ . }}
  {{- end }}
  {{- with .sessionAffinity }}
  sessionAffinity: {{ . }}
  {{- end }}
  ports:
    {{- range .ports }}
    - name: {{ .name | default "http" }}
      port: {{ .port }}
      targetPort: {{ .targetPort | default .port }}
      protocol: {{ .protocol | default "TCP" }}
      {{- if and (eq ($.Values.services.%s.service.type | default "ClusterIP") "NodePort") .nodePort }}
      nodePort: {{ .nodePort }}
      {{- end }}
    {{- end }}
  selector:
    {{- include "%s.selectorLabels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName,
		ctx.ChartName, serviceName,
		serviceName,
		ctx.ChartName, serviceName)

	return template
}
