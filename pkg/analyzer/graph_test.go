package analyzer

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test helpers
// ============================================================

func makeTestResource(kind, name, namespace, serviceName string) *types.ProcessedResource {
	gvk := schema.GroupVersionKind{Version: "v1", Kind: kind}
	if kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" {
		gvk = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind}
	}

	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": gvk.Group + "/" + gvk.Version,
					"kind":       kind,
					"metadata": map[string]interface{}{
						"name":      name,
						"namespace": namespace,
					},
				},
			},
			GVK: gvk,
		},
		ServiceName: serviceName,
	}
}

func buildTestGraph(resources []*types.ProcessedResource, relationships []types.Relationship) *types.ResourceGraph {
	graph := types.NewResourceGraph()
	for _, r := range resources {
		graph.AddResource(r)
	}
	for _, rel := range relationships {
		graph.AddRelationship(rel)
	}
	return graph
}

// ============================================================
// 5.3.1: DOT/Graphviz graph
// ============================================================

func TestGenerateDOTGraph_NilGraph(t *testing.T) {
	result := GenerateDOTGraph(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil graph, got %q", result)
	}
}

func TestGenerateDOTGraph_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()
	result := GenerateDOTGraph(graph)

	testutil.AssertContains(t, result, "digraph resources", "should be a digraph")
	testutil.AssertContains(t, result, "rankdir=LR", "should have LR direction")
}

func TestGenerateDOTGraph_SingleNode(t *testing.T) {
	deploy := makeTestResource("Deployment", "web", "default", "web")
	graph := buildTestGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateDOTGraph(graph)

	testutil.AssertContains(t, result, "Deployment", "should contain kind")
	testutil.AssertContains(t, result, "web", "should contain name")
	testutil.AssertContains(t, result, "#4A90D9", "Deployment should have blue color")
}

func TestGenerateDOTGraph_NodesColored(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeTestResource("Deployment", "api", "default", "api"),
		makeTestResource("Service", "api-svc", "default", "api"),
		makeTestResource("ConfigMap", "api-config", "default", "api"),
		makeTestResource("Secret", "api-secret", "default", "api"),
	}

	graph := buildTestGraph(resources, nil)
	result := GenerateDOTGraph(graph)

	testutil.AssertContains(t, result, "#4A90D9", "Deployment color")
	testutil.AssertContains(t, result, "#50C878", "Service color")
	testutil.AssertContains(t, result, "#87CEEB", "ConfigMap color")
	testutil.AssertContains(t, result, "#FF6B6B", "Secret color")
}

func TestGenerateDOTGraph_UnknownKindDefaultColor(t *testing.T) {
	res := makeTestResource("CustomWidget", "w1", "default", "w1")
	// Override GVK for custom kind
	res.Original.GVK = schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "CustomWidget"}
	res.Original.Object.Object["kind"] = "CustomWidget"

	graph := buildTestGraph([]*types.ProcessedResource{res}, nil)
	result := GenerateDOTGraph(graph)

	testutil.AssertContains(t, result, "#C0C0C0", "unknown kind should get default gray color")
}

func TestGenerateDOTGraph_Edges(t *testing.T) {
	deploy := makeTestResource("Deployment", "web", "default", "web")
	svc := makeTestResource("Service", "web-svc", "default", "web")

	deployKey := deploy.Original.ResourceKey()
	svcKey := svc.Original.ResourceKey()

	rels := []types.Relationship{
		{
			From: svcKey,
			To:   deployKey,
			Type: types.RelationLabelSelector,
		},
	}

	graph := buildTestGraph([]*types.ProcessedResource{deploy, svc}, rels)
	result := GenerateDOTGraph(graph)

	testutil.AssertContains(t, result, "->", "should have edge")
	testutil.AssertContains(t, result, "label_selector", "should have relationship label")
	testutil.AssertContains(t, result, "style=solid", "label_selector should be solid")
}

func TestGenerateDOTGraph_EdgeStyles(t *testing.T) {
	deploy := makeTestResource("Deployment", "app", "default", "app")
	cm := makeTestResource("ConfigMap", "app-config", "default", "app")

	deployKey := deploy.Original.ResourceKey()
	cmKey := cm.Original.ResourceKey()

	rels := []types.Relationship{
		{From: deployKey, To: cmKey, Type: types.RelationVolumeMount},
	}

	graph := buildTestGraph([]*types.ProcessedResource{deploy, cm}, rels)
	result := GenerateDOTGraph(graph)

	testutil.AssertContains(t, result, "style=dotted", "volume_mount should be dotted")
}

