package generator

import (
	"reflect"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestEgressDetectHosts_WithGraphConnections(t *testing.T) {
	g := types.NewResourceGraph()

	workloadKey := types.ResourceKey{GVK: schema.GroupVersionKind{Kind: "Deployment", Version: "v1", Group: "apps"}, Namespace: "test", Name: "app"}
	wObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "app",
				"namespace": "test",
			},
		},
	}
	res := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: wObj,
			GVK:    wObj.GroupVersionKind(),
		},
	}
	g.AddResource(res)

	// Add an external connection
	g.AddConnection(types.Connection{
		From:     workloadKey,
		Type:     types.RelationExternalCall,
		To:       types.ConnectionTarget{Node: "external-host:api.example.com"},
		Protocol: "HTTPS",
		TLS:      true,
		Port:     443,
	})

	workloads := []secWorkload{
		{
			res:     res,
			kind:    "Deployment",
			name:    "app",
			podSpec: map[string]interface{}{},
		},
	}

	hosts := egressDetectHosts(workloads, g)

	wantHosts := map[string]map[egressPort]bool{
		"api.example.com": {
			egressPort{Number: 443, Protocol: "TLS"}: true,
		},
	}

	if !reflect.DeepEqual(hosts, wantHosts) {
		t.Errorf("Expected %v, got %v", wantHosts, hosts)
	}
}

func TestEgressDetectHosts_EmptyGraphFallback(t *testing.T) {
	g := types.NewResourceGraph() // Empty graph

	wObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "app",
				"namespace": "test",
			},
		},
	}
	res := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: wObj,
			GVK:    wObj.GroupVersionKind(),
		},
	}

	podSpec := map[string]interface{}{
		"containers": []interface{}{
			map[string]interface{}{
				"name": "app",
				"env": []interface{}{
					map[string]interface{}{
						"name":  "API_URL",
						"value": "https://fallback.example.com",
					},
				},
			},
		},
	}

	workloads := []secWorkload{
		{
			res:     res,
			kind:    "Deployment",
			name:    "app",
			podSpec: podSpec,
		},
	}

	hosts := egressDetectHosts(workloads, g)

	wantHosts := map[string]map[egressPort]bool{
		"fallback.example.com": {
			egressPort{Number: 443, Protocol: "TLS"}: true, // https translates to 443 TLS in fallback
		},
	}

	if !reflect.DeepEqual(hosts, wantHosts) {
		t.Errorf("Expected %v, got %v", wantHosts, hosts)
	}
}
