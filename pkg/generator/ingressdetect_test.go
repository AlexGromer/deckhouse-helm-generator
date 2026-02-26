package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ============================================================
// Test Helpers (local to ingressdetect tests)
// ============================================================

// makeIngressResource creates a ProcessedResource with the given kind, name,
// labels, and annotations. It does NOT take a namespace parameter to avoid
// conflicting with the existing makeProcessedResource(kind, name, namespace,
// labels) helper defined in grouping_test.go.
func makeIngressResource(kind, name string, labels, annotations map[string]string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetAPIVersion("v1")
	if labels != nil {
		obj.SetLabels(labels)
	}
	if annotations != nil {
		obj.SetAnnotations(annotations)
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Kind: kind},
		},
	}
}

// makeIngressClass creates a ProcessedResource representing an IngressClass
// with the given spec.controller value.
func makeIngressClass(name, controller string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind("IngressClass")
	obj.SetName(name)
	obj.SetAPIVersion("networking.k8s.io/v1")
	obj.Object["spec"] = map[string]interface{}{
		"controller": controller,
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK: schema.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "IngressClass",
			},
		},
	}
}

// makeDeploymentWithImage creates a ProcessedResource representing a Deployment
// whose first container uses the given image string.
func makeDeploymentWithImage(name, image string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind("Deployment")
	obj.SetName(name)
	obj.SetAPIVersion("apps/v1")
	obj.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": image,
					},
				},
			},
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK: schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			},
		},
	}
}

// ============================================================
// Section 1: DetectIngressController — detection from IngressClass
// ============================================================

func TestIngressDetect_NginxFromIngressClass(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeIngressClass("nginx", "k8s.io/ingress-nginx"),
	}

	got := DetectIngressController(resources)

	if got != ControllerNginx {
		t.Errorf("expected ControllerNginx, got %q", got)
	}
}

func TestIngressDetect_TraefikFromIngressClass(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeIngressClass("traefik", "traefik.io/ingress-controller"),
	}

	got := DetectIngressController(resources)

	if got != ControllerTraefik {
		t.Errorf("expected ControllerTraefik, got %q", got)
	}
}

// ============================================================
// Section 2: DetectIngressController — detection from annotations
// ============================================================

func TestIngressDetect_NginxFromAnnotation(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeIngressResource("Ingress", "my-ingress", nil, map[string]string{
			"kubernetes.io/ingress.class": "nginx",
		}),
	}

	got := DetectIngressController(resources)

	if got != ControllerNginx {
		t.Errorf("expected ControllerNginx from ingress.class annotation, got %q", got)
	}
}

func TestIngressDetect_HAProxyFromAnnotation(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeIngressResource("Ingress", "my-ingress", nil, map[string]string{
			"kubernetes.io/ingress.class": "haproxy",
		}),
	}

	got := DetectIngressController(resources)

	if got != ControllerHAProxy {
		t.Errorf("expected ControllerHAProxy from ingress.class annotation, got %q", got)
	}
}

// ============================================================
// Section 3: DetectIngressController — detection from Deployment image
// ============================================================

func TestIngressDetect_TraefikFromDeploymentImage(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithImage("traefik", "traefik:v2.10"),
	}

	got := DetectIngressController(resources)

	if got != ControllerTraefik {
		t.Errorf("expected ControllerTraefik from Deployment image 'traefik:v2.10', got %q", got)
	}
}

func TestIngressDetect_IstioFromDeploymentImage(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithImage("istiod", "istio/pilot:1.19"),
	}

	got := DetectIngressController(resources)

	if got != ControllerIstio {
		t.Errorf("expected ControllerIstio from Deployment image 'istio/pilot:1.19', got %q", got)
	}
}

// ============================================================
// Section 4: DetectIngressController — edge and fallback cases
// ============================================================

func TestIngressDetect_NoResources_Unknown(t *testing.T) {
	got := DetectIngressController([]*types.ProcessedResource{})

	if got != ControllerUnknown {
		t.Errorf("expected ControllerUnknown for empty resource list, got %q", got)
	}
}

func TestIngressDetect_NilResources_Unknown(t *testing.T) {
	got := DetectIngressController(nil)

	if got != ControllerUnknown {
		t.Errorf("expected ControllerUnknown for nil resource list, got %q", got)
	}
}

