package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// cpKey builds a resource key for connection-policy graph fixtures.
func cpKey(kind, name string) types.ResourceKey {
	return types.ResourceKey{
		GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind},
		Namespace: "ns",
		Name:      name,
	}
}

// cpGraph builds a graph with a Deployment per service (selector labels
// app.kubernetes.io/name=<service>) and the given connections.
func cpGraph(services []string, conns ...types.Connection) *types.ResourceGraph {
	g := types.NewResourceGraph()
	for _, s := range services {
		g.Resources[cpKey("Deployment", s)] = &types.ProcessedResource{
			ServiceName: s,
			Values: map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{"app.kubernetes.io/name": s},
				},
			},
		}
	}
	g.Connections = conns
	return g
}

func cpEdge(from, to string, port int32) types.Connection {
	toKey := cpKey("Deployment", to)
	return types.Connection{
		From: cpKey("Deployment", from),
		To:   types.ConnectionTarget{Resource: &toKey},
		Port: port,
	}
}

func applyCP(t *testing.T, chartName string, g *types.ResourceGraph) (*types.GeneratedChart, []string) {
	t.Helper()
	var notes []string
	chart := &types.GeneratedChart{Name: chartName, ValuesYAML: "image: x\n", Templates: map[string]string{}}
	res, err := applyConnectionPolicies(chart, FeatureContext{Graph: g, Notes: &notes})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return res, notes
}

func TestConnectionPolicies_NoEdges_NoDenyIngress_Disabled(t *testing.T) {
	res, notes := applyCP(t, "web", cpGraph([]string{"web"}))
	policy := res.Templates["templates/connection-policy.yaml"]

	if strings.Contains(policy, "- Ingress") {
		t.Errorf("policy without known ingress sources must not deny ingress:\n%s", policy)
	}
	if !strings.Contains(policy, "(.Values.connectionPolicies).enabled") {
		t.Errorf("policy must be gated by connectionPolicies.enabled:\n%s", policy)
	}
	if !strings.Contains(res.ValuesYAML, "connectionPolicies:") || !strings.Contains(res.ValuesYAML, "enabled: false") {
		t.Errorf("connectionPolicies.enabled must default to false without edges:\n%s", res.ValuesYAML)
	}
	if len(notes) == 0 {
		t.Error("expected a note explaining that no connections were found")
	}
}

func TestConnectionPolicies_EgressOnly_NoDenyIngress_Enabled(t *testing.T) {
	g := cpGraph([]string{"web", "db"}, cpEdge("web", "db", 5432))
	res, notes := applyCP(t, "web", g)
	policy := res.Templates["templates/connection-policy.yaml"]

	if strings.Contains(policy, "- Ingress") || strings.Contains(policy, "\n  ingress:") {
		t.Errorf("egress-only chart must not restrict ingress:\n%s", policy)
	}
	if !strings.Contains(policy, "- Egress") || !strings.Contains(policy, "port: 5432") {
		t.Errorf("expected egress rule to db:\n%s", policy)
	}
	if !strings.Contains(res.ValuesYAML, "enabled: true") {
		t.Errorf("connectionPolicies.enabled must default to true when edges exist:\n%s", res.ValuesYAML)
	}
	found := false
	for _, n := range notes {
		if strings.Contains(n, "ingress") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a note about unrestricted ingress, got %v", notes)
	}
}

func TestConnectionPolicies_IngressSource_RestrictsIngress(t *testing.T) {
	g := cpGraph([]string{"web", "api"}, cpEdge("web", "api", 8080), cpEdge("api", "web", 80))
	res, _ := applyCP(t, "api", g)
	policy := res.Templates["templates/connection-policy.yaml"]

	if !strings.Contains(policy, "- Ingress") || !strings.Contains(policy, "  ingress:\n  - from:") {
		t.Errorf("expected ingress rule from web:\n%s", policy)
	}
	// The egress rule to web must stay under egress even though the ingress
	// edge is seen first in the graph.
	egress := strings.Index(policy, "  egress:")
	ingress := strings.Index(policy, "  ingress:")
	if egress < 0 || ingress < 0 || egress > ingress {
		t.Errorf("egress section must precede ingress section:\n%s", policy)
	}
	if strings.Count(policy, "  egress:") != 1 || strings.Count(policy, "  ingress:") != 1 {
		t.Errorf("each rule section must appear once:\n%s", policy)
	}
	if strings.Count(policy, "port: 80\n") != 1 || strings.Index(policy, "port: 80\n") > ingress {
		t.Errorf("egress port 80 must be listed under egress:\n%s", policy)
	}
}

// Ingress-only: the policy must not list Egress, or the pods lose all
// outbound traffic except DNS (external APIs, S3, ...).
func TestConnectionPolicies_IngressOnly_NoEgressRestriction(t *testing.T) {
	g := cpGraph([]string{"web", "api"}, cpEdge("web", "api", 8080))
	res, notes := applyCP(t, "api", g)
	policy := res.Templates["templates/connection-policy.yaml"]

	if !strings.Contains(policy, "- Ingress") || !strings.Contains(policy, "  ingress:\n  - from:") {
		t.Errorf("expected ingress rule from web:\n%s", policy)
	}
	if strings.Contains(policy, "- Egress") || strings.Contains(policy, "  egress:") || strings.Contains(policy, "port: 53") {
		t.Errorf("ingress-only chart must not restrict egress:\n%s", policy)
	}
	found := false
	for _, n := range notes {
		if strings.Contains(n, "egress") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a note about unrestricted egress, got %v", notes)
	}
}

func TestConnectionPolicies_PodSelectorUsesWorkloadLabels(t *testing.T) {
	g := types.NewResourceGraph()
	g.Resources[cpKey("Deployment", "web")] = &types.ProcessedResource{
		ServiceName: "web",
		Values: map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app.kubernetes.io/name": "web-frontend", "tier": "front"},
			},
		},
	}
	g.Resources[cpKey("Deployment", "db")] = &types.ProcessedResource{ServiceName: "db"}
	g.Connections = []types.Connection{cpEdge("web", "db", 5432)}

	res, _ := applyCP(t, "web", g)
	policy := res.Templates["templates/connection-policy.yaml"]

	if strings.Contains(policy, "      app: web\n") {
		t.Errorf("podSelector must not be hardcoded to app: <chart>:\n%s", policy)
	}
	if !strings.Contains(policy, `app.kubernetes.io/name: "web-frontend"`) || !strings.Contains(policy, `tier: "front"`) {
		t.Errorf("podSelector must use the workload selector labels:\n%s", policy)
	}
}

func TestConnectionPolicies_PodSelectorFallsBackToChartSelectorLabels(t *testing.T) {
	res, _ := applyCP(t, "web", nil)
	policy := res.Templates["templates/connection-policy.yaml"]

	if !strings.Contains(policy, `include "web.selectorLabels" .`) {
		t.Errorf("without a known workload the chart selectorLabels helper must be used:\n%s", policy)
	}
}
