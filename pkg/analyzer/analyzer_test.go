package analyzer

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func makeProcessed(kind, name, namespace, serviceName string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion("v1")
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "", Version: "v1", Kind: kind},
		},
		ServiceName: serviceName,
	}
}

// stubDetector implements Detector for testing.
type stubDetector struct {
	name     string
	priority int
	results  []types.Relationship
}

func (d *stubDetector) Detect(_ context.Context, _ *types.ProcessedResource, _ map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	return d.results
}
func (d *stubDetector) Name() string { return d.name }
func (d *stubDetector) Priority() int { return d.priority }

// ── NewDefaultAnalyzer ───────────────────────────────────────────────────────

func TestNewDefaultAnalyzer(t *testing.T) {
	a := NewDefaultAnalyzer()
	if a == nil {
		t.Fatal("NewDefaultAnalyzer() returned nil")
	}
	if len(a.detectors) != 0 {
		t.Error("new analyzer should have no detectors")
	}
}

// ── AddDetector ──────────────────────────────────────────────────────────────

func TestAddDetector_SortsByPriority(t *testing.T) {
	a := NewDefaultAnalyzer()
	a.AddDetector(&stubDetector{name: "low", priority: 1})
	a.AddDetector(&stubDetector{name: "high", priority: 100})
	a.AddDetector(&stubDetector{name: "mid", priority: 50})

	if len(a.detectors) != 3 {
		t.Fatalf("got %d detectors; want 3", len(a.detectors))
	}
	if a.detectors[0].Name() != "high" {
		t.Errorf("first detector should be 'high' (priority 100), got %q", a.detectors[0].Name())
	}
	if a.detectors[1].Name() != "mid" {
		t.Errorf("second detector should be 'mid' (priority 50), got %q", a.detectors[1].Name())
	}
	if a.detectors[2].Name() != "low" {
		t.Errorf("third detector should be 'low' (priority 1), got %q", a.detectors[2].Name())
	}
}

// ── Analyze ──────────────────────────────────────────────────────────────────

func TestAnalyze_EmptyResources(t *testing.T) {
	a := NewDefaultAnalyzer()
	graph, err := a.Analyze(context.Background(), nil)
	if err != nil {
		t.Fatalf("Analyze() error: %v", err)
	}
	if len(graph.Resources) != 0 {
		t.Error("empty input should produce empty graph")
	}
	if len(graph.Groups) != 0 {
		t.Error("empty input should produce no groups")
	}
}

func TestAnalyze_AddsResourcesToGraph(t *testing.T) {
	a := NewDefaultAnalyzer()
	resources := []*types.ProcessedResource{
		makeProcessed("Deployment", "web", "default", "web"),
		makeProcessed("Service", "web-svc", "default", "web"),
	}

	graph, err := a.Analyze(context.Background(), resources)
	if err != nil {
		t.Fatalf("Analyze() error: %v", err)
	}
	if len(graph.Resources) != 2 {
		t.Errorf("len(Resources) = %d; want 2", len(graph.Resources))
	}
}

func TestAnalyze_RunsDetectors(t *testing.T) {
	a := NewDefaultAnalyzer()

	svc := makeProcessed("Service", "svc", "default", "web")
	deploy := makeProcessed("Deployment", "deploy", "default", "web")

	svcKey := svc.Original.ResourceKey()
	deployKey := deploy.Original.ResourceKey()

	a.AddDetector(&stubDetector{
		name:     "test-detector",
		priority: 10,
		results: []types.Relationship{
			{From: svcKey, To: deployKey, Type: types.RelationLabelSelector},
		},
	})

	graph, err := a.Analyze(context.Background(), []*types.ProcessedResource{svc, deploy})
	if err != nil {
		t.Fatalf("Analyze() error: %v", err)
	}
	if len(graph.Relationships) == 0 {
		t.Error("expected relationships from detector")
	}
}

