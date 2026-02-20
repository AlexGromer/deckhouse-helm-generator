package types

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func makeResource(kind, name, namespace string) *ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion("v1")
	return &ProcessedResource{
		Original: &ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "", Version: "v1", Kind: kind},
		},
	}
}

func makeRelationship(from, to ResourceKey, relType RelationshipType) Relationship {
	return Relationship{From: from, To: to, Type: relType}
}

// ── ResourceKey.String ────────────────────────────────────────────────────────

func TestResourceKey_String_WithNamespace(t *testing.T) {
	key := ResourceKey{
		GVK:       schema.GroupVersionKind{Kind: "Deployment"},
		Namespace: "default",
		Name:      "web-app",
	}
	want := "Deployment/default/web-app"
	if got := key.String(); got != want {
		t.Errorf("String() = %q; want %q", got, want)
	}
}

func TestResourceKey_String_WithoutNamespace(t *testing.T) {
	key := ResourceKey{
		GVK:  schema.GroupVersionKind{Kind: "ClusterRole"},
		Name: "admin",
	}
	want := "ClusterRole/admin"
	if got := key.String(); got != want {
		t.Errorf("String() = %q; want %q", got, want)
	}
}

// ── ExtractedResource.ResourceKey ─────────────────────────────────────────────

func TestExtractedResource_ResourceKey(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("nginx")
	obj.SetNamespace("prod")
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	er := &ExtractedResource{Object: obj, GVK: gvk}
	key := er.ResourceKey()

	if key.GVK != gvk {
		t.Errorf("GVK = %v; want %v", key.GVK, gvk)
	}
	if key.Namespace != "prod" {
		t.Errorf("Namespace = %q; want %q", key.Namespace, "prod")
	}
	if key.Name != "nginx" {
		t.Errorf("Name = %q; want %q", key.Name, "nginx")
	}
}

// ── NewResourceGraph ──────────────────────────────────────────────────────────

func TestNewResourceGraph(t *testing.T) {
	g := NewResourceGraph()
	if g == nil {
		t.Fatal("NewResourceGraph() returned nil")
	}
	if g.Resources == nil || len(g.Resources) != 0 {
		t.Error("Resources not initialized or not empty")
	}
	if g.Relationships == nil || len(g.Relationships) != 0 {
		t.Error("Relationships not initialized or not empty")
	}
	if g.Groups == nil || len(g.Groups) != 0 {
		t.Error("Groups not initialized or not empty")
	}
	if g.Orphans == nil || len(g.Orphans) != 0 {
		t.Error("Orphans not initialized or not empty")
	}
}

// ── ResourceGraph.AddResource ─────────────────────────────────────────────────

func TestResourceGraph_AddResource(t *testing.T) {
	g := NewResourceGraph()
	r := makeResource("Deployment", "web-app", "default")
	g.AddResource(r)

	if len(g.Resources) != 1 {
		t.Fatalf("len(Resources) = %d; want 1", len(g.Resources))
	}
	key := r.Original.ResourceKey()
	if got, ok := g.Resources[key]; !ok || got != r {
		t.Error("resource not found or pointer mismatch")
	}
}

func TestResourceGraph_AddResource_Overwrite(t *testing.T) {
	g := NewResourceGraph()
	r1 := makeResource("ConfigMap", "cfg", "default")
	r2 := makeResource("ConfigMap", "cfg", "default")
	r2.ServiceName = "overwritten"

	g.AddResource(r1)
	g.AddResource(r2)

	if len(g.Resources) != 1 {
		t.Fatalf("len(Resources) = %d; want 1", len(g.Resources))
	}
	key := r1.Original.ResourceKey()
	if g.Resources[key].ServiceName != "overwritten" {
		t.Error("second AddResource should overwrite")
	}
}

// ── ResourceGraph.AddRelationship ─────────────────────────────────────────────

func TestResourceGraph_AddRelationship(t *testing.T) {
	g := NewResourceGraph()
	from := makeResource("Service", "svc", "default")
	to := makeResource("Deployment", "app", "default")

	fromKey := from.Original.ResourceKey()
	toKey := to.Original.ResourceKey()

	rel := makeRelationship(fromKey, toKey, RelationLabelSelector)
	g.AddRelationship(rel)

	if len(g.Relationships) != 1 {
		t.Errorf("len(Relationships) = %d; want 1", len(g.Relationships))
	}
	if len(g.GetRelationshipsFrom(fromKey)) != 1 {
		t.Error("relFrom index not updated")
	}
	if len(g.GetRelationshipsTo(toKey)) != 1 {
		t.Error("relTo index not updated")
	}
}

// ── ResourceGraph.GetResourceByKey ────────────────────────────────────────────

