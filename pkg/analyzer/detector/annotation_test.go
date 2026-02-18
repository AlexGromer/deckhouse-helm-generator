package detector

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// makeAnnotationResource creates a ProcessedResource for annotation-focused tests.
// annotations is a map[string]string where keys are annotation keys and values are
// annotation values. extraFields is merged into the top-level object map (e.g. "spec").
func makeAnnotationResource(apiVersion, kind, name, namespace string, annotations map[string]string, extraFields map[string]interface{}) *types.ProcessedResource {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}

	if len(annotations) > 0 {
		annIface := make(map[string]interface{}, len(annotations))
		for k, v := range annotations {
			annIface[k] = v
		}
		meta := obj.Object["metadata"].(map[string]interface{})
		meta["annotations"] = annIface
	}

	for k, v := range extraFields {
		obj.Object[k] = v
	}

	gv, _ := schema.ParseGroupVersion(apiVersion)
	gvk := gv.WithKind(kind)

	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			Source: types.SourceFile,
			GVK:    gvk,
		},
	}
}

// buildAnnotationAllResources converts a slice of ProcessedResources into the
// allResources map keyed by their ResourceKey.
func buildAnnotationAllResources(resources ...*types.ProcessedResource) map[types.ResourceKey]*types.ProcessedResource {
	m := make(map[types.ResourceKey]*types.ProcessedResource, len(resources))
	for _, r := range resources {
		m[r.Original.ResourceKey()] = r
	}
	return m
}

// TestNewAnnotationDetector verifies the constructor, Name(), and Priority().
func TestNewAnnotationDetector(t *testing.T) {
	d := NewAnnotationDetector()

	if d == nil {
		t.Fatal("NewAnnotationDetector() returned nil")
	}

	if got := d.Name(); got != "annotation" {
		t.Errorf("Name() = %q; want %q", got, "annotation")
	}

	if got := d.Priority(); got != 70 {
		t.Errorf("Priority() = %d; want 70", got)
	}
}

// TestAnnotationDetector_CertManagerClusterIssuer verifies that an Ingress with a
// cert-manager.io/cluster-issuer annotation pointing to an existing ClusterIssuer
// produces exactly one relationship of type RelationAnnotation.
func TestAnnotationDetector_CertManagerClusterIssuer(t *testing.T) {
	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", "default",
		map[string]string{
			"cert-manager.io/cluster-issuer": "letsencrypt-prod",
		},
		nil,
	)

	// ClusterIssuer is cluster-scoped — namespace must be empty.
	clusterIssuer := makeAnnotationResource(
		"cert-manager.io/v1", "ClusterIssuer", "letsencrypt-prod", "",
		nil, nil,
	)

	allResources := buildAnnotationAllResources(ingress, clusterIssuer)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d: %v", len(rels), rels)
	}

	rel := rels[0]

	if rel.Type != types.RelationAnnotation {
		t.Errorf("Type = %q; want %q", rel.Type, types.RelationAnnotation)
	}

	wantField := "metadata.annotations[cert-manager.io/cluster-issuer]"
	if rel.Field != wantField {
		t.Errorf("Field = %q; want %q", rel.Field, wantField)
	}

	if rel.From != ingress.Original.ResourceKey() {
		t.Errorf("From = %v; want %v", rel.From, ingress.Original.ResourceKey())
	}

	wantTo := types.ResourceKey{
		GVK:  schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "ClusterIssuer"},
		Name: "letsencrypt-prod",
	}
	if rel.To != wantTo {
		t.Errorf("To = %v; want %v", rel.To, wantTo)
	}

	if rel.Details["clusterIssuer"] != "letsencrypt-prod" {
		t.Errorf("Details[clusterIssuer] = %q; want %q", rel.Details["clusterIssuer"], "letsencrypt-prod")
	}

	if rel.Details["annotation"] != "cert-manager.io/cluster-issuer" {
		t.Errorf("Details[annotation] = %q; want %q", rel.Details["annotation"], "cert-manager.io/cluster-issuer")
	}
}

