package generator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// IngressController identifies the type of ingress controller in use.
type IngressController string

const (
	ControllerNginx   IngressController = "nginx"
	ControllerTraefik IngressController = "traefik"
	ControllerHAProxy IngressController = "haproxy"
	ControllerIstio   IngressController = "istio"
	ControllerUnknown IngressController = "unknown"
)

// IngressFeature represents a specific ingress capability to generate annotations for.
type IngressFeature string

const (
	IngressCanary      IngressFeature = "canary"
	IngressRateLimit   IngressFeature = "rate-limit"
	IngressCORS        IngressFeature = "cors"
	IngressAuth        IngressFeature = "auth"
	IngressRewrite     IngressFeature = "rewrite"
	IngressSSLRedirect IngressFeature = "ssl-redirect"
)

// DetectIngressController inspects a list of ProcessedResources and returns the
// ingress controller in use by applying the following priority order:
//
//  1. IngressClass spec.controller field
//  2. kubernetes.io/ingress.class annotation on Ingress resources
//  3. Deployment container image names
//  4. ControllerUnknown
func DetectIngressController(resources []*types.ProcessedResource) IngressController {
	if len(resources) == 0 {
		return ControllerUnknown
	}

	// Priority 1: IngressClass spec.controller
	for _, r := range resources {
		if r == nil || r.Original == nil {
			continue
		}
		if r.Original.GVK.Kind != "IngressClass" {
			continue
		}
		spec, ok := r.Original.Object.Object["spec"].(map[string]interface{})
		if !ok {
			continue
		}
		ctrl, ok := spec["controller"].(string)
		if !ok || ctrl == "" {
			continue
		}
		ctrl = strings.ToLower(ctrl)
		switch {
		case strings.Contains(ctrl, "ingress-nginx") || strings.Contains(ctrl, "nginx"):
			return ControllerNginx
		case strings.Contains(ctrl, "traefik"):
			return ControllerTraefik
		case strings.Contains(ctrl, "haproxy"):
			return ControllerHAProxy
		case strings.Contains(ctrl, "istio"):
			return ControllerIstio
		}
	}

	// Priority 2: kubernetes.io/ingress.class annotation on Ingress resources
	for _, r := range resources {
		if r == nil || r.Original == nil {
			continue
		}
		if r.Original.GVK.Kind != "Ingress" {
			continue
		}
		annotations := r.Original.Object.GetAnnotations()
		class, ok := annotations["kubernetes.io/ingress.class"]
		if !ok || class == "" {
			continue
		}
		class = strings.ToLower(class)
		switch {
		case strings.Contains(class, "nginx"):
			return ControllerNginx
		case strings.Contains(class, "traefik"):
			return ControllerTraefik
		case strings.Contains(class, "haproxy"):
			return ControllerHAProxy
		}
	}

	// Priority 3: Deployment container image names
	for _, r := range resources {
		if r == nil || r.Original == nil {
			continue
		}
		if r.Original.GVK.Kind != "Deployment" {
			continue
		}
		spec, ok := r.Original.Object.Object["spec"].(map[string]interface{})
		if !ok {
			continue
		}
		tmpl, ok := spec["template"].(map[string]interface{})
		if !ok {
			continue
		}
		podSpec, ok := tmpl["spec"].(map[string]interface{})
		if !ok {
			continue
		}
		containers, ok := podSpec["containers"].([]interface{})
		if !ok {
			continue
		}
		for _, c := range containers {
			container, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			image, ok := container["image"].(string)
			if !ok || image == "" {
				continue
			}
			imageLower := strings.ToLower(image)
			switch {
			case strings.Contains(imageLower, "traefik"):
				return ControllerTraefik
			case strings.Contains(imageLower, "nginx"):
				return ControllerNginx
			case strings.Contains(imageLower, "istio"):
				return ControllerIstio
			case strings.Contains(imageLower, "haproxy"):
				return ControllerHAProxy
			}
		}
	}

	return ControllerUnknown
}

