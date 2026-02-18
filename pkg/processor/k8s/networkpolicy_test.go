package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Task 4.3: NetworkPolicy Processor Tests (TDD)
// ============================================================

func makeNetPolObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

// ============================================================
// Subtask 1: Extract podSelector
// ============================================================

func TestProcessNetPol_ExtractsPodSelector(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"role": "db",
			},
		},
		"policyTypes": []interface{}{"Ingress"},
	}
	obj := makeNetPolObj("db-netpol", "default",
		map[string]interface{}{"app": "db"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	podSelector, ok := result.Values["podSelector"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected podSelector in values")
	}
	ml := podSelector["matchLabels"].(map[string]interface{})
	testutil.AssertEqual(t, "db", ml["role"])
}

// ============================================================
// Subtask 2: Extract policyTypes
// ============================================================

func TestProcessNetPol_ExtractsPolicyTypes(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{},
		"policyTypes": []interface{}{"Ingress", "Egress"},
	}
	obj := makeNetPolObj("bidirectional", "default",
		map[string]interface{}{"app": "bidirectional"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	policyTypes, ok := result.Values["policyTypes"].([]interface{})
	if !ok {
		t.Fatal("Expected policyTypes in values")
	}
	testutil.AssertEqual(t, 2, len(policyTypes))
	testutil.AssertEqual(t, "Ingress", policyTypes[0])
	testutil.AssertEqual(t, "Egress", policyTypes[1])
}

// ============================================================
// Subtask 3: Extract ingress rule (from pods)
// ============================================================

func TestProcessNetPol_ExtractsIngressFromPods(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"role": "db"},
		},
		"policyTypes": []interface{}{"Ingress"},
		"ingress": []interface{}{
			map[string]interface{}{
				"from": []interface{}{
					map[string]interface{}{
						"podSelector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"role": "frontend",
							},
						},
					},
				},
				"ports": []interface{}{
					map[string]interface{}{
						"protocol": "TCP",
						"port":     int64(5432),
					},
				},
			},
		},
	}
	obj := makeNetPolObj("db-ingress", "default",
		map[string]interface{}{"app": "db"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	ingress, ok := result.Values["ingress"].([]interface{})
	if !ok {
		t.Fatal("Expected ingress in values")
	}
	testutil.AssertEqual(t, 1, len(ingress))
	rule := ingress[0].(map[string]interface{})
	from := rule["from"].([]interface{})
	testutil.AssertEqual(t, 1, len(from))
}

// ============================================================
// Subtask 4: Extract ingress rule (from namespaces)
// ============================================================

func TestProcessNetPol_ExtractsIngressFromNamespaces(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "api"},
		},
		"policyTypes": []interface{}{"Ingress"},
		"ingress": []interface{}{
			map[string]interface{}{
				"from": []interface{}{
					map[string]interface{}{
						"namespaceSelector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"name": "prod",
							},
						},
					},
				},
			},
		},
	}
	obj := makeNetPolObj("api-ns-ingress", "default",
		map[string]interface{}{"app": "api"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	ingress := result.Values["ingress"].([]interface{})
	rule := ingress[0].(map[string]interface{})
	from := rule["from"].([]interface{})
	peer := from[0].(map[string]interface{})
	nsSelector, ok := peer["namespaceSelector"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected namespaceSelector in from peer")
	}
	ml := nsSelector["matchLabels"].(map[string]interface{})
	testutil.AssertEqual(t, "prod", ml["name"])
}

// ============================================================
// Subtask 5: Extract ingress rule (from IP blocks)
// ============================================================

func TestProcessNetPol_ExtractsIngressFromIPBlocks(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "web"},
		},
		"policyTypes": []interface{}{"Ingress"},
		"ingress": []interface{}{
			map[string]interface{}{
				"from": []interface{}{
					map[string]interface{}{
						"ipBlock": map[string]interface{}{
							"cidr":   "10.0.0.0/16",
							"except": []interface{}{"10.0.1.0/24"},
						},
					},
				},
			},
		},
	}
	obj := makeNetPolObj("web-ip-ingress", "default",
		map[string]interface{}{"app": "web"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	ingress := result.Values["ingress"].([]interface{})
	rule := ingress[0].(map[string]interface{})
	from := rule["from"].([]interface{})
	peer := from[0].(map[string]interface{})
	ipBlock, ok := peer["ipBlock"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected ipBlock in from peer")
	}
	testutil.AssertEqual(t, "10.0.0.0/16", ipBlock["cidr"])
}

// ============================================================
// Subtask 6: Extract egress rule (to pods)
// ============================================================

func TestProcessNetPol_ExtractsEgressToPods(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "worker"},
		},
		"policyTypes": []interface{}{"Egress"},
		"egress": []interface{}{
			map[string]interface{}{
				"to": []interface{}{
					map[string]interface{}{
						"podSelector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"role": "cache",
							},
						},
					},
				},
				"ports": []interface{}{
					map[string]interface{}{
						"protocol": "TCP",
						"port":     int64(6379),
					},
				},
			},
		},
	}
	obj := makeNetPolObj("worker-egress", "default",
		map[string]interface{}{"app": "worker"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	egress, ok := result.Values["egress"].([]interface{})
	if !ok {
		t.Fatal("Expected egress in values")
	}
	testutil.AssertEqual(t, 1, len(egress))
	rule := egress[0].(map[string]interface{})
	to := rule["to"].([]interface{})
	testutil.AssertEqual(t, 1, len(to))
}

// ============================================================
// Subtask 7: Extract egress rule (to IP blocks)
// ============================================================

func TestProcessNetPol_ExtractsEgressToIPBlocks(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "proxy"},
		},
		"policyTypes": []interface{}{"Egress"},
		"egress": []interface{}{
			map[string]interface{}{
				"to": []interface{}{
					map[string]interface{}{
						"ipBlock": map[string]interface{}{
							"cidr": "0.0.0.0/0",
						},
					},
				},
			},
		},
	}
	obj := makeNetPolObj("proxy-egress", "default",
		map[string]interface{}{"app": "proxy"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	egress := result.Values["egress"].([]interface{})
	rule := egress[0].(map[string]interface{})
	to := rule["to"].([]interface{})
	peer := to[0].(map[string]interface{})
	ipBlock := peer["ipBlock"].(map[string]interface{})
	testutil.AssertEqual(t, "0.0.0.0/0", ipBlock["cidr"])
}

// ============================================================
// Subtask 8: Extract port specifications
// ============================================================

func TestProcessNetPol_ExtractsPorts(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "multiport"},
		},
		"policyTypes": []interface{}{"Ingress"},
		"ingress": []interface{}{
			map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{"protocol": "TCP", "port": int64(80)},
					map[string]interface{}{"protocol": "TCP", "port": int64(443)},
					map[string]interface{}{"protocol": "UDP", "port": int64(53)},
				},
			},
		},
	}
	obj := makeNetPolObj("multiport-np", "default",
		map[string]interface{}{"app": "multiport"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	ingress := result.Values["ingress"].([]interface{})
	rule := ingress[0].(map[string]interface{})
	ports := rule["ports"].([]interface{})
	testutil.AssertEqual(t, 3, len(ports))
}

// ============================================================
// Subtask 9: Default-deny policy
// ============================================================

func TestProcessNetPol_DefaultDeny(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{},
		"policyTypes": []interface{}{"Ingress", "Egress"},
	}
	obj := makeNetPolObj("deny-all", "default",
		map[string]interface{}{"app": "deny-all"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	// No ingress or egress rules = deny all
	if _, exists := result.Values["ingress"]; exists {
		t.Error("Default-deny should not have ingress rules")
	}
	if _, exists := result.Values["egress"]; exists {
		t.Error("Default-deny should not have egress rules")
	}
}

// ============================================================
// Subtask 10: Edge cases
// ============================================================

func TestProcessNetPol_EdgeCases(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilNetPol", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil NetworkPolicy")
		}
	})

	t.Run("NoPolicyTypes", func(t *testing.T) {
		spec := map[string]interface{}{
			"podSelector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "notype"},
			},
		}
		obj := makeNetPolObj("notype-np", "default",
			map[string]interface{}{"app": "notype"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
	})
}