// TestAnnotationDetector_CertManagerIssuer verifies that an Ingress with a
// cert-manager.io/issuer annotation pointing to an existing Issuer in the same
// namespace produces exactly one relationship of type RelationAnnotation.
func TestAnnotationDetector_CertManagerIssuer(t *testing.T) {
	const ns = "production"

	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", ns,
		map[string]string{
			"cert-manager.io/issuer": "my-issuer",
		},
		nil,
	)

	// Issuer is namespace-scoped — must share namespace with the Ingress.
	issuer := makeAnnotationResource(
		"cert-manager.io/v1", "Issuer", "my-issuer", ns,
		nil, nil,
	)

	allResources := buildAnnotationAllResources(ingress, issuer)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d: %v", len(rels), rels)
	}

	rel := rels[0]

	if rel.Type != types.RelationAnnotation {
		t.Errorf("Type = %q; want %q", rel.Type, types.RelationAnnotation)
	}

	wantField := "metadata.annotations[cert-manager.io/issuer]"
	if rel.Field != wantField {
		t.Errorf("Field = %q; want %q", rel.Field, wantField)
	}

	if rel.From != ingress.Original.ResourceKey() {
		t.Errorf("From = %v; want %v", rel.From, ingress.Original.ResourceKey())
	}

	wantTo := types.ResourceKey{
		GVK:       schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "Issuer"},
		Namespace: ns,
		Name:      "my-issuer",
	}
	if rel.To != wantTo {
		t.Errorf("To = %v; want %v", rel.To, wantTo)
	}

	if rel.Details["issuer"] != "my-issuer" {
		t.Errorf("Details[issuer] = %q; want %q", rel.Details["issuer"], "my-issuer")
	}

	if rel.Details["annotation"] != "cert-manager.io/issuer" {
		t.Errorf("Details[annotation] = %q; want %q", rel.Details["annotation"], "cert-manager.io/issuer")
	}
}

// TestAnnotationDetector_CertManagerMissing verifies that when a cert-manager
// annotation is present but the referenced issuer does not exist in allResources,
// no relationship is created.
func TestAnnotationDetector_CertManagerMissing(t *testing.T) {
	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", "default",
		map[string]string{
			"cert-manager.io/cluster-issuer": "nonexistent-issuer",
		},
		nil,
	)

	// allResources intentionally does not contain the referenced ClusterIssuer.
	allResources := buildAnnotationAllResources(ingress)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships when issuer is absent, got %d: %v", len(rels), rels)
	}
}

// TestAnnotationDetector_NoAnnotations verifies that a resource with no annotations
// produces no relationships even if potential targets exist in allResources.
func TestAnnotationDetector_NoAnnotations(t *testing.T) {
	// Resource created with no annotations map.
	deploy := makeAnnotationResource(
		"apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
	)

	clusterIssuer := makeAnnotationResource(
		"cert-manager.io/v1", "ClusterIssuer", "letsencrypt-prod", "",
		nil, nil,
	)

	allResources := buildAnnotationAllResources(deploy, clusterIssuer)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), deploy, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships for resource with no annotations, got %d: %v", len(rels), rels)
	}
}

