package pattern_test

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/analyzer/pattern"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor/k8s"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor/value"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// realGraph runs the real processors over manifests and puts their output
// into a resource graph, the same way the CLI does before it analyzes it.
func realGraph(t *testing.T, manifests string) *types.ResourceGraph {
	t.Helper()
	registry := processor.NewRegistry()
	k8s.RegisterAll(registry)
	files := value.NewExternalFileManager()
	graph := types.NewResourceGraph()
	for _, doc := range strings.Split(manifests, "\n---\n") {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal([]byte(doc), &obj.Object); err != nil {
			t.Fatalf("parsing manifest: %v", err)
		}
		r := &types.ExtractedResource{Object: obj, Source: types.SourceFile, GVK: obj.GroupVersionKind()}
		res, err := registry.Process(processor.Context{
			Ctx: context.Background(), ChartName: "app", OutputMode: types.OutputModeUniversal,
			Namespace: obj.GetNamespace(), ExternalFileManager: files, ValueProcessor: value.DefaultProcessor(),
		}, obj)
		if err != nil {
			t.Fatalf("processing %s: %v", r.ResourceKey(), err)
		}
		graph.AddResource(&types.ProcessedResource{
			Original: r, ServiceName: res.ServiceName, TemplatePath: res.TemplatePath,
			TemplateContent: res.TemplateContent, ValuesPath: res.ValuesPath, Values: res.Values,
			Dependencies: res.Dependencies,
		})
	}
	return graph
}

func practiceByID(res *pattern.AnalysisResult, id string) *pattern.BestPractice {
	for i := range res.BestPractices {
		if res.BestPractices[i].ID == id && !res.BestPractices[i].Compliant {
			return &res.BestPractices[i]
		}
	}
	return nil
}

// Containers are a map by name in the processor output (ADR-054): the
// pattern checkers must still see them.
func TestPatternCheckers_RealProcessorOutput_BareDeployment(t *testing.T) {
	graph := realGraph(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: shop
spec:
  replicas: 1
  selector:
    matchLabels: {app: web}
  template:
    metadata:
      labels: {app: web}
    spec:
      containers:
      - name: app
        image: registry.example.com/shop/web:1.0
      - name: envoy
        image: registry.example.com/mesh/proxy:1.0
`)
	res := pattern.DefaultAnalyzer().Analyze(graph)

	for _, id := range []string{"BP-001", "BP-002", "BP-SEC-001", "BP-HA-002"} {
		bp := practiceByID(res, id)
		if bp == nil {
			t.Errorf("%s: want a non-compliant practice for the bare Deployment, got none (practices: %v)", id, res.BestPractices)
			continue
		}
		if len(bp.AffectedResources) != 1 || bp.AffectedResources[0].Name != "web" {
			t.Errorf("%s: affected = %v, want [web]", id, bp.AffectedResources)
		}
	}

	found := false
	for _, p := range res.DetectedPatterns {
		if p == pattern.PatternSidecar {
			found = true
		}
	}
	if !found {
		t.Errorf("sidecar pattern not detected for a two-container Deployment; got %v", res.DetectedPatterns)
	}
}

func TestPatternCheckers_RealProcessorOutput_HardenedDeployment(t *testing.T) {
	graph := realGraph(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: shop
spec:
  replicas: 2
  selector:
    matchLabels: {app: web}
  template:
    metadata:
      labels: {app: web}
    spec:
      containers:
      - name: app
        image: registry.example.com/shop/web:1.0
        resources:
          limits: {cpu: 500m, memory: 256Mi}
          requests: {cpu: 250m, memory: 128Mi}
        livenessProbe: {httpGet: {path: /healthz, port: 8080}}
        readinessProbe: {httpGet: {path: /ready, port: 8080}}
        securityContext:
          runAsNonRoot: true
          readOnlyRootFilesystem: true
`)
	res := pattern.DefaultAnalyzer().Analyze(graph)
	for _, id := range []string{"BP-001", "BP-002", "BP-SEC-001", "BP-SEC-002", "BP-HA-002"} {
		if bp := practiceByID(res, id); bp != nil {
			t.Errorf("%s: unexpected violation for a hardened Deployment: %s, affected %v", id, bp.Title, bp.AffectedResources)
		}
	}
}