func TestGenerateDOTGraph_NamespaceInLabel(t *testing.T) {
	res := makeTestResource("Deployment", "api", "production", "api")
	graph := buildTestGraph([]*types.ProcessedResource{res}, nil)
	result := GenerateDOTGraph(graph)

	testutil.AssertContains(t, result, "production", "should contain namespace")
}

func TestGenerateDOTGraph_DeterministicOutput(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeTestResource("Deployment", "b-app", "default", "b"),
		makeTestResource("Service", "a-svc", "default", "a"),
		makeTestResource("ConfigMap", "c-config", "default", "c"),
	}

	graph := buildTestGraph(resources, nil)
	result1 := GenerateDOTGraph(graph)
	result2 := GenerateDOTGraph(graph)

	testutil.AssertEqual(t, result1, result2, "DOT output should be deterministic")
}

// ============================================================
// 5.3.2: Circular dependency detection
// ============================================================

func TestDetectCircularDependencies_NilGraph(t *testing.T) {
	err := DetectCircularDependencies(nil)
	if err != nil {
		t.Errorf("Expected nil error for nil graph, got %v", err)
	}
}

func TestDetectCircularDependencies_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()
	err := DetectCircularDependencies(graph)
	if err != nil {
		t.Errorf("Expected nil error for empty graph, got %v", err)
	}
}

func TestDetectCircularDependencies_NoCycle(t *testing.T) {
	a := makeTestResource("Deployment", "a", "default", "a")
	b := makeTestResource("Service", "b", "default", "b")
	c := makeTestResource("ConfigMap", "c", "default", "c")

	aKey := a.Original.ResourceKey()
	bKey := b.Original.ResourceKey()
	cKey := c.Original.ResourceKey()

	rels := []types.Relationship{
		{From: aKey, To: bKey, Type: types.RelationNameReference},
		{From: bKey, To: cKey, Type: types.RelationNameReference},
	}

	graph := buildTestGraph([]*types.ProcessedResource{a, b, c}, rels)
	err := DetectCircularDependencies(graph)
	if err != nil {
		t.Errorf("Expected no cycle, got %v", err)
	}
}

func TestDetectCircularDependencies_SimpleCycle(t *testing.T) {
	a := makeTestResource("Deployment", "a", "default", "a")
	b := makeTestResource("Service", "b", "default", "b")

	aKey := a.Original.ResourceKey()
	bKey := b.Original.ResourceKey()

	rels := []types.Relationship{
		{From: aKey, To: bKey, Type: types.RelationNameReference},
		{From: bKey, To: aKey, Type: types.RelationNameReference},
	}

	graph := buildTestGraph([]*types.ProcessedResource{a, b}, rels)
	err := DetectCircularDependencies(graph)
	if err == nil {
		t.Fatal("Expected cycle error")
	}
	testutil.AssertContains(t, err.Error(), "circular dependency detected", "error message")
}

func TestDetectCircularDependencies_ThreeNodeCycle(t *testing.T) {
	a := makeTestResource("Deployment", "a", "default", "a")
	b := makeTestResource("Service", "b", "default", "b")
	c := makeTestResource("ConfigMap", "c", "default", "c")

	aKey := a.Original.ResourceKey()
	bKey := b.Original.ResourceKey()
	cKey := c.Original.ResourceKey()

	rels := []types.Relationship{
		{From: aKey, To: bKey, Type: types.RelationNameReference},
		{From: bKey, To: cKey, Type: types.RelationNameReference},
		{From: cKey, To: aKey, Type: types.RelationNameReference},
	}

	graph := buildTestGraph([]*types.ProcessedResource{a, b, c}, rels)
	err := DetectCircularDependencies(graph)
	if err == nil {
		t.Fatal("Expected cycle error")
	}
	testutil.AssertContains(t, err.Error(), "circular dependency detected", "error message")
	// Should contain the cycle path with ->
	testutil.AssertContains(t, err.Error(), "->", "should show cycle path")
}

func TestDetectCircularDependencies_SelfLoop(t *testing.T) {
	a := makeTestResource("Deployment", "a", "default", "a")
	aKey := a.Original.ResourceKey()

	rels := []types.Relationship{
		{From: aKey, To: aKey, Type: types.RelationNameReference},
	}

	graph := buildTestGraph([]*types.ProcessedResource{a}, rels)
	err := DetectCircularDependencies(graph)
	if err == nil {
		t.Fatal("Expected cycle error for self-loop")
	}
	testutil.AssertContains(t, err.Error(), "circular dependency detected", "error message")
}

