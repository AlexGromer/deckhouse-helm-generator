package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Task 5.1: Role Processor Tests (TDD)
// ============================================================

func makeRoleObj(name, namespace string, labels map[string]interface{}, rules []interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	obj := map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "Role",
		"metadata":   metadata,
	}
	if rules != nil {
		obj["rules"] = rules
	}
	return &unstructured.Unstructured{Object: obj}
}

func TestProcessRole_ExtractsRules(t *testing.T) {
	p := NewRoleProcessor()
	ctx := newTestProcessorContext()

	rules := []interface{}{
		map[string]interface{}{
			"apiGroups": []interface{}{""},
			"resources": []interface{}{"pods", "services"},
			"verbs":     []interface{}{"get", "list", "watch"},
		},
	}
	obj := makeRoleObj("pod-reader", "default",
		map[string]interface{}{"app": "myapp"}, rules)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	extractedRules, ok := result.Values["rules"].([]interface{})
	if !ok {
		t.Fatal("Expected rules in values")
	}
	testutil.AssertEqual(t, 1, len(extractedRules))
}

func TestProcessRole_ExtractsMultipleRules(t *testing.T) {
	p := NewRoleProcessor()
	ctx := newTestProcessorContext()

	rules := []interface{}{
		map[string]interface{}{
			"apiGroups": []interface{}{""},
			"resources": []interface{}{"pods"},
			"verbs":     []interface{}{"get", "list"},
		},
		map[string]interface{}{
			"apiGroups": []interface{}{"apps"},
			"resources": []interface{}{"deployments"},
			"verbs":     []interface{}{"get", "list", "create", "update"},
		},
		map[string]interface{}{
			"apiGroups": []interface{}{""},
			"resources": []interface{}{"configmaps"},
			"verbs":     []interface{}{"*"},
		},
	}
	obj := makeRoleObj("multi-rule", "default",
		map[string]interface{}{"app": "admin"}, rules)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	extractedRules, ok := result.Values["rules"].([]interface{})
	if !ok {
		t.Fatal("Expected rules in values")
	}
	testutil.AssertEqual(t, 3, len(extractedRules))
}

func TestProcessRole_ExtractsResourceNames(t *testing.T) {
	p := NewRoleProcessor()
	ctx := newTestProcessorContext()

	rules := []interface{}{
		map[string]interface{}{
			"apiGroups":     []interface{}{""},
			"resources":     []interface{}{"configmaps"},
			"resourceNames": []interface{}{"my-config"},
			"verbs":         []interface{}{"get", "update"},
		},
	}
	obj := makeRoleObj("config-editor", "default",
		map[string]interface{}{"app": "myapp"}, rules)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	extractedRules := result.Values["rules"].([]interface{})
	rule := extractedRules[0].(map[string]interface{})
	resourceNames, ok := rule["resourceNames"].([]interface{})
	if !ok {
		t.Fatal("Expected resourceNames in rule")
	}
	testutil.AssertEqual(t, 1, len(resourceNames))
	testutil.AssertEqual(t, "my-config", resourceNames[0])
}

func TestProcessRole_EdgeCases(t *testing.T) {
	p := NewRoleProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilRole", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil Role")
		}
	})

	t.Run("EmptyRules", func(t *testing.T) {
		obj := makeRoleObj("empty-role", "default",
			map[string]interface{}{"app": "empty"}, []interface{}{})

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
	})

	t.Run("NoRules", func(t *testing.T) {
		obj := makeRoleObj("no-rules", "default",
			map[string]interface{}{"app": "none"}, nil)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
	})
}

func TestNewRoleProcessor(t *testing.T) {
	p := NewRoleProcessor()
	testutil.AssertEqual(t, "role", p.Name())
	testutil.AssertEqual(t, 80, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role",
	}, gvks[0])
}

func TestProcessRole_ResultMetadata(t *testing.T) {
	p := NewRoleProcessor()
	ctx := newTestProcessorContext()

	rules := []interface{}{
		map[string]interface{}{
			"apiGroups": []interface{}{""},
			"resources": []interface{}{"pods"},
			"verbs":     []interface{}{"get"},
		},
	}
	obj := makeRoleObj("viewer-role", "default",
		map[string]interface{}{"app": "viewer"}, rules)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "viewer", result.ServiceName)
	testutil.AssertEqual(t, "templates/viewer-role.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.viewer.role", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: Role")
}

func TestProcessRole_GeneratesTemplate(t *testing.T) {
	p := NewRoleProcessor()
	ctx := newTestProcessorContext()

	rules := []interface{}{
		map[string]interface{}{
			"apiGroups": []interface{}{""},
			"resources": []interface{}{"pods"},
			"verbs":     []interface{}{"get", "list"},
		},
	}
	obj := makeRoleObj("tmpl-role", "default",
		map[string]interface{}{"app": "tmpl"}, rules)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "rbac.authorization.k8s.io/v1")
	testutil.AssertContains(t, tmpl, "Role")
	testutil.AssertContains(t, tmpl, "rules")
	if !strings.Contains(tmpl, "test-chart") {
		t.Error("Expected template to reference chart name")
	}
}