// TestAnnotationDetector_NginxIngressAnnotation verifies that a resource carrying a
// nginx.ingress.kubernetes.io/* annotation produces a relationship of type
// RelationDeckhouse when an IngressNginxController exists in allResources.
func TestAnnotationDetector_NginxIngressAnnotation(t *testing.T) {
	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", "default",
		map[string]string{
			"nginx.ingress.kubernetes.io/rewrite-target": "/",
		},
		nil,
	)

	// IngressNginxController is a Deckhouse CRD; the detector matches on GVK.Kind only.
	nginxController := makeAnnotationResource(
		"deckhouse.io/v1", "IngressNginxController", "main", "",
		nil, nil,
	)

	allResources := buildAnnotationAllResources(ingress, nginxController)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for nginx annotation, got %d: %v", len(rels), rels)
	}

	rel := rels[0]

	if rel.Type != types.RelationDeckhouse {
		t.Errorf("Type = %q; want %q", rel.Type, types.RelationDeckhouse)
	}

	if rel.From != ingress.Original.ResourceKey() {
		t.Errorf("From = %v; want %v", rel.From, ingress.Original.ResourceKey())
	}

	if rel.To != nginxController.Original.ResourceKey() {
		t.Errorf("To = %v; want %v", rel.To, nginxController.Original.ResourceKey())
	}

	wantField := "metadata.annotations[nginx.ingress.kubernetes.io/rewrite-target]"
	if rel.Field != wantField {
		t.Errorf("Field = %q; want %q", rel.Field, wantField)
	}

	if rel.Details["annotation"] != "nginx.ingress.kubernetes.io/rewrite-target" {
		t.Errorf("Details[annotation] = %q; want %q", rel.Details["annotation"], "nginx.ingress.kubernetes.io/rewrite-target")
	}

	if rel.Details["annotationValue"] != "/" {
		t.Errorf("Details[annotationValue] = %q; want %q", rel.Details["annotationValue"], "/")
	}
}

// TestAnnotationDetector_DeckhouseAuthAnnotation verifies that a resource with a
// deckhouse.io/* annotation whose key contains "auth" produces a relationship of
// type RelationDeckhouse when a DexAuthenticator exists in the same namespace.
func TestAnnotationDetector_DeckhouseAuthAnnotation(t *testing.T) {
	const ns = "staging"

	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", ns,
		map[string]string{
			"deckhouse.io/auth-signin": "https://dex.example.com/sign_in",
		},
		nil,
	)

	dexAuth := makeAnnotationResource(
		"deckhouse.io/v1", "DexAuthenticator", "my-dex-auth", ns,
		nil, nil,
	)

	allResources := buildAnnotationAllResources(ingress, dexAuth)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for deckhouse auth annotation, got %d: %v", len(rels), rels)
	}

	rel := rels[0]

	if rel.Type != types.RelationDeckhouse {
		t.Errorf("Type = %q; want %q", rel.Type, types.RelationDeckhouse)
	}

	if rel.From != ingress.Original.ResourceKey() {
		t.Errorf("From = %v; want %v", rel.From, ingress.Original.ResourceKey())
	}

	if rel.To != dexAuth.Original.ResourceKey() {
		t.Errorf("To = %v; want %v", rel.To, dexAuth.Original.ResourceKey())
	}

	wantField := "metadata.annotations[deckhouse.io/auth-signin]"
	if rel.Field != wantField {
		t.Errorf("Field = %q; want %q", rel.Field, wantField)
	}

	if rel.Details["annotation"] != "deckhouse.io/auth-signin" {
		t.Errorf("Details[annotation] = %q; want %q", rel.Details["annotation"], "deckhouse.io/auth-signin")
	}
}

// TestAnnotationDetector_PrometheusAnnotation verifies that prometheus.io/scrape
// (and other prometheus.io/* annotations) do NOT produce any relationships because
// detectPrometheusReferences currently returns an empty slice.
func TestAnnotationDetector_PrometheusAnnotation(t *testing.T) {
	pod := makeAnnotationResource(
		"v1", "Pod", "my-pod", "default",
		map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/path":   "/metrics",
			"prometheus.io/port":   "8080",
			"prometheus.io/scheme": "http",
		},
		nil,
	)

	allResources := buildAnnotationAllResources(pod)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), pod, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships for prometheus annotations (not yet implemented), got %d: %v", len(rels), rels)
	}
}

// TestAnnotationDetector_CertManagerIssuer_WrongNamespace verifies that a
// cert-manager.io/issuer annotation does not match an Issuer in a different
// namespace, since Issuers are namespace-scoped.
func TestAnnotationDetector_CertManagerIssuer_WrongNamespace(t *testing.T) {
	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", "ns-a",
		map[string]string{
			"cert-manager.io/issuer": "my-issuer",
		},
		nil,
	)

	// Issuer lives in ns-b, Ingress is in ns-a — should not match.
	issuer := makeAnnotationResource(
		"cert-manager.io/v1", "Issuer", "my-issuer", "ns-b",
		nil, nil,
	)

	allResources := buildAnnotationAllResources(ingress, issuer)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 0 {
		t.Errorf("cross-namespace issuer: expected 0 relationships, got %d: %v", len(rels), rels)
	}
}

