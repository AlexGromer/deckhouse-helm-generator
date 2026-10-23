package types

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"testing"
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