// GenerateIngressAnnotations returns a map of controller-specific Kubernetes
// annotations that enable the requested ingress features.
func GenerateIngressAnnotations(controller IngressController, features []IngressFeature) map[string]string {
	annotations := make(map[string]string)

	switch controller {
	case ControllerNginx:
		for _, f := range features {
			switch f {
			case IngressCanary:
				annotations["nginx.ingress.kubernetes.io/canary"] = "true"
				annotations["nginx.ingress.kubernetes.io/canary-weight"] = "20"
			case IngressRateLimit:
				annotations["nginx.ingress.kubernetes.io/limit-rps"] = "10"
			case IngressCORS:
				annotations["nginx.ingress.kubernetes.io/enable-cors"] = "true"
				annotations["nginx.ingress.kubernetes.io/cors-allow-origin"] = "*"
			case IngressAuth:
				annotations["nginx.ingress.kubernetes.io/auth-url"] = "https://auth.example.com/verify"
			case IngressRewrite:
				annotations["nginx.ingress.kubernetes.io/rewrite-target"] = "/$1"
			case IngressSSLRedirect:
				annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
			}
		}

	case ControllerTraefik:
		for _, f := range features {
			switch f {
			case IngressRateLimit:
				annotations["traefik.ingress.kubernetes.io/rate-limit"] = "10"
			case IngressSSLRedirect:
				annotations["traefik.ingress.kubernetes.io/redirect-entry-point"] = "https"
			case IngressCanary:
				annotations["traefik.ingress.kubernetes.io/router.middlewares"] = "canary@kubernetescrd"
			case IngressCORS:
				annotations["traefik.ingress.kubernetes.io/router.middlewares"] = "cors@kubernetescrd"
			case IngressAuth:
				annotations["traefik.ingress.kubernetes.io/router.middlewares"] = "auth@kubernetescrd"
			case IngressRewrite:
				annotations["traefik.ingress.kubernetes.io/router.middlewares"] = "rewrite@kubernetescrd"
			}
		}

	case ControllerHAProxy:
		for _, f := range features {
			switch f {
			case IngressSSLRedirect:
				annotations["haproxy.org/ssl-redirect"] = "true"
			case IngressRateLimit:
				annotations["haproxy.org/rate-limit-requests"] = "10"
			case IngressCanary:
				annotations["haproxy.org/canary"] = "true"
			case IngressCORS:
				annotations["haproxy.org/cors-enable"] = "true"
			case IngressAuth:
				annotations["haproxy.org/auth-url"] = "https://auth.example.com/verify"
			case IngressRewrite:
				annotations["haproxy.org/rewrite-target"] = "/$1"
			}
		}

	default:
		// Unknown controller: return only the generic class annotation.
		annotations["kubernetes.io/ingress.class"] = ""
	}

	return annotations
}

// InjectIngressAnnotations adds controller-specific ingress annotations to every
// Ingress template in the chart. Non-Ingress templates are left untouched.
// If chart is nil, nil is returned.
func InjectIngressAnnotations(chart *types.GeneratedChart, controller IngressController, features []IngressFeature) *types.GeneratedChart {
	if chart == nil {
		return nil
	}

	annotations := GenerateIngressAnnotations(controller, features)

	// Copy templates map — do not mutate the original chart.
	newTemplates := make(map[string]string, len(chart.Templates))
	for path, content := range chart.Templates {
		if strings.Contains(content, "kind: Ingress") {
			content = injectIngressAnnotationsBlock(content, annotations)
		}
		newTemplates[path] = content
	}

	return &types.GeneratedChart{
		Name:          chart.Name,
		Path:          chart.Path,
		ChartYAML:     chart.ChartYAML,
		ValuesYAML:    chart.ValuesYAML,
		Templates:     newTemplates,
		Helpers:       chart.Helpers,
		Notes:         chart.Notes,
		ValuesSchema:  chart.ValuesSchema,
		ExternalFiles: chart.ExternalFiles,
	}
}

// injectIngressAnnotationsBlock inserts an annotations block immediately after
// the first "metadata:\n  name: <name>" block found in the template string.
// Keys are sorted for deterministic output.
// This is the ingress-specific helper; the shared injectAnnotationsIntoTemplate
// function is defined in cloudannotations.go and is reused where available.
func injectIngressAnnotationsBlock(tmpl string, annotations map[string]string) string {
	if len(annotations) == 0 {
		return tmpl
	}

	metadataRe := regexp.MustCompile(`(metadata:\s*\n\s+name:\s*\S+)`)

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(annotations))
	for k := range annotations {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	lines = append(lines, "  annotations:")
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("    %s: %q", k, annotations[k]))
	}
	annotationsBlock := strings.Join(lines, "\n")

	return metadataRe.ReplaceAllString(tmpl, "$1\n"+annotationsBlock)
}