// TestAnnotationDetector_EmptyAnnotationValue verifies that a cert-manager annotation
// present with an empty string value does not produce a relationship (the detector
// guards against empty annotation values with the `&& clusterIssuer != ""` check).
func TestAnnotationDetector_EmptyAnnotationValue(t *testing.T) {
	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", "default",
		map[string]string{
			"cert-manager.io/cluster-issuer": "",
		},
		nil,
	)

	clusterIssuer := makeAnnotationResource(
		"cert-manager.io/v1", "ClusterIssuer", "letsencrypt-prod", "",
		nil, nil,
	)

	allResources := buildAnnotationAllResources(ingress, clusterIssuer)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 0 {
		t.Errorf("empty annotation value: expected 0 relationships, got %d: %v", len(rels), rels)
	}
}

// TestAnnotationDetector_BothCertManagerAnnotations verifies that a resource
// carrying both cert-manager.io/cluster-issuer and cert-manager.io/issuer
// annotations (with both targets present) produces exactly two relationships.
func TestAnnotationDetector_BothCertManagerAnnotations(t *testing.T) {
	const ns = "default"

	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", ns,
		map[string]string{
			"cert-manager.io/cluster-issuer": "letsencrypt-prod",
			"cert-manager.io/issuer":         "local-issuer",
		},
		nil,
	)

	clusterIssuer := makeAnnotationResource(
		"cert-manager.io/v1", "ClusterIssuer", "letsencrypt-prod", "",
		nil, nil,
	)

	issuer := makeAnnotationResource(
		"cert-manager.io/v1", "Issuer", "local-issuer", ns,
		nil, nil,
	)

	allResources := buildAnnotationAllResources(ingress, clusterIssuer, issuer)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 2 {
		t.Fatalf("expected 2 relationships (one per cert-manager annotation), got %d: %v", len(rels), rels)
	}

	for _, rel := range rels {
		if rel.Type != types.RelationAnnotation {
			t.Errorf("Type = %q; want %q", rel.Type, types.RelationAnnotation)
		}
		if rel.From != ingress.Original.ResourceKey() {
			t.Errorf("From = %v; want %v", rel.From, ingress.Original.ResourceKey())
		}
	}
}

// TestAnnotationDetector_NginxAnnotation_NoController verifies that an
// nginx.ingress.kubernetes.io/* annotation produces no relationship when no
// IngressNginxController exists in allResources.
func TestAnnotationDetector_NginxAnnotation_NoController(t *testing.T) {
	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", "default",
		map[string]string{
			"nginx.ingress.kubernetes.io/proxy-body-size": "10m",
		},
		nil,
	)

	allResources := buildAnnotationAllResources(ingress)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships when IngressNginxController is absent, got %d: %v", len(rels), rels)
	}
}

// TestAnnotationDetector_DeckhouseDexAnnotation verifies that a deckhouse.io/*
// annotation whose key contains "dex" also triggers DexAuthenticator lookup,
// since the detector checks for either "auth" or "dex" in the key.
func TestAnnotationDetector_DeckhouseDexAnnotation(t *testing.T) {
	const ns = "production"

	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", ns,
		map[string]string{
			"deckhouse.io/dex-authenticator-id": "my-app",
		},
		nil,
	)

	dexAuth := makeAnnotationResource(
		"deckhouse.io/v1", "DexAuthenticator", "my-dex-auth", ns,
		nil, nil,
	)

	allResources := buildAnnotationAllResources(ingress, dexAuth)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for deckhouse dex annotation, got %d: %v", len(rels), rels)
	}

	rel := rels[0]

	if rel.Type != types.RelationDeckhouse {
		t.Errorf("Type = %q; want %q", rel.Type, types.RelationDeckhouse)
	}

	if rel.To != dexAuth.Original.ResourceKey() {
		t.Errorf("To = %v; want %v", rel.To, dexAuth.Original.ResourceKey())
	}
}

