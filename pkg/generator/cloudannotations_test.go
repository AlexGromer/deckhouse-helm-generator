package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test Plan
// ============================================================
//
// 1.  TestCloudAnnotations_AWS_ExternalNLB         — happy    AWS+external → 3 annotations (nlb, internet-facing, cross-zone)
// 2.  TestCloudAnnotations_AWS_InternalNLB         — happy    AWS+internal → scheme=internal
// 3.  TestCloudAnnotations_GCP_ExternalLB          — happy    GCP+external → NEG annotation present
// 4.  TestCloudAnnotations_GCP_InternalLB          — happy    GCP+internal → Internal load-balancer-type annotation
// 5.  TestCloudAnnotations_Azure_ExternalLB        — happy    Azure+external → health-probe present, no internal annotation
// 6.  TestCloudAnnotations_Azure_InternalLB        — happy    Azure+internal → azure-load-balancer-internal=true
// 7.  TestCloudAnnotations_EmptyProvider           — edge     empty provider → empty map, no panic
// 8.  TestCloudAnnotations_UnknownProvider         — error    "digitalocean" → empty map (unknown provider)
// 9.  TestCloudAnnotations_InjectIntoChart_Service — integration  Service template → annotation block injected
// 10. TestCloudAnnotations_InjectIntoChart_Ingress — integration  AWS + Ingress template → ALB annotations
// 11. TestCloudAnnotations_ValuesStructure         — happy    generateCloudValues → required keys present
// 12. TestCloudAnnotations_MultipleServices        — edge     2 Service templates → both annotated
// 13. TestCloudAnnotations_NilChart_ReturnsNil     — error    nil chart → nil returned, no panic

// ============================================================
// Helpers — note: makeChart is defined in airgap_test.go
// ============================================================

// cloudSvcTemplate is a minimal realistic LoadBalancer Service YAML used by cloud annotation tests.
const cloudSvcTemplate = "apiVersion: v1\nkind: Service\nmetadata:\n  name: myapp\nspec:\n  type: LoadBalancer\n  ports:\n    - port: 80"

// cloudIngressTemplate is a minimal realistic Ingress YAML used by cloud annotation tests.
const cloudIngressTemplate = "apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: myapp\nspec:\n  rules:\n    - host: example.com"

// ============================================================
// Section 1: GenerateCloudAnnotations — AWS
// ============================================================

func TestCloudAnnotations_AWS_ExternalNLB(t *testing.T) {
	config := CloudAnnotationConfig{
		Provider: CloudAWS,
		Internal: false,
		Scheme:   "internet-facing",
	}

	annotations := GenerateCloudAnnotations(config)

	if len(annotations) == 0 {
		t.Fatal("expected non-empty annotations for AWS external NLB")
	}

	lbType, ok := annotations["service.beta.kubernetes.io/aws-load-balancer-type"]
	if !ok {
		t.Error("expected annotation 'service.beta.kubernetes.io/aws-load-balancer-type'")
	} else if lbType != "nlb" {
		t.Errorf("expected aws-load-balancer-type='nlb', got '%s'", lbType)
	}

	scheme, ok := annotations["service.beta.kubernetes.io/aws-load-balancer-scheme"]
	if !ok {
		t.Error("expected annotation 'service.beta.kubernetes.io/aws-load-balancer-scheme'")
	} else if scheme != "internet-facing" {
		t.Errorf("expected aws-load-balancer-scheme='internet-facing', got '%s'", scheme)
	}

	crossZone, ok := annotations["service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled"]
	if !ok {
		t.Error("expected annotation 'service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled'")
	} else if crossZone != "true" {
		t.Errorf("expected cross-zone='true', got '%s'", crossZone)
	}

	if len(annotations) < 3 {
		t.Errorf("expected at least 3 annotations for AWS external NLB, got %d", len(annotations))
	}
}

func TestCloudAnnotations_AWS_InternalNLB(t *testing.T) {
	config := CloudAnnotationConfig{
		Provider: CloudAWS,
		Internal: true,
		Scheme:   "internal",
	}

	annotations := GenerateCloudAnnotations(config)

	scheme, ok := annotations["service.beta.kubernetes.io/aws-load-balancer-scheme"]
	if !ok {
		t.Error("expected annotation 'service.beta.kubernetes.io/aws-load-balancer-scheme'")
	} else if scheme != "internal" {
		t.Errorf("expected aws-load-balancer-scheme='internal' for internal NLB, got '%s'", scheme)
	}
}

// ============================================================
// Section 2: GenerateCloudAnnotations — GCP
// ============================================================

