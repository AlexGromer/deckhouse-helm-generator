package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// IngressProcessor processes Kubernetes Ingress resources.
type IngressProcessor struct {
	processor.BaseProcessor
}

// NewIngressProcessor creates a new Ingress processor.
func NewIngressProcessor() *IngressProcessor {
	return &IngressProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"ingress",
			100,
			schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
		),
	}
}

// Process processes an Ingress resource.
func (p *IngressProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("ingress object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	// Extract values from the ingress
	values, deps := p.extractValues(obj)

	// Generate template
	template := p.generateTemplate(ctx, obj, serviceName)

	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}

	// Detect ExternalDNS annotations
	if edns := detectExternalDNS(obj); edns != nil {
		metadata["external_dns"] = edns
	}

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-ingress.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.ingress", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata:        metadata,
	}, nil
}

func (p *IngressProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	values["enabled"] = true

	// IngressClassName
	if className, found, _ := unstructured.NestedString(obj.Object, "spec", "ingressClassName"); found {
		values["className"] = className
	}

	// Annotations (important for ingress controllers)
	if annotations := obj.GetAnnotations(); len(annotations) > 0 {
		values["annotations"] = annotations

		// Check for cert-manager annotations
		if issuer, ok := annotations["cert-manager.io/cluster-issuer"]; ok {
			deps = append(deps, types.ResourceKey{
				GVK:  schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "ClusterIssuer"},
				Name: issuer,
			})
		}
		if issuer, ok := annotations["cert-manager.io/issuer"]; ok {
			deps = append(deps, types.ResourceKey{
				GVK:       schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "Issuer"},
				Namespace: obj.GetNamespace(),
				Name:      issuer,
			})
		}
	}

	// TLS
	if tls, found, _ := unstructured.NestedSlice(obj.Object, "spec", "tls"); found {
		tlsValues := make([]map[string]interface{}, 0, len(tls))
		for _, t := range tls {
			tlsEntry, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			tv := make(map[string]interface{})

			if hosts, ok := tlsEntry["hosts"].([]interface{}); ok {
				hostStrings := make([]string, 0, len(hosts))
				for _, h := range hosts {
					if hs, ok := h.(string); ok {
						hostStrings = append(hostStrings, hs)
					}
				}
				tv["hosts"] = hostStrings
			}

			if secretName, ok := tlsEntry["secretName"].(string); ok {
				tv["secretName"] = secretName
				// Add dependency on TLS secret
				deps = append(deps, types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
					Namespace: obj.GetNamespace(),
					Name:      secretName,
				})
			}

			tlsValues = append(tlsValues, tv)
		}
		values["tls"] = tlsValues
	}

	// Default backend
	if defaultBackend, found, _ := unstructured.NestedMap(obj.Object, "spec", "defaultBackend"); found {
		dbValues := make(map[string]interface{})
		if service, ok := defaultBackend["service"].(map[string]interface{}); ok {
			svcBackend := make(map[string]interface{})
			if svcName, ok := service["name"].(string); ok {
				svcBackend["name"] = svcName
				deps = append(deps, types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Service"},
					Namespace: obj.GetNamespace(),
					Name:      svcName,
				})
			}
			if port, ok := service["port"].(map[string]interface{}); ok {
				if number, ok := port["number"].(int64); ok {
					svcBackend["port"] = number
				}
				if name, ok := port["name"].(string); ok {
					svcBackend["portName"] = name
				}
			}
			dbValues["service"] = svcBackend
		}
		values["defaultBackend"] = dbValues
	}

	// Rules
	if rules, found, _ := unstructured.NestedSlice(obj.Object, "spec", "rules"); found {
		hosts := make([]map[string]interface{}, 0, len(rules))
		for _, r := range rules {
			rule, ok := r.(map[string]interface{})
			if !ok {
				continue
			}

			hostEntry := make(map[string]interface{})

			if host, ok := rule["host"].(string); ok {
				hostEntry["host"] = host
			}

			// HTTP rules
			if http, ok := rule["http"].(map[string]interface{}); ok {
				if paths, ok := http["paths"].([]interface{}); ok {
					pathEntries := make([]map[string]interface{}, 0, len(paths))
					for _, pathItem := range paths {
						pathEntry, ok := pathItem.(map[string]interface{})
						if !ok {
							continue
						}

						pe := make(map[string]interface{})

						if path, ok := pathEntry["path"].(string); ok {
							pe["path"] = path
						}
						if pathType, ok := pathEntry["pathType"].(string); ok {
							pe["pathType"] = pathType
						}

						// Backend
						if backend, ok := pathEntry["backend"].(map[string]interface{}); ok {
							if service, ok := backend["service"].(map[string]interface{}); ok {
								svcBackend := make(map[string]interface{})
								if svcName, ok := service["name"].(string); ok {
									svcBackend["name"] = svcName
									// Add dependency on service
									deps = append(deps, types.ResourceKey{
										GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Service"},
										Namespace: obj.GetNamespace(),
										Name:      svcName,
									})
								}
								if port, ok := service["port"].(map[string]interface{}); ok {
									if number, ok := port["number"].(int64); ok {
										svcBackend["port"] = number
									}
									if name, ok := port["name"].(string); ok {
										svcBackend["portName"] = name
									}
								}
								pe["service"] = svcBackend
							}
						}

						pathEntries = append(pathEntries, pe)
					}
					hostEntry["paths"] = pathEntries
				}
			}

			hosts = append(hosts, hostEntry)
		}
		values["rules"] = hosts
		values["hosts"] = hosts // backward-compatible alias used by template
	}

	return values, deps
}

func (p *IngressProcessor) generateTemplate(ctx processor.Context, obj *unstructured.Unstructured, serviceName string) string {
	fullnameHelper := fmt.Sprintf("{{ include \"%s.fullname\" $ }}", ctx.ChartName)

	template := fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.ingress }}
{{- if .enabled }}
apiVersion: networking.k8s.io/v1
kind: Ingress
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
  {{- with .className }}
  ingressClassName: {{ . }}
  {{- end }}
  {{- if .tls }}
  tls:
    {{- range .tls }}
    - hosts:
        {{- range .hosts }}
        - {{ . | quote }}
        {{- end }}
      secretName: {{ .secretName }}
    {{- end }}
  {{- end }}
  rules:
    {{- range .rules }}
    - host: {{ .host | quote }}
      http:
        paths:
          {{- range .paths }}
          - path: {{ .path }}
            pathType: {{ .pathType | default "Prefix" }}
            backend:
              service:
                name: {{ .service.name }}
                port:
                  {{- if .service.portName }}
                  name: {{ .service.portName }}
                  {{- else }}
                  number: {{ .service.port }}
                  {{- end }}
          {{- end }}
    {{- end }}
{{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName,
		ctx.ChartName, serviceName)

	return template
}