// TestAnnotationDetector_DeckhouseAuthAnnotation_CrossNamespace verifies that a
// DexAuthenticator in a different namespace is NOT matched since the detector
// requires targetKey.Namespace == namespace of the annotated resource.
func TestAnnotationDetector_DeckhouseAuthAnnotation_CrossNamespace(t *testing.T) {
	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", "ns-a",
		map[string]string{
			"deckhouse.io/auth-signin": "https://dex.example.com/sign_in",
		},
		nil,
	)

	// DexAuthenticator is in a different namespace.
	dexAuth := makeAnnotationResource(
		"deckhouse.io/v1", "DexAuthenticator", "my-dex-auth", "ns-b",
		nil, nil,
	)

	allResources := buildAnnotationAllResources(ingress, dexAuth)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 0 {
		t.Errorf("cross-namespace DexAuthenticator: expected 0 relationships, got %d: %v", len(rels), rels)
	}
}

// TestAnnotationDetector_UnrelatedDeckhouseAnnotation verifies that a
// deckhouse.io/* annotation whose key contains neither "auth" nor "dex" produces
// no relationship even when a DexAuthenticator exists in the same namespace.
func TestAnnotationDetector_UnrelatedDeckhouseAnnotation(t *testing.T) {
	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", "default",
		map[string]string{
			"deckhouse.io/public-domain": "example.com",
		},
		nil,
	)

	dexAuth := makeAnnotationResource(
		"deckhouse.io/v1", "DexAuthenticator", "my-dex-auth", "default",
		nil, nil,
	)

	allResources := buildAnnotationAllResources(ingress, dexAuth)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 0 {
		t.Errorf("unrelated deckhouse annotation: expected 0 relationships, got %d: %v", len(rels), rels)
	}
}

// TestAnnotationDetector_CustomDependsOn verifies that the dhg.deckhouse.io/depends-on
// annotation creates a custom_dependency relationship to the named resource.
func TestAnnotationDetector_CustomDependsOn(t *testing.T) {
	deploy := makeAnnotationResource(
		"apps/v1", "Deployment", "web-app", "default",
		map[string]string{
			"dhg.deckhouse.io/depends-on": "database",
		},
		nil,
	)

	// Target resource that matches by name
	db := makeAnnotationResource(
		"apps/v1", "StatefulSet", "database", "default",
		nil, nil,
	)

	allResources := buildAnnotationAllResources(deploy, db)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), deploy, allResources)

	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationCustomDependency && rel.To.Name == "database" {
			found = true
			if rel.Details["dependsOn"] != "database" {
				t.Errorf("expected dependsOn 'database', got %q", rel.Details["dependsOn"])
			}
			if rel.Field != "metadata.annotations[dhg.deckhouse.io/depends-on]" {
				t.Errorf("expected field annotation path, got %q", rel.Field)
			}
		}
	}
	if !found {
		t.Error("expected custom_dependency relationship from Deployment to StatefulSet 'database'")
	}
}

// TestAnnotationDetector_CustomDependsOn_Missing verifies no relationship when the
// depends-on target doesn't exist in allResources.
func TestAnnotationDetector_CustomDependsOn_Missing(t *testing.T) {
	deploy := makeAnnotationResource(
		"apps/v1", "Deployment", "web-app", "default",
		map[string]string{
			"dhg.deckhouse.io/depends-on": "nonexistent",
		},
		nil,
	)

	allResources := buildAnnotationAllResources(deploy)

	d := NewAnnotationDetector()
	rels := d.Detect(context.Background(), deploy, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationCustomDependency {
			t.Errorf("expected no custom_dependency for missing target, got: %v", rel)
		}
	}
}