// TestIngressDetect_PriorityIngressClassOverAnnotation verifies that the
// IngressClass spec.controller signal takes priority over the
// kubernetes.io/ingress.class annotation when both are present.
func TestIngressDetect_PriorityIngressClassOverAnnotation(t *testing.T) {
	resources := []*types.ProcessedResource{
		// IngressClass says nginx (priority 1)
		makeIngressClass("nginx", "k8s.io/ingress-nginx"),
		// Annotation says traefik (priority 2 — lower)
		makeIngressResource("Ingress", "my-ingress", nil, map[string]string{
			"kubernetes.io/ingress.class": "traefik",
		}),
	}

	got := DetectIngressController(resources)

	if got != ControllerNginx {
		t.Errorf("expected ControllerNginx (IngressClass takes priority over annotation), got %q", got)
	}
}

// ============================================================
// Section 5: GenerateIngressAnnotations — nginx feature sets
// ============================================================

func TestIngressAnnotations_NginxCanary(t *testing.T) {
	annotations := GenerateIngressAnnotations(ControllerNginx, []IngressFeature{IngressCanary})

	canary, ok := annotations["nginx.ingress.kubernetes.io/canary"]
	if !ok {
		t.Fatal("expected annotation 'nginx.ingress.kubernetes.io/canary'")
	}
	if canary != "true" {
		t.Errorf("expected canary='true', got %q", canary)
	}

	weight, ok := annotations["nginx.ingress.kubernetes.io/canary-weight"]
	if !ok {
		t.Fatal("expected annotation 'nginx.ingress.kubernetes.io/canary-weight'")
	}
	if weight != "20" {
		t.Errorf("expected canary-weight='20', got %q", weight)
	}
}

func TestIngressAnnotations_NginxRateLimitCORS(t *testing.T) {
	annotations := GenerateIngressAnnotations(ControllerNginx, []IngressFeature{IngressRateLimit, IngressCORS})

	// Rate-limit assertions
	rps, ok := annotations["nginx.ingress.kubernetes.io/limit-rps"]
	if !ok {
		t.Fatal("expected annotation 'nginx.ingress.kubernetes.io/limit-rps'")
	}
	if rps != "10" {
		t.Errorf("expected limit-rps='10', got %q", rps)
	}

	// CORS assertions
	cors, ok := annotations["nginx.ingress.kubernetes.io/enable-cors"]
	if !ok {
		t.Fatal("expected annotation 'nginx.ingress.kubernetes.io/enable-cors'")
	}
	if cors != "true" {
		t.Errorf("expected enable-cors='true', got %q", cors)
	}

	origin, ok := annotations["nginx.ingress.kubernetes.io/cors-allow-origin"]
	if !ok {
		t.Fatal("expected annotation 'nginx.ingress.kubernetes.io/cors-allow-origin'")
	}
	if origin != "*" {
		t.Errorf("expected cors-allow-origin='*', got %q", origin)
	}
}

func TestIngressAnnotations_NginxAuth(t *testing.T) {
	annotations := GenerateIngressAnnotations(ControllerNginx, []IngressFeature{IngressAuth})

	_, ok := annotations["nginx.ingress.kubernetes.io/auth-url"]
	if !ok {
		t.Fatal("expected annotation 'nginx.ingress.kubernetes.io/auth-url'")
	}
}

func TestIngressAnnotations_NginxRewrite(t *testing.T) {
	annotations := GenerateIngressAnnotations(ControllerNginx, []IngressFeature{IngressRewrite})

	target, ok := annotations["nginx.ingress.kubernetes.io/rewrite-target"]
	if !ok {
		t.Fatal("expected annotation 'nginx.ingress.kubernetes.io/rewrite-target'")
	}
	if !strings.Contains(target, "$1") {
		t.Errorf("expected rewrite-target to contain '$1', got %q", target)
	}
}

func TestIngressAnnotations_NginxSSLRedirect(t *testing.T) {
	annotations := GenerateIngressAnnotations(ControllerNginx, []IngressFeature{IngressSSLRedirect})

	redirect, ok := annotations["nginx.ingress.kubernetes.io/ssl-redirect"]
	if !ok {
		t.Fatal("expected annotation 'nginx.ingress.kubernetes.io/ssl-redirect'")
	}
	if redirect != "true" {
		t.Errorf("expected ssl-redirect='true', got %q", redirect)
	}
}

// ============================================================
// Section 6: GenerateIngressAnnotations — traefik and haproxy
// ============================================================

func TestIngressAnnotations_TraefikAnnotations(t *testing.T) {
	annotations := GenerateIngressAnnotations(ControllerTraefik, []IngressFeature{IngressRateLimit})

	if len(annotations) == 0 {
		t.Fatal("expected non-empty annotations for traefik + rate-limit")
	}

	// Traefik uses its own annotation namespace; none should be nginx-prefixed.
	for key := range annotations {
		if strings.HasPrefix(key, "nginx.ingress.kubernetes.io/") {
			t.Errorf("traefik annotations must not contain nginx prefix, found key %q", key)
		}
	}
}