func TestCloudAnnotations_GCP_ExternalLB(t *testing.T) {
	config := CloudAnnotationConfig{
		Provider: CloudGCP,
		Internal: false,
	}

	annotations := GenerateCloudAnnotations(config)

	if len(annotations) == 0 {
		t.Fatal("expected non-empty annotations for GCP external LB")
	}

	neg, ok := annotations["cloud.google.com/neg"]
	if !ok {
		t.Error("expected annotation 'cloud.google.com/neg' for GCP external LB")
	} else if !strings.Contains(neg, "ingress") {
		t.Errorf("expected 'cloud.google.com/neg' to reference ingress, got '%s'", neg)
	}

	// External GCP LB must NOT carry load-balancer-type=Internal
	if val, exists := annotations["cloud.google.com/load-balancer-type"]; exists && val == "Internal" {
		t.Error("external GCP LB must not have load-balancer-type=Internal")
	}
}

func TestCloudAnnotations_GCP_InternalLB(t *testing.T) {
	config := CloudAnnotationConfig{
		Provider: CloudGCP,
		Internal: true,
	}

	annotations := GenerateCloudAnnotations(config)

	lbType, ok := annotations["cloud.google.com/load-balancer-type"]
	if !ok {
		t.Error("expected annotation 'cloud.google.com/load-balancer-type' for GCP internal LB")
	} else if lbType != "Internal" {
		t.Errorf("expected load-balancer-type='Internal', got '%s'", lbType)
	}
}

// ============================================================
// Section 3: GenerateCloudAnnotations — Azure
// ============================================================

func TestCloudAnnotations_Azure_ExternalLB(t *testing.T) {
	config := CloudAnnotationConfig{
		Provider: CloudAzure,
		Internal: false,
	}

	annotations := GenerateCloudAnnotations(config)

	if len(annotations) == 0 {
		t.Fatal("expected non-empty annotations for Azure external LB")
	}

	// Health probe annotation must be present for external Azure LB
	probe, ok := annotations["service.beta.kubernetes.io/azure-load-balancer-health-probe-request-path"]
	if !ok {
		t.Error("expected annotation 'service.beta.kubernetes.io/azure-load-balancer-health-probe-request-path' for Azure external LB")
	} else if probe != "/healthz" {
		t.Errorf("expected health-probe-request-path='/healthz', got '%s'", probe)
	}

	// Internal annotation must NOT be set to "true" for external LB
	if val, exists := annotations["service.beta.kubernetes.io/azure-load-balancer-internal"]; exists && val == "true" {
		t.Error("azure-load-balancer-internal must not be 'true' for external Azure LB")
	}
}

func TestCloudAnnotations_Azure_InternalLB(t *testing.T) {
	config := CloudAnnotationConfig{
		Provider: CloudAzure,
		Internal: true,
	}

	annotations := GenerateCloudAnnotations(config)

	internal, ok := annotations["service.beta.kubernetes.io/azure-load-balancer-internal"]
	if !ok {
		t.Error("expected annotation 'service.beta.kubernetes.io/azure-load-balancer-internal' for Azure internal LB")
	} else if internal != "true" {
		t.Errorf("expected azure-load-balancer-internal='true', got '%s'", internal)
	}
}

// ============================================================
// Section 4: GenerateCloudAnnotations — edge and error cases
// ============================================================

func TestCloudAnnotations_EmptyProvider_NoAnnotations(t *testing.T) {
	config := CloudAnnotationConfig{
		Provider: CloudProvider(""),
		Internal: false,
	}

	annotations := GenerateCloudAnnotations(config)

	if annotations == nil {
		t.Fatal("GenerateCloudAnnotations must return a non-nil map (may be empty)")
	}
	if len(annotations) != 0 {
		t.Errorf("expected empty annotations map for empty provider, got %d entries: %v", len(annotations), annotations)
	}
}

func TestCloudAnnotations_UnknownProvider_ReturnsError(t *testing.T) {
	config := CloudAnnotationConfig{
		Provider: CloudProvider("digitalocean"),
		Internal: false,
	}

	annotations := GenerateCloudAnnotations(config)

	// An unknown provider must not silently produce AWS/GCP/Azure-specific annotations.
	if annotations == nil {
		t.Fatal("GenerateCloudAnnotations must return a non-nil map for unknown provider")
	}

	awsKey := "service.beta.kubernetes.io/aws-load-balancer-type"
	gcpKey := "cloud.google.com/neg"
	azureKey := "service.beta.kubernetes.io/azure-load-balancer-internal"

	if _, found := annotations[awsKey]; found {
		t.Errorf("unexpected AWS annotation for unknown provider 'digitalocean'")
	}
	if _, found := annotations[gcpKey]; found {
		t.Errorf("unexpected GCP annotation for unknown provider 'digitalocean'")
	}
	if _, found := annotations[azureKey]; found {
		t.Errorf("unexpected Azure annotation for unknown provider 'digitalocean'")
	}
}

// ============================================================
// Section 5: InjectCloudAnnotations — chart injection
// ============================================================

