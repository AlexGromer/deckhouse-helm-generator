package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Task 5.2: ClusterRole Processor Tests (TDD)
// ============================================================

func makeClusterRoleObj(name string, labels map[string]interface{}, rules []interface{}, aggregationRule map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name": name,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	obj := map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata":   metadata,
	}
	if rules != nil {
		obj["rules"] = rules
	}
	if aggregationRule != nil {
		obj["aggregationRule"] = aggregationRule
	}
	return &unstructured.Unstructured{Object: obj}
}

func TestProcessClusterRole_ExtractsRules(t *testing.T) {
	p := NewClusterRoleProcessor()
	ctx := newTestProcessorContext()

	rules := []interface{}{
		map[string]interface{}{
			"apiGroups": []interface{}{""},
			"resources": []interface{}{"nodes"},
			"verbs":     []interface{}{"get", "list", "watch"},
		},
	}
	obj := makeClusterRoleObj("node-reader",
		map[string]interface{}{"app": "monitor"}, rules, nil)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	extractedRules, ok := result.Values["rules"].([]interface{})
	if !ok {
		t.Fatal("Expected rules in values")
	}
	testutil.AssertEqual(t, 1, len(extractedRules))
}

func TestProcessClusterRole_ExtractsMultipleRules(t *testing.T) {
	p := NewClusterRoleProcessor()
	ctx := newTestProcessorContext()

	rules := []interface{}{
		map[string]interface{}{
			"apiGroups": []interface{}{""},
			"resources": []interface{}{"pods"},
			"verbs":     []interface{}{"get", "list"},
		},
		map[string]interface{}{
			"apiGroups": []interface{}{"apps"},
			"resources": []interface{}{"deployments", "statefulsets"},
			"verbs":     []interface{}{"get", "list", "create", "update", "delete"},
		},
	}
	obj := makeClusterRoleObj("workload-admin",
		map[string]interface{}{"app": "admin"}, rules, nil)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	extractedRules := result.Values["rules"].([]interface{})
	testutil.AssertEqual(t, 2, len(extractedRules))
}

func TestProcessClusterRole_ExtractsAggregationRule(t *testing.T) {
	p := NewClusterRoleProcessor()
	ctx := newTestProcessorContext()

	aggregationRule := map[string]interface{}{
		"clusterRoleSelectors": []interface{}{
			map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"rbac.example.com/aggregate-to-monitoring": "true",
				},
			},
		},
	}
	obj := makeClusterRoleObj("aggregated-monitoring",
		map[string]interface{}{"app": "monitoring"}, nil, aggregationRule)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	agg, ok := result.Values["aggregationRule"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected aggregationRule in values")
	}
	selectors, ok := agg["clusterRoleSelectors"].([]interface{})
	if !ok {
		t.Fatal("Expected clusterRoleSelectors in aggregationRule")
	}
	testutil.AssertEqual(t, 1, len(selectors))
}

func TestProcessClusterRole_ExtractsNonResourceURLs(t *testing.T) {
	p := NewClusterRoleProcessor()
	ctx := newTestProcessorContext()

	rules := []interface{}{
		map[string]interface{}{
			"nonResourceURLs": []interface{}{"/healthz", "/readyz"},
			"verbs":           []interface{}{"get"},
		},
	}
	obj := makeClusterRoleObj("health-checker",
		map[string]interface{}{"app": "monitor"}, rules, nil)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	extractedRules := result.Values["rules"].([]interface{})
	rule := extractedRules[0].(map[string]interface{})
	urls, ok := rule["nonResourceURLs"].([]interface{})
	if !ok {
		t.Fatal("Expected nonResourceURLs in rule")
	}
	testutil.AssertEqual(t, 2, len(urls))
}

func TestProcessClusterRole_EdgeCases(t *testing.T) {
	p := NewClusterRoleProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilClusterRole", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil ClusterRole")
		}
	})

	t.Run("EmptyRules", func(t *testing.T) {
		obj := makeClusterRoleObj("empty-cr",
			map[string]interface{}{"app": "empty"}, []interface{}{}, nil)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
	})
}

func TestNewClusterRoleProcessor(t *testing.T) {
	p := NewClusterRoleProcessor()
	testutil.AssertEqual(t, "clusterrole", p.Name())
	testutil.AssertEqual(t, 80, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole",
	}, gvks[0])
}

func TestProcessClusterRole_ResultMetadata(t *testing.T) {
	p := NewClusterRoleProcessor()
	ctx := newTestProcessorContext()

	rules := []interface{}{
		map[string]interface{}{
			"apiGroups": []interface{}{""},
			"resources": []interface{}{"nodes"},
			"verbs":     []interface{}{"get"},
		},
	}
	obj := makeClusterRoleObj("viewer-cr",
		map[string]interface{}{"app": "viewer"}, rules, nil)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "viewer", result.ServiceName)
	testutil.AssertEqual(t, "templates/viewer-clusterrole.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.viewer.clusterRole", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: ClusterRole")
}

func TestProcessClusterRole_GeneratesTemplate(t *testing.T) {
	p := NewClusterRoleProcessor()
	ctx := newTestProcessorContext()

	rules := []interface{}{
		map[string]interface{}{
			"apiGroups": []interface{}{""},
			"resources": []interface{}{"pods"},
			"verbs":     []interface{}{"get"},
		},
	}
	obj := makeClusterRoleObj("tmpl-cr",
		map[string]interface{}{"app": "tmpl"}, rules, nil)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "rbac.authorization.k8s.io/v1")
	testutil.AssertContains(t, tmpl, "ClusterRole")
	testutil.AssertContains(t, tmpl, "rules")
	if !strings.Contains(tmpl, "test-chart") {
		t.Error("Expected template to reference chart name")
	}
}
