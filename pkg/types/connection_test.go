package types

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestConnectionID(t *testing.T) {
	conn := DetectedConnection{
		Source: ResourceKey{
			GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Namespace: "test-ns",
			Name:      "test-deploy",
		},
		Target: ResolvedTarget{
			Kind:      "Service",
			Namespace: "test-ns",
			Name:      "db-svc",
		},
		Kind: ConnectionDatabase,
		Endpoints: []ConnectionEndpoint{
			{Protocol: "tcp", Host: "db-svc", Port: 5432},
		},
	}
	expectedID := "Deployment/test-ns/test-deploy|database|Service/test-ns/db-svc|tcp://db-svc:5432"

	if conn.ID() != expectedID {
		t.Errorf("Expected ID %q, got %q", expectedID, conn.ID())
	}
}

func TestAddConnection(t *testing.T) {
	graph := NewResourceGraph()

	conn1 := DetectedConnection{
		Source: ResourceKey{Name: "app", GVK: schema.GroupVersionKind{Kind: "Deployment"}},
		Target: ResolvedTarget{Name: "db", Kind: "Service"},
		Kind:   ConnectionDatabase,
		Endpoints: []ConnectionEndpoint{
			{Protocol: "tcp", Host: "db", Port: 5432},
		},
		Evidence: []ConnectionEvidence{
			{Source: "env", Key: "DB_URL", Value: "db:5432"},
		},
		Confidence: ConfidenceLow,
	}

	// conn2 is same connection but with higher confidence and different evidence
	conn2 := DetectedConnection{
		Source: ResourceKey{Name: "app", GVK: schema.GroupVersionKind{Kind: "Deployment"}},
		Target: ResolvedTarget{Name: "db", Kind: "Service"},
		Kind:   ConnectionDatabase,
		Endpoints: []ConnectionEndpoint{
			{Protocol: "tcp", Host: "db", Port: 5432},
		},
		Evidence: []ConnectionEvidence{
			{Source: "configmap", Key: "database.url", Value: "db:5432"},
		},
		Confidence: ConfidenceHigh,
	}

	graph.AddConnection(conn1)
	graph.AddConnection(conn2)

	if len(graph.Connections) != 1 {
		t.Fatalf("Expected 1 connection after deduplication, got %d", len(graph.Connections))
	}

	c := graph.Connections[0]
	if c.Confidence != ConfidenceHigh {
		t.Errorf("Expected confidence High, got %v", c.Confidence)
	}
	if len(c.Evidence) != 2 {
		t.Errorf("Expected 2 evidences after merge, got %d", len(c.Evidence))
	}
}

func TestIsConnection(t *testing.T) {
	if !RelationServiceCall.IsConnection() {
		t.Error("RelationServiceCall should be a connection")
	}
	if RelationLabelSelector.IsConnection() {
		t.Error("RelationLabelSelector should not be a connection")
	}
}

func TestConnectionMethods(t *testing.T) {
	if !ConfidenceHigh.AtLeast(ConfidenceMedium) {
		t.Error("High should be at least Medium")
	}
	if ConfidenceUnknown.AtLeast(ConfidenceMedium) {
		t.Error("Unknown should not be at least Medium")
	}
	ct := ConnectionTarget{Node: "external-host:example.com"}
	if ct.String() != "external-host:example.com" {
		t.Errorf("unexpected ConnectionTarget.String(): %q", ct.String())
	}
	var nilTopo *Topology
	if nilTopo.PBCOf(ResourceKey{}) != "" {
		t.Error("expected empty PBC for nil topology")
	}
	topo := &Topology{
		PBCs:       []PBC{{Name: "orders", Services: []string{"order-api"}}},
		ServicePBC: map[string]string{"order-api": "orders", "orphan": ""},
	}
	if _, ok := topo.PBC("orders"); !ok {
		t.Error("expected to find orders PBC")
	}
	if len(topo.Unassigned()) != 1 || topo.Unassigned()[0] != "orphan" {
		t.Errorf("unexpected Unassigned: %v", topo.Unassigned())
	}
}
