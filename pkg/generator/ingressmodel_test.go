package generator

import (
	"reflect"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildTestGraph(objs ...*unstructured.Unstructured) *types.ResourceGraph {
	graph := types.NewResourceGraph()
	for _, obj := range objs {
		r := &types.ProcessedResource{
			Original: &types.ExtractedResource{
				Object: obj,
				GVK:    obj.GroupVersionKind(),
			},
			TemplatePath: "templates/" + obj.GetName() + ".yaml",
		}
		graph.AddResource(r)
	}
	return graph
}

func makeIngress(name, ns string, ann map[string]string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": spec,
		},
	}
	obj.SetAnnotations(ann)
	return obj
}

func makeService(name, ns string, ports []map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"ports": toInterfacesMap(ports),
			},
		},
	}
	return obj
}

// Convert slice of maps to slice of interfaces
func toInterfacesMap(maps []map[string]interface{}) []interface{} {
	var res []interface{}
	for _, m := range maps {
		res = append(res, m)
	}
	return res
}

func TestExtractIngressModel_HostsGrouped(t *testing.T) {
	ing := makeIngress("ing1", "default", nil, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"host": "a.example.com", "http": map[string]interface{}{"paths": []interface{}{map[string]interface{}{"path": "/x"}}}},
			map[string]interface{}{"host": "b.example.com", "http": map[string]interface{}{"paths": []interface{}{map[string]interface{}{"path": "/"}}}},
			map[string]interface{}{"host": "a.example.com", "http": map[string]interface{}{"paths": []interface{}{map[string]interface{}{"path": "/y"}}}},
		},
	})
	graph := buildTestGraph(ing)

	model := ExtractIngressModel(graph)
	if len(model.Items) != 1 {
		t.Fatalf("expected 1 ingress, got %d", len(model.Items))
	}
	hosts := model.Items[0].Hosts
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	// 'a.example.com' first, 'b.example.com' second due to lex ordering
	if hosts[0].Host != "a.example.com" {
		t.Errorf("expected a.example.com, got %s", hosts[0].Host)
	}
	if len(hosts[0].Rules) != 2 {
		t.Errorf("expected 2 rules for a, got %d", len(hosts[0].Rules))
	}
	if hosts[1].Host != "b.example.com" {
		t.Errorf("expected b.example.com, got %s", hosts[1].Host)
	}
}

func TestExtractIngressModel_Defaults(t *testing.T) {
	ing := makeIngress("ing1", "default", nil, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"http": map[string]interface{}{"paths": []interface{}{map[string]interface{}{}}}},
		},
	})
	graph := buildTestGraph(ing)

	model := ExtractIngressModel(graph)
	hosts := model.Items[0].Hosts
	if hosts[0].Host != "*" {
		t.Errorf("expected host *, got %s", hosts[0].Host)
	}
	match := hosts[0].Rules[0].Matches[0]
	if match.Path != "/" {
		t.Errorf("expected path /, got %s", match.Path)
	}
	if match.PathType != PathPrefix {
		t.Errorf("expected PathPrefix, got %s", match.PathType)
	}
}

func TestExtractIngressModel_PortNameResolved(t *testing.T) {
	ing := makeIngress("ing1", "default", nil, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"http": map[string]interface{}{"paths": []interface{}{
				map[string]interface{}{"backend": map[string]interface{}{"service": map[string]interface{}{"name": "orders", "port": map[string]interface{}{"name": "http"}}}},
			}}},
		},
	})
	svc := makeService("orders", "default", []map[string]interface{}{
		{"name": "http", "port": int64(8080)},
	})
	graph := buildTestGraph(ing, svc)

	model := ExtractIngressModel(graph)
	target := model.Items[0].Hosts[0].Rules[0].BackendTargets[0]
	if target.PortNumber != 8080 {
		t.Errorf("expected port 8080, got %d", target.PortNumber)
	}
}

func TestExtractIngressModel_PortNumberString(t *testing.T) {
	ing := makeIngress("ing1", "default", nil, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"http": map[string]interface{}{"paths": []interface{}{
				map[string]interface{}{"backend": map[string]interface{}{"service": map[string]interface{}{"name": "orders", "port": map[string]interface{}{"number": "80"}}}},
			}}},
		},
	})
	graph := buildTestGraph(ing)

	model := ExtractIngressModel(graph)
	target := model.Items[0].Hosts[0].Rules[0].BackendTargets[0]
	if target.PortNumber != 80 {
		t.Errorf("expected port 80, got %d", target.PortNumber)
	}
}

func TestExtractIngressModel_PassthroughTLS(t *testing.T) {
	ing := makeIngress("ing1", "default", map[string]string{"nginx.ingress.kubernetes.io/ssl-passthrough": "true"}, map[string]interface{}{
		"tls": []interface{}{
			map[string]interface{}{"hosts": []interface{}{"a.com"}, "secretName": "tls-secret"},
		},
	})
	graph := buildTestGraph(ing)

	model := ExtractIngressModel(graph)
	tls := model.Items[0].TLS[0]
	if tls.Mode != TLSModePassthrough {
		t.Errorf("expected passthrough mode, got %s", tls.Mode)
	}
}

func TestExtractIngressModel_SortOrder(t *testing.T) {
	ing1 := makeIngress("b", "default", nil, nil)
	ing2 := makeIngress("a", "default", nil, nil)
	ing3 := makeIngress("a", "kube-system", nil, nil)
	graph := buildTestGraph(ing1, ing2, ing3)

	model := ExtractIngressModel(graph)
	if model.Items[0].Namespace != "default" || model.Items[0].Name != "a" {
		t.Errorf("expected default/a, got %s/%s", model.Items[0].Namespace, model.Items[0].Name)
	}
	if model.Items[1].Namespace != "default" || model.Items[1].Name != "b" {
		t.Errorf("expected default/b, got %s/%s", model.Items[1].Namespace, model.Items[1].Name)
	}
	if model.Items[2].Namespace != "kube-system" || model.Items[2].Name != "a" {
		t.Errorf("expected kube-system/a, got %s/%s", model.Items[2].Namespace, model.Items[2].Name)
	}
}

func TestExtractIngressModel_Determinism(t *testing.T) {
	ing := makeIngress("ing1", "default", nil, map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"host": "b", "http": map[string]interface{}{"paths": []interface{}{
				map[string]interface{}{"path": "/", "pathType": "Prefix"},
				map[string]interface{}{"path": "/exact", "pathType": "Exact"},
			}}},
			map[string]interface{}{"host": "*", "http": map[string]interface{}{"paths": []interface{}{
				map[string]interface{}{"path": "/b", "pathType": "Prefix"},
			}}},
			map[string]interface{}{"host": "a", "http": map[string]interface{}{"paths": []interface{}{
				map[string]interface{}{"path": "/a", "pathType": "Prefix"},
			}}},
		},
	})
	graph := buildTestGraph(ing)

	model1 := ExtractIngressModel(graph)
	model2 := ExtractIngressModel(graph)

	if !reflect.DeepEqual(model1, model2) {
		t.Errorf("expected deterministic result")
	}

	hosts := model1.Items[0].Hosts
	if hosts[0].Host != "a" || hosts[1].Host != "b" || hosts[2].Host != "*" {
		t.Errorf("hosts order incorrect: %v", hosts)
	}

	// Check exact before prefix for 'b'
	if hosts[1].Rules[0].Matches[0].PathType != PathExact {
		t.Errorf("expected Exact first for b")
	}
}