func TestDetectCircularDependencies_DisconnectedComponents(t *testing.T) {
	a := makeTestResource("Deployment", "a", "default", "a")
	b := makeTestResource("Service", "b", "default", "b")
	c := makeTestResource("ConfigMap", "c", "ns2", "c")
	d := makeTestResource("Secret", "d", "ns2", "d")

	aKey := a.Original.ResourceKey()
	bKey := b.Original.ResourceKey()
	cKey := c.Original.ResourceKey()
	dKey := d.Original.ResourceKey()

	rels := []types.Relationship{
		{From: aKey, To: bKey, Type: types.RelationNameReference},
		{From: cKey, To: dKey, Type: types.RelationNameReference},
	}

	graph := buildTestGraph([]*types.ProcessedResource{a, b, c, d}, rels)
	err := DetectCircularDependencies(graph)
	if err != nil {
		t.Errorf("Expected no cycle in disconnected graph, got %v", err)
	}
}

func TestDetectCircularDependencies_ExternalRefIgnored(t *testing.T) {
	a := makeTestResource("Deployment", "a", "default", "a")
	aKey := a.Original.ResourceKey()

	// Edge to a resource not in the graph
	nonExistentKey := types.ResourceKey{
		GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Service"},
		Namespace: "default",
		Name:      "external-svc",
	}

	rels := []types.Relationship{
		{From: aKey, To: nonExistentKey, Type: types.RelationNameReference},
	}

	graph := buildTestGraph([]*types.ProcessedResource{a}, rels)
	err := DetectCircularDependencies(graph)
	if err != nil {
		t.Errorf("Expected no cycle when edge targets external resource, got %v", err)
	}
}

// ============================================================
// 5.3.3: Decomposition recommendations
// ============================================================

func TestAnalyzeDecomposition_NilGraph(t *testing.T) {
	rec := AnalyzeDecomposition(nil)
	if rec == nil {
		t.Fatal("Expected non-nil recommendation")
	}
	testutil.AssertEqual(t, float64(0), rec.CouplingScore, "coupling score")
	testutil.AssertContains(t, rec.Reason, "no decomposition needed", "reason")
}

func TestAnalyzeDecomposition_SingleGroup(t *testing.T) {
	a := makeTestResource("Deployment", "api", "default", "api")
	b := makeTestResource("Service", "api-svc", "default", "api")

	graph := buildTestGraph([]*types.ProcessedResource{a, b}, nil)
	graph.AddGroup(&types.ResourceGroup{
		Name:      "api",
		Resources: []*types.ProcessedResource{a, b},
	})

	rec := AnalyzeDecomposition(graph)
	testutil.AssertEqual(t, float64(0), rec.CouplingScore, "single group coupling")
	testutil.AssertContains(t, rec.Reason, "no decomposition needed", "reason")
}

func TestAnalyzeDecomposition_TwoGroups_NoCoupling(t *testing.T) {
	a := makeTestResource("Deployment", "api", "default", "api")
	b := makeTestResource("Service", "api-svc", "default", "api")
	c := makeTestResource("Deployment", "worker", "default", "worker")
	d := makeTestResource("ConfigMap", "worker-config", "default", "worker")

	aKey := a.Original.ResourceKey()
	bKey := b.Original.ResourceKey()
	cKey := c.Original.ResourceKey()
	dKey := d.Original.ResourceKey()

	rels := []types.Relationship{
		{From: bKey, To: aKey, Type: types.RelationLabelSelector},
		{From: cKey, To: dKey, Type: types.RelationVolumeMount},
	}

	graph := buildTestGraph([]*types.ProcessedResource{a, b, c, d}, rels)
	graph.AddGroup(&types.ResourceGroup{
		Name:      "api",
		Resources: []*types.ProcessedResource{a, b},
	})
	graph.AddGroup(&types.ResourceGroup{
		Name:      "worker",
		Resources: []*types.ProcessedResource{c, d},
	})

	rec := AnalyzeDecomposition(graph)
	testutil.AssertEqual(t, float64(0), rec.CouplingScore, "no inter-group edges")
	if len(rec.SuggestedGroups) != 2 {
		t.Fatalf("Expected 2 groups, got %d", len(rec.SuggestedGroups))
	}
}