func TestAnalyze_CancelledContext(t *testing.T) {
	a := NewDefaultAnalyzer()
	a.AddDetector(&stubDetector{name: "d", priority: 1})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := a.Analyze(ctx, []*types.ProcessedResource{
		makeProcessed("Deployment", "web", "default", "web"),
	})
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

// ── groupResources ───────────────────────────────────────────────────────────

func TestGroupResources_ByServiceName(t *testing.T) {
	a := NewDefaultAnalyzer()
	resources := []*types.ProcessedResource{
		makeProcessed("Deployment", "web-deploy", "default", "web"),
		makeProcessed("Service", "web-svc", "default", "web"),
		makeProcessed("ConfigMap", "api-cfg", "default", "api"),
	}

	graph, err := a.Analyze(context.Background(), resources)
	if err != nil {
		t.Fatalf("Analyze() error: %v", err)
	}

	if len(graph.Groups) != 2 {
		t.Errorf("expected 2 groups (web, api), got %d", len(graph.Groups))
	}

	groupNames := make(map[string]int)
	for _, g := range graph.Groups {
		groupNames[g.Name] = len(g.Resources)
	}
	if groupNames["web"] != 2 {
		t.Errorf("web group should have 2 resources, got %d", groupNames["web"])
	}
	if groupNames["api"] != 1 {
		t.Errorf("api group should have 1 resource, got %d", groupNames["api"])
	}
}

func TestGroupResources_OrphansGetStandaloneGroup(t *testing.T) {
	a := NewDefaultAnalyzer()
	resources := []*types.ProcessedResource{
		makeProcessed("ConfigMap", "orphan-cfg", "default", ""),
	}

	graph, err := a.Analyze(context.Background(), resources)
	if err != nil {
		t.Fatalf("Analyze() error: %v", err)
	}

	// Orphan should get its own standalone group using the resource name
	if len(graph.Groups) != 1 {
		t.Errorf("expected 1 group for orphan, got %d", len(graph.Groups))
	}
	if graph.Groups[0].Name != "orphan-cfg" {
		t.Errorf("orphan group name = %q; want orphan-cfg", graph.Groups[0].Name)
	}
}

func TestGroupResources_ByRelationship(t *testing.T) {
	a := NewDefaultAnalyzer()

	deploy := makeProcessed("Deployment", "app", "default", "web")
	cm := makeProcessed("ConfigMap", "app-config", "default", "")

	deployKey := deploy.Original.ResourceKey()
	cmKey := cm.Original.ResourceKey()

	// Detector that creates relationship from configmap to deployment
	a.AddDetector(&stubDetector{
		name:     "rel",
		priority: 10,
		results: []types.Relationship{
			{From: cmKey, To: deployKey, Type: types.RelationEnvFrom},
		},
	})

	graph, err := a.Analyze(context.Background(), []*types.ProcessedResource{deploy, cm})
	if err != nil {
		t.Fatalf("Analyze() error: %v", err)
	}

	// ConfigMap should be grouped with "web" via relationship
	for _, g := range graph.Groups {
		if g.Name == "web" {
			if len(g.Resources) != 2 {
				t.Errorf("web group should have 2 resources (deploy + configmap), got %d", len(g.Resources))
			}
			return
		}
	}
	t.Error("expected 'web' group containing both resources")
}

// ── findRelatedService ───────────────────────────────────────────────────────

func TestFindRelatedService_Outgoing(t *testing.T) {
	a := NewDefaultAnalyzer()
	graph := types.NewResourceGraph()

	svc := makeProcessed("Service", "web-svc", "default", "web")
	cm := makeProcessed("ConfigMap", "cm", "default", "")

	graph.AddResource(svc)
	graph.AddResource(cm)

	svcKey := svc.Original.ResourceKey()
	cmKey := cm.Original.ResourceKey()

	graph.AddRelationship(types.Relationship{
		From: cmKey, To: svcKey, Type: types.RelationNameReference,
	})

	grouped := map[string]bool{svcKey.String(): true}
	result := a.findRelatedService(cmKey, graph, grouped)
	if result != "web" {
		t.Errorf("findRelatedService = %q; want web", result)
	}
}

func TestFindRelatedService_Incoming(t *testing.T) {
	a := NewDefaultAnalyzer()
	graph := types.NewResourceGraph()

	deploy := makeProcessed("Deployment", "app", "default", "backend")
	secret := makeProcessed("Secret", "app-secret", "default", "")

	graph.AddResource(deploy)
	graph.AddResource(secret)

	deployKey := deploy.Original.ResourceKey()
	secretKey := secret.Original.ResourceKey()

	graph.AddRelationship(types.Relationship{
		From: deployKey, To: secretKey, Type: types.RelationEnvFrom,
	})

	grouped := map[string]bool{deployKey.String(): true}
	result := a.findRelatedService(secretKey, graph, grouped)
	if result != "backend" {
		t.Errorf("findRelatedService = %q; want backend", result)
	}
}

func TestFindRelatedService_NoRelation(t *testing.T) {
	a := NewDefaultAnalyzer()
	graph := types.NewResourceGraph()

	cm := makeProcessed("ConfigMap", "orphan", "default", "")
	graph.AddResource(cm)
	cmKey := cm.Original.ResourceKey()

	result := a.findRelatedService(cmKey, graph, map[string]bool{})
	if result != "" {
		t.Errorf("findRelatedService = %q; want empty", result)
	}
}
