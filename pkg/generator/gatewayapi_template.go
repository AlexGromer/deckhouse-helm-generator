package generator

import (
	"fmt"
	"strings"
)

// In complete implementation, this generates the required Gateway API templates.
// 1. Gateway
// 2. HTTPRoute
// 3. ListenerSet
// 4. TLSRoute
// 5. ReferenceGrant

func gatewayAPITemplate(rt *resourceTemplate, ingressName string, t ingressTranslation) string {
	var b []string

	b = append(b, "{{- if and $.Values.gatewayApi $.Values.gatewayApi.enabled }}")

	// Gateway
	if t.Gateway != nil {
		gw := t.Gateway
		name, _ := gw["name"].(string)
		if name == "" {
			name = ingressName + "-gateway"
		}
		b = append(b, "apiVersion: gateway.networking.k8s.io/v1")
		b = append(b, "kind: Gateway")
		b = append(b, "metadata:")
		b = append(b, fmt.Sprintf("  name: %s", name))
		b = append(b, "  namespace: {{ $.Release.Namespace }}")
		b = append(b, "spec:")
		if spec, ok := gw["spec"].(map[string]interface{}); ok {
			if class, ok := spec["gatewayClassName"]; ok {
				b = append(b, fmt.Sprintf("  gatewayClassName: %s", class))
			}
			if listeners, ok := spec["listeners"].([]interface{}); ok {
				b = append(b, "  listeners:")
				for _, l := range listeners {
					lm := l.(map[string]interface{})
					b = append(b, fmt.Sprintf("    - name: %s", lm["name"]))
					b = append(b, fmt.Sprintf("      port: %v", lm["port"]))
					b = append(b, fmt.Sprintf("      protocol: %s", lm["protocol"]))
					if allowed, ok := lm["allowedRoutes"].(map[string]interface{}); ok {
						b = append(b, "      allowedRoutes:")
						if ns, ok := allowed["namespaces"].(map[string]interface{}); ok {
							b = append(b, "        namespaces:")
							b = append(b, fmt.Sprintf("          from: %s", ns["from"]))
						}
					}
				}
			}
		}
		b = append(b, "---")
	}

	// ListenerSet
	if len(t.ListenerSets) > 0 {
		b = append(b, "{{- if $.Values.gatewayApi.listenerSet.enabled }}")
		for _, ls := range t.ListenerSets {
			b = append(b, "apiVersion: gateway.networking.k8s.io/v1")
			b = append(b, "kind: ListenerSet")
			b = append(b, "metadata:")
			b = append(b, fmt.Sprintf("  name: %s", ls["name"]))
			b = append(b, "  namespace: {{ $.Release.Namespace }}")
			b = append(b, "spec:")
			if spec, ok := ls["spec"].(map[string]interface{}); ok {
				if pref, ok := spec["parentRef"].(map[string]interface{}); ok {
					b = append(b, "  parentRef:")
					if n, ok := pref["name"].(string); ok {
						b = append(b, fmt.Sprintf("    name: %s", n))
					}
					if ns, ok := pref["namespace"].(string); ok && ns != "" {
						b = append(b, fmt.Sprintf("    namespace: %s", ns))
					}
				}
				if listeners, ok := spec["listeners"].([]interface{}); ok {
					b = append(b, "  listeners:")
					for _, l := range listeners {
						lm := l.(map[string]interface{})
						b = append(b, fmt.Sprintf("    - name: %s", lm["name"]))
						b = append(b, fmt.Sprintf("      port: %v", lm["port"]))
						b = append(b, fmt.Sprintf("      protocol: %s", lm["protocol"]))
						b = append(b, fmt.Sprintf("      hostname: %s", lm["hostname"]))
						if tlsRaw, ok := lm["tls"]; ok {
							b = append(b, "      tls:")
							tls := tlsRaw.(map[string]interface{})
							b = append(b, fmt.Sprintf("        mode: %s", tls["mode"]))
							if crefRaw, ok := tls["certificateRefs"]; ok {
								cref := crefRaw.([]interface{})
								b = append(b, "        certificateRefs:")
								for _, c := range cref {
									cMap := c.(map[string]interface{})
									b = append(b, fmt.Sprintf("          - name: %s", cMap["name"]))
								}
							}
						}
					}
				}
			}
			b = append(b, "---")
		}
		b = append(b, "{{- end }}")
	}

	// HTTPRoute
	for _, hr := range t.HTTPRoutes {
		b = append(b, "apiVersion: gateway.networking.k8s.io/v1")
		b = append(b, "kind: HTTPRoute")
		b = append(b, "metadata:")
		b = append(b, fmt.Sprintf("  name: %s", hr["name"]))
		b = append(b, "  namespace: {{ $.Release.Namespace }}")
		b = append(b, "spec:")
		spec := hr["spec"].(map[string]interface{})

		if pRefs, ok := spec["parentRefs"].([]interface{}); ok && len(pRefs) > 0 {
			b = append(b, "  parentRefs:")
			for _, pRaw := range pRefs {
				p := pRaw.(map[string]interface{})
				b = append(b, fmt.Sprintf("    - group: %s", p["group"]))
				b = append(b, fmt.Sprintf("      kind: %s", p["kind"]))
				b = append(b, fmt.Sprintf("      name: %s", p["name"]))
				if ns, ok := p["namespace"].(string); ok && ns != "" {
					b = append(b, fmt.Sprintf("      namespace: %s", ns))
				}
			}
		}

		if hostnamesRaw, ok := spec["hostnames"]; ok {
			hostnames := hostnamesRaw.([]string)
			b = append(b, "  hostnames:")
			for _, h := range hostnames {
				b = append(b, fmt.Sprintf("    - %s", h))
			}
		}

		if rulesRaw, ok := spec["rules"]; ok {
			rules := rulesRaw.([]interface{})
			b = append(b, "  rules:")
			for _, ruleRaw := range rules {
				rule := ruleRaw.(map[string]interface{})
				b = append(b, "    - matches:")
				if matchesRaw, ok := rule["matches"].([]interface{}); ok {
					for _, mRaw := range matchesRaw {
						m := mRaw.(map[string]interface{})
						path := m["path"].(map[string]interface{})
						b = append(b, "        - path:")
						b = append(b, fmt.Sprintf("            type: %s", path["type"]))
						b = append(b, fmt.Sprintf("            value: %s", path["value"]))
					}
				}
				if backendRefsRaw, ok := rule["backendRefs"].([]interface{}); ok {
					b = append(b, "      backendRefs:")
					for _, brRaw := range backendRefsRaw {
						br := brRaw.(map[string]interface{})
						b = append(b, fmt.Sprintf("        - name: %s", br["name"]))
						if port, ok := br["port"]; ok {
							b = append(b, fmt.Sprintf("          port: %v", port))
						}
					}
				}
			}
		}
		b = append(b, "---")
	}

	// TLSRoute
	for _, tr := range t.TLSRoutes {
		apiVersion := "gateway.networking.k8s.io/v1"
		if v, ok := tr["apiVersion"].(string); ok && v != "" {
			apiVersion = v
		}
		b = append(b, fmt.Sprintf("apiVersion: %s", apiVersion))
		b = append(b, "kind: TLSRoute")
		b = append(b, "metadata:")
		b = append(b, fmt.Sprintf("  name: %s", tr["name"]))
		b = append(b, "  namespace: {{ $.Release.Namespace }}")
		b = append(b, "spec:")
		spec := tr["spec"].(map[string]interface{})

		if pRefs, ok := spec["parentRefs"].([]interface{}); ok && len(pRefs) > 0 {
			b = append(b, "  parentRefs:")
			for _, pRaw := range pRefs {
				p := pRaw.(map[string]interface{})
				b = append(b, fmt.Sprintf("    - group: %s", p["group"]))
				b = append(b, fmt.Sprintf("      kind: %s", p["kind"]))
				b = append(b, fmt.Sprintf("      name: %s", p["name"]))
				if ns, ok := p["namespace"].(string); ok && ns != "" {
					b = append(b, fmt.Sprintf("      namespace: %s", ns))
				}
			}
		}

		if hostnamesRaw, ok := spec["hostnames"]; ok {
			hostnames := hostnamesRaw.([]string)
			b = append(b, "  hostnames:")
			for _, h := range hostnames {
				b = append(b, fmt.Sprintf("    - %s", h))
			}
		}

		if rulesRaw, ok := spec["rules"]; ok {
			rules := rulesRaw.([]interface{})
			b = append(b, "  rules:")
			for _, ruleRaw := range rules {
				rule := ruleRaw.(map[string]interface{})
				b = append(b, "    - backendRefs:")
				if backendRefsRaw, ok := rule["backendRefs"].([]interface{}); ok {
					for _, brRaw := range backendRefsRaw {
						br := brRaw.(map[string]interface{})
						b = append(b, fmt.Sprintf("        - name: %s", br["name"]))
						if port, ok := br["port"]; ok {
							b = append(b, fmt.Sprintf("          port: %v", port))
						}
					}
				}
			}
		}
		b = append(b, "---")
	}

	// ReferenceGrant
	for _, rg := range t.ReferenceGrants {
		b = append(b, "apiVersion: gateway.networking.k8s.io/v1beta1")
		b = append(b, "kind: ReferenceGrant")
		b = append(b, "metadata:")
		b = append(b, fmt.Sprintf("  name: %s", rg["name"]))
		b = append(b, "  namespace: {{ $.Release.Namespace }}")
		b = append(b, "spec:")
		if spec, ok := rg["spec"].(map[string]interface{}); ok {
			if from, ok := spec["from"].([]interface{}); ok {
				b = append(b, "  from:")
				for _, fRaw := range from {
					f := fRaw.(map[string]interface{})
					b = append(b, fmt.Sprintf("    - group: %s", f["group"]))
					b = append(b, fmt.Sprintf("      kind: %s", f["kind"]))
					b = append(b, fmt.Sprintf("      namespace: %s", f["namespace"]))
				}
			}
			if to, ok := spec["to"].([]interface{}); ok {
				b = append(b, "  to:")
				for _, tRaw := range to {
					tg := tRaw.(map[string]interface{})
					b = append(b, fmt.Sprintf("    - group: \"%s\"", tg["group"]))
					b = append(b, fmt.Sprintf("      kind: %s", tg["kind"]))
					b = append(b, fmt.Sprintf("      name: %s", tg["name"]))
				}
			}
		}
		b = append(b, "---")
	}

	b = append(b, "{{- end }}")

	return strings.Join(b, "\n") + "\n"
}

func guardIngressTemplate(content string) (string, bool) {
	// Wraps original ingress template with a gatewayApi disable check
	if strings.Contains(content, "{{- if not (and $.Values.gatewayApi $.Values.gatewayApi.enabled) }}") {
		return content, true
	}
	guardStr := "{{- if not (and $.Values.gatewayApi $.Values.gatewayApi.enabled) }}\n"
	return guardStr + content + "{{- end }}\n", true
}