func TestAnalyzeDecomposition_TwoGroups_WithCoupling(t *testing.T) {
	a := makeTestResource("Deployment", "api", "default", "api")
	b := makeTestResource("Service", "api-svc", "default", "api")
	c := makeTestResource("Deployment", "worker", "default", "worker")

	aKey := a.Original.ResourceKey()
	bKey := b.Original.ResourceKey()
	cKey := c.Original.ResourceKey()

	rels := []types.Relationship{
		{From: bKey, To: aKey, Type: types.RelationLabelSelector},     // intra
		{From: cKey, To: bKey, Type: types.RelationNameReference},      // inter
	}

	graph := buildTestGraph([]*types.ProcessedResource{a, b, c}, rels)
	graph.AddGroup(&types.ResourceGroup{
		Name:      "api",
		Resources: []*types.ProcessedResource{a, b},
	})
	graph.AddGroup(&types.ResourceGroup{
		Name:      "worker",
		Resources: []*types.ProcessedResource{c},
	})

	rec := AnalyzeDecomposition(graph)
	if rec.CouplingScore <= 0 {
		t.Error("Expected positive coupling score with inter-group edge")
	}
	if rec.CouplingScore >= 1 {
		t.Error("Expected coupling score < 1")
	}
	// 1 inter / (1 inter + 1 intra) = 0.5
	testutil.AssertEqual(t, 0.5, rec.CouplingScore, "coupling score")
	testutil.AssertContains(t, rec.Reason, "coupling", "should note coupling")
}

func TestAnalyzeDecomposition_HighCoupling(t *testing.T) {
	a := makeTestResource("Deployment", "a", "default", "group-a")
	b := makeTestResource("Service", "b", "default", "group-b")

	aKey := a.Original.ResourceKey()
	bKey := b.Original.ResourceKey()

	// All edges are inter-group
	rels := []types.Relationship{
		{From: aKey, To: bKey, Type: types.RelationNameReference},
		{From: bKey, To: aKey, Type: types.RelationNameReference},
	}

	graph := buildTestGraph([]*types.ProcessedResource{a, b}, rels)
	graph.AddGroup(&types.ResourceGroup{
		Name:      "group-a",
		Resources: []*types.ProcessedResource{a},
	})
	graph.AddGroup(&types.ResourceGroup{
		Name:      "group-b",
		Resources: []*types.ProcessedResource{b},
	})

	rec := AnalyzeDecomposition(graph)
	testutil.AssertEqual(t, float64(1), rec.CouplingScore, "all inter-group = 1.0")
	testutil.AssertContains(t, rec.Reason, "high coupling", "should warn about high coupling")
}

func TestAnalyzeDecomposition_ManyGroups(t *testing.T) {
	var resources []*types.ProcessedResource
	graph := types.NewResourceGraph()

	// Create 6 isolated groups
	for i := 0; i < 6; i++ {
		name := strings.Repeat(string(rune('a'+i)), 1) + "-svc"
		r := makeTestResource("Deployment", name, "default", name)
		resources = append(resources, r)
		graph.AddResource(r)
		graph.AddGroup(&types.ResourceGroup{
			Name:      name,
			Resources: []*types.ProcessedResource{r},
		})
	}

	rec := AnalyzeDecomposition(graph)
	testutil.AssertContains(t, rec.Reason, "many groups", "should note many groups")
	testutil.AssertContains(t, rec.Reason, "separate charts", "should suggest separate charts")
}

func TestAnalyzeDecomposition_GroupResourceKeys(t *testing.T) {
	a := makeTestResource("Deployment", "api", "default", "api")
	b := makeTestResource("Service", "api-svc", "default", "api")

	graph := buildTestGraph([]*types.ProcessedResource{a, b}, nil)
	graph.AddGroup(&types.ResourceGroup{
		Name:      "api",
		Resources: []*types.ProcessedResource{a, b},
	})
	graph.AddGroup(&types.ResourceGroup{
		Name:      "worker",
		Resources: []*types.ProcessedResource{},
	})

	rec := AnalyzeDecomposition(graph)
	if len(rec.SuggestedGroups) != 2 {
		t.Fatalf("Expected 2 groups, got %d", len(rec.SuggestedGroups))
	}

	// Find the api group
	var apiGroup *RecommendedGroup
	for i := range rec.SuggestedGroups {
		if rec.SuggestedGroups[i].Name == "api" {
			apiGroup = &rec.SuggestedGroups[i]
			break
		}
	}
	if apiGroup == nil {
		t.Fatal("Expected api group in recommendations")
	}
	if len(apiGroup.Resources) != 2 {
		t.Errorf("Expected 2 resources in api group, got %d", len(apiGroup.Resources))
	}
}