func TestResourceGraph_GetResourceByKey(t *testing.T) {
	g := NewResourceGraph()
	r := makeResource("Service", "svc", "default")
	g.AddResource(r)

	key := r.Original.ResourceKey()
	got, ok := g.GetResourceByKey(key)
	if !ok || got != r {
		t.Error("GetResourceByKey failed for existing resource")
	}

	_, ok = g.GetResourceByKey(ResourceKey{GVK: schema.GroupVersionKind{Kind: "Ghost"}, Name: "x"})
	if ok {
		t.Error("GetResourceByKey should return false for missing key")
	}
}

// ── ResourceGraph.GetRelationshipsFrom/To ─────────────────────────────────────

func TestResourceGraph_GetRelationshipsFrom_Directional(t *testing.T) {
	g := NewResourceGraph()
	src := makeResource("Ingress", "ing", "default")
	dst := makeResource("Service", "svc", "default")
	srcKey := src.Original.ResourceKey()
	dstKey := dst.Original.ResourceKey()

	g.AddRelationship(makeRelationship(srcKey, dstKey, RelationNameReference))

	if len(g.GetRelationshipsFrom(srcKey)) != 1 {
		t.Error("expected 1 rel from src")
	}
	if len(g.GetRelationshipsFrom(dstKey)) != 0 {
		t.Error("expected 0 rels from dst")
	}
	if len(g.GetRelationshipsTo(dstKey)) != 1 {
		t.Error("expected 1 rel to dst")
	}
	if len(g.GetRelationshipsTo(srcKey)) != 0 {
		t.Error("expected 0 rels to src")
	}
}

// ── ResourceGraph.GetResourcesByKind ──────────────────────────────────────────

func TestResourceGraph_GetResourcesByKind(t *testing.T) {
	g := NewResourceGraph()
	g.AddResource(makeResource("Deployment", "d1", "default"))
	g.AddResource(makeResource("Deployment", "d2", "default"))
	g.AddResource(makeResource("Service", "s1", "default"))

	deploys := g.GetResourcesByKind("Deployment")
	if len(deploys) != 2 {
		t.Errorf("GetResourcesByKind(Deployment) = %d; want 2", len(deploys))
	}
	if len(g.GetResourcesByKind("Ingress")) != 0 {
		t.Error("expected 0 for missing kind")
	}
}

// ── ResourceGraph.AddGroup / AddOrphan ────────────────────────────────────────

func TestResourceGraph_AddGroup(t *testing.T) {
	g := NewResourceGraph()
	grp := &ResourceGroup{Name: "web", Namespace: "default"}
	g.AddGroup(grp)

	if len(g.Groups) != 1 || g.Groups[0] != grp {
		t.Error("AddGroup failed")
	}
}

func TestResourceGraph_AddOrphan(t *testing.T) {
	g := NewResourceGraph()
	r := makeResource("Secret", "orphan", "default")
	g.AddOrphan(r)

	if len(g.Orphans) != 1 || g.Orphans[0] != r {
		t.Error("AddOrphan failed")
	}
}

// ── Constants ─────────────────────────────────────────────────────────────────

func TestSource_Constants(t *testing.T) {
	tests := []struct{ c Source; w string }{
		{SourceCluster, "cluster"},
		{SourceFile, "file"},
		{SourceGitOps, "gitops"},
	}
	for _, tc := range tests {
		if string(tc.c) != tc.w {
			t.Errorf("%q != %q", tc.c, tc.w)
		}
	}
}

func TestOutputMode_Constants(t *testing.T) {
	tests := []struct{ c OutputMode; w string }{
		{OutputModeUniversal, "universal"},
		{OutputModeSeparate, "separate"},
		{OutputModeLibrary, "library"},
		{OutputModeUmbrella, "umbrella"},
	}
	for _, tc := range tests {
		if string(tc.c) != tc.w {
			t.Errorf("%q != %q", tc.c, tc.w)
		}
	}
}

func TestRelationshipType_Constants(t *testing.T) {
	tests := []struct{ c RelationshipType; w string }{
		{RelationLabelSelector, "label_selector"},
		{RelationNameReference, "name_reference"},
		{RelationVolumeMount, "volume_mount"},
		{RelationEnvFrom, "env_from"},
		{RelationEnvValueFrom, "env_value_from"},
		{RelationAnnotation, "annotation"},
		{RelationServiceAccount, "service_account"},
		{RelationOwnerReference, "owner_reference"},
		{RelationImagePullSecret, "image_pull_secret"},
		{RelationClusterRoleBinding, "cluster_role_binding"},
		{RelationRoleBinding, "role_binding"},
		{RelationPVC, "pvc"},
		{RelationIngressClass, "ingress_class"},
		{RelationServiceMonitor, "service_monitor"},
		{RelationDeckhouse, "deckhouse"},
		{RelationGatewayRoute, "gateway_route"},
		{RelationScaleTarget, "scale_target"},
		{RelationStorageClass, "storage_class"},
		{RelationCustomDependency, "custom_dependency"},
	}
	for _, tc := range tests {
		if string(tc.c) != tc.w {
			t.Errorf("%q != %q", tc.c, tc.w)
		}
	}
}