// ============================================================
// Constructor and metadata tests
// ============================================================

func TestNewNetworkPolicyProcessor(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	testutil.AssertEqual(t, "networkpolicy", p.Name())
	testutil.AssertEqual(t, 90, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy",
	}, gvks[0])
}

func TestProcessNetPol_ResultMetadata(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "webapp"},
		},
		"policyTypes": []interface{}{"Ingress"},
	}
	obj := makeNetPolObj("webapp-np", "default",
		map[string]interface{}{"app": "webapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "webapp", result.ServiceName)
	testutil.AssertEqual(t, "templates/webapp-networkpolicy.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.webapp.networkPolicy", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: NetworkPolicy")
}

func TestProcessNetPol_GeneratesTemplate(t *testing.T) {
	p := NewNetworkPolicyProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "tmplapp"},
		},
		"policyTypes": []interface{}{"Ingress", "Egress"},
		"ingress": []interface{}{
			map[string]interface{}{
				"from": []interface{}{
					map[string]interface{}{
						"podSelector": map[string]interface{}{
							"matchLabels": map[string]interface{}{"role": "frontend"},
						},
					},
				},
			},
		},
	}
	obj := makeNetPolObj("tmplapp-np", "default",
		map[string]interface{}{"app": "tmplapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "networking.k8s.io/v1")
	testutil.AssertContains(t, tmpl, "NetworkPolicy")
	testutil.AssertContains(t, tmpl, "podSelector")
	testutil.AssertContains(t, tmpl, "policyTypes")
	testutil.AssertContains(t, tmpl, "ingress")
	if !strings.Contains(tmpl, "test-chart") {
		t.Error("Expected template to reference chart name")
	}
}