func TestCloudAnnotations_InjectIntoChart_ServiceGetsAnnotations(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/service.yaml": cloudSvcTemplate,
	})

	config := CloudAnnotationConfig{
		Provider: CloudAWS,
		Internal: false,
		Scheme:   "internet-facing",
	}

	result := InjectCloudAnnotations(chart, config)

	if result == nil {
		t.Fatal("InjectCloudAnnotations returned nil for valid chart")
	}

	svcContent, ok := result.Templates["templates/service.yaml"]
	if !ok {
		t.Fatal("templates/service.yaml missing after injection")
	}

	// The Service template must contain an annotations block with at least one cloud annotation.
	if !strings.Contains(svcContent, "annotations") {
		t.Error("expected 'annotations' block injected into Service template")
	}

	// At minimum, an AWS NLB annotation key must appear in the rendered template.
	if !strings.Contains(svcContent, "aws-load-balancer") {
		t.Error("expected AWS load-balancer annotation key in injected Service template")
	}
}

func TestCloudAnnotations_InjectIntoChart_IngressGetsALBAnnotations(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/ingress.yaml": cloudIngressTemplate,
	})

	config := CloudAnnotationConfig{
		Provider: CloudAWS,
		Internal: false,
		Scheme:   "internet-facing",
	}

	result := InjectCloudAnnotations(chart, config)

	if result == nil {
		t.Fatal("InjectCloudAnnotations returned nil for valid chart with Ingress")
	}

	ingressContent, ok := result.Templates["templates/ingress.yaml"]
	if !ok {
		t.Fatal("templates/ingress.yaml missing after injection")
	}

	// AWS ALB Ingress annotations use the alb.ingress.kubernetes.io prefix.
	if !strings.Contains(ingressContent, "alb.ingress.kubernetes.io") {
		t.Error("expected ALB ingress annotations (alb.ingress.kubernetes.io/*) in AWS Ingress template")
	}
}

// ============================================================
// Section 6: generateCloudValues — values map structure
// ============================================================

func TestCloudAnnotations_ValuesStructure(t *testing.T) {
	config := CloudAnnotationConfig{
		Provider: CloudGCP,
		Internal: true,
		Scheme:   "internal",
	}

	values := generateCloudValues(config)

	if values == nil {
		t.Fatal("generateCloudValues must return a non-nil map")
	}

	// Must contain top-level "cloud" key.
	cloud, ok := values["cloud"]
	if !ok {
		t.Fatal("expected top-level 'cloud' key in cloud values map")
	}

	cloudMap, ok := cloud.(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'cloud' to be map[string]interface{}, got %T", cloud)
	}

	// cloud.provider
	provider, ok := cloudMap["provider"]
	if !ok {
		t.Error("expected 'cloud.provider' key in cloud values")
	} else if provider != string(CloudGCP) {
		t.Errorf("expected cloud.provider='gcp', got '%v'", provider)
	}

	// cloud.loadBalancer
	lb, ok := cloudMap["loadBalancer"]
	if !ok {
		t.Fatal("expected 'cloud.loadBalancer' key in cloud values")
	}

	lbMap, ok := lb.(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'cloud.loadBalancer' to be map[string]interface{}, got %T", lb)
	}

	// cloud.loadBalancer.internal
	if _, ok := lbMap["internal"]; !ok {
		t.Error("expected 'cloud.loadBalancer.internal' key in cloud values")
	}

	// cloud.loadBalancer.scheme
	if _, ok := lbMap["scheme"]; !ok {
		t.Error("expected 'cloud.loadBalancer.scheme' key in cloud values")
	}
}

// ============================================================
// Section 7: InjectCloudAnnotations — multi-service and nil
// ============================================================

func TestCloudAnnotations_MultipleServices_AllAnnotated(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/service-frontend.yaml": "apiVersion: v1\nkind: Service\nmetadata:\n  name: frontend\nspec:\n  type: LoadBalancer\n  ports:\n    - port: 80",
		"templates/service-backend.yaml":  "apiVersion: v1\nkind: Service\nmetadata:\n  name: backend\nspec:\n  type: LoadBalancer\n  ports:\n    - port: 8080",
	})

	config := CloudAnnotationConfig{
		Provider: CloudAzure,
		Internal: false,
	}

	result := InjectCloudAnnotations(chart, config)

	if result == nil {
		t.Fatal("InjectCloudAnnotations returned nil for valid multi-service chart")
	}

	for _, templateName := range []string{
		"templates/service-frontend.yaml",
		"templates/service-backend.yaml",
	} {
		content, ok := result.Templates[templateName]
		if !ok {
			t.Errorf("template '%s' missing after injection", templateName)
			continue
		}
		if !strings.Contains(content, "annotations") {
			t.Errorf("template '%s' missing 'annotations' block after Azure injection", templateName)
		}
		if !strings.Contains(content, "azure") {
			t.Errorf("template '%s' missing Azure annotation key after injection", templateName)
		}
	}
}

func TestCloudAnnotations_NilChart_ReturnsNil(t *testing.T) {
	config := CloudAnnotationConfig{
		Provider: CloudAWS,
		Internal: false,
		Scheme:   "internet-facing",
	}

	// Must not panic; must return nil.
	var chart *types.GeneratedChart
	result := InjectCloudAnnotations(chart, config)

	if result != nil {
		t.Errorf("expected nil return for nil chart input, got %+v", result)
	}
}