func TestIngressAnnotations_HAProxySSLRedirect(t *testing.T) {
	annotations := GenerateIngressAnnotations(ControllerHAProxy, []IngressFeature{IngressSSLRedirect})

	if len(annotations) == 0 {
		t.Fatal("expected non-empty annotations for haproxy + ssl-redirect")
	}

	// HAProxy must not produce nginx-prefixed annotations.
	for key := range annotations {
		if strings.HasPrefix(key, "nginx.ingress.kubernetes.io/") {
			t.Errorf("haproxy annotations must not contain nginx prefix, found key %q", key)
		}
	}

	// At least one annotation must reference ssl or redirect.
	found := false
	for key, val := range annotations {
		if (strings.Contains(strings.ToLower(key), "ssl") ||
			strings.Contains(strings.ToLower(key), "redirect")) &&
			val != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one SSL/redirect annotation for haproxy + IngressSSLRedirect")
	}
}

// ============================================================
// Section 7: GenerateIngressAnnotations — unknown controller fallback
// ============================================================

// TestIngressAnnotations_UnknownController_Fallback verifies that an unknown
// controller still produces the generic kubernetes.io/ingress.class annotation
// and does not emit any controller-specific (nginx/traefik/haproxy) annotations.
func TestIngressAnnotations_UnknownController_Fallback(t *testing.T) {
	annotations := GenerateIngressAnnotations(ControllerUnknown, []IngressFeature{IngressCanary, IngressRateLimit})

	if len(annotations) == 0 {
		t.Fatal("expected at least the generic kubernetes.io/ingress.class annotation for unknown controller")
	}

	_, ok := annotations["kubernetes.io/ingress.class"]
	if !ok {
		t.Error("expected fallback annotation 'kubernetes.io/ingress.class' for unknown controller")
	}

	// Must not produce any controller-specific prefixed annotations.
	for key := range annotations {
		if strings.HasPrefix(key, "nginx.ingress.kubernetes.io/") {
			t.Errorf("unknown controller must not produce nginx-prefixed annotation %q", key)
		}
		if strings.HasPrefix(key, "traefik.ingress.kubernetes.io/") {
			t.Errorf("unknown controller must not produce traefik-prefixed annotation %q", key)
		}
		if strings.HasPrefix(key, "haproxy.org/") {
			t.Errorf("unknown controller must not produce haproxy-prefixed annotation %q", key)
		}
	}
}

// ============================================================
// Section 8: InjectIngressAnnotations — chart injection
// ============================================================

func TestInjectIngressAnnotations_NginxCanary_InjectsIntoIngressTemplate(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/ingress.yaml": "apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: myapp\nspec:\n  rules:\n    - host: example.com",
	})

	result := InjectIngressAnnotations(chart, ControllerNginx, []IngressFeature{IngressCanary})

	if result == nil {
		t.Fatal("InjectIngressAnnotations returned nil for valid chart")
	}

	content, ok := result.Templates["templates/ingress.yaml"]
	if !ok {
		t.Fatal("templates/ingress.yaml missing after injection")
	}

	if !strings.Contains(content, "annotations") {
		t.Error("expected 'annotations' block to be injected into Ingress template")
	}
	if !strings.Contains(content, "nginx.ingress.kubernetes.io/canary") {
		t.Error("expected canary annotation key in injected Ingress template")
	}
}

func TestInjectIngressAnnotations_NilChart_ReturnsNil(t *testing.T) {
	var chart *types.GeneratedChart

	result := InjectIngressAnnotations(chart, ControllerNginx, []IngressFeature{IngressCanary})

	if result != nil {
		t.Errorf("expected nil return for nil chart input, got %+v", result)
	}
}

func TestInjectIngressAnnotations_NoIngressTemplate_PreservesChart(t *testing.T) {
	original := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	result := InjectIngressAnnotations(original, ControllerNginx, []IngressFeature{IngressSSLRedirect})

	if result == nil {
		t.Fatal("InjectIngressAnnotations returned nil for chart without Ingress template")
	}

	// Original templates must be preserved unmodified.
	content, ok := result.Templates["templates/deployment.yaml"]
	if !ok {
		t.Fatal("templates/deployment.yaml missing after injection")
	}
	if !strings.Contains(content, "kind: Deployment") {
		t.Error("original Deployment template content was corrupted")
	}
}
