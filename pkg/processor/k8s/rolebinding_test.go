package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Task 5.3: RoleBinding Processor Tests (TDD)
// ============================================================

func makeRoleBindingObj(name, namespace string, labels map[string]interface{}, roleRef map[string]interface{}, subjects []interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	obj := map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "RoleBinding",
		"metadata":   metadata,
	}
	if roleRef != nil {
		obj["roleRef"] = roleRef
	}
	if subjects != nil {
		obj["subjects"] = subjects
	}
	return &unstructured.Unstructured{Object: obj}
}

func TestProcessRoleBinding_ExtractsRoleRef(t *testing.T) {
	p := NewRoleBindingProcessor()
	ctx := newTestProcessorContext()

	roleRef := map[string]interface{}{
		"apiGroup": "rbac.authorization.k8s.io",
		"kind":     "Role",
		"name":     "pod-reader",
	}
	subjects := []interface{}{
		map[string]interface{}{
			"kind":      "ServiceAccount",
			"name":      "default",
			"namespace": "default",
		},
	}
	obj := makeRoleBindingObj("read-pods", "default",
		map[string]interface{}{"app": "myapp"}, roleRef, subjects)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	ref, ok := result.Values["roleRef"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected roleRef in values")
	}
	testutil.AssertEqual(t, "Role", ref["kind"])
	testutil.AssertEqual(t, "pod-reader", ref["name"])
}

func TestProcessRoleBinding_ExtractsSubjects(t *testing.T) {
	p := NewRoleBindingProcessor()
	ctx := newTestProcessorContext()

	roleRef := map[string]interface{}{
		"apiGroup": "rbac.authorization.k8s.io",
		"kind":     "Role",
		"name":     "editor",
	}
	subjects := []interface{}{
		map[string]interface{}{
			"kind":      "ServiceAccount",
			"name":      "app-sa",
			"namespace": "default",
		},
		map[string]interface{}{
			"kind":     "User",
			"name":     "jane",
			"apiGroup": "rbac.authorization.k8s.io",
		},
		map[string]interface{}{
			"kind":     "Group",
			"name":     "developers",
			"apiGroup": "rbac.authorization.k8s.io",
		},
	}
	obj := makeRoleBindingObj("editor-binding", "default",
		map[string]interface{}{"app": "editor"}, roleRef, subjects)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	extractedSubjects, ok := result.Values["subjects"].([]interface{})
	if !ok {
		t.Fatal("Expected subjects in values")
	}
	testutil.AssertEqual(t, 3, len(extractedSubjects))
}

func TestProcessRoleBinding_CreatesDependencies(t *testing.T) {
	p := NewRoleBindingProcessor()
	ctx := newTestProcessorContext()

	roleRef := map[string]interface{}{
		"apiGroup": "rbac.authorization.k8s.io",
		"kind":     "Role",
		"name":     "target-role",
	}
	subjects := []interface{}{
		map[string]interface{}{
			"kind":      "ServiceAccount",
			"name":      "my-sa",
			"namespace": "default",
		},
	}
	obj := makeRoleBindingObj("my-binding", "default",
		map[string]interface{}{"app": "myapp"}, roleRef, subjects)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// Should have Role dependency
	foundRole := false
	foundSA := false
	for _, dep := range result.Dependencies {
		if dep.GVK.Kind == "Role" && dep.Name == "target-role" {
			foundRole = true
		}
		if dep.GVK.Kind == "ServiceAccount" && dep.Name == "my-sa" {
			foundSA = true
		}
	}
	if !foundRole {
		t.Error("Expected Role dependency")
	}
	if !foundSA {
		t.Error("Expected ServiceAccount dependency")
	}
}

func TestProcessRoleBinding_EdgeCases(t *testing.T) {
	p := NewRoleBindingProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilRoleBinding", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil RoleBinding")
		}
	})

	t.Run("NoSubjects", func(t *testing.T) {
		roleRef := map[string]interface{}{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "Role",
			"name":     "orphan-role",
		}
		obj := makeRoleBindingObj("orphan-binding", "default",
			map[string]interface{}{"app": "orphan"}, roleRef, nil)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
	})
}

func TestNewRoleBindingProcessor(t *testing.T) {
	p := NewRoleBindingProcessor()
	testutil.AssertEqual(t, "rolebinding", p.Name())
	testutil.AssertEqual(t, 70, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding",
	}, gvks[0])
}

func TestProcessRoleBinding_ResultMetadata(t *testing.T) {
	p := NewRoleBindingProcessor()
	ctx := newTestProcessorContext()

	roleRef := map[string]interface{}{
		"apiGroup": "rbac.authorization.k8s.io",
		"kind":     "Role",
		"name":     "viewer",
	}
	subjects := []interface{}{
		map[string]interface{}{
			"kind":      "ServiceAccount",
			"name":      "viewer-sa",
			"namespace": "default",
		},
	}
	obj := makeRoleBindingObj("viewer-binding", "default",
		map[string]interface{}{"app": "viewer"}, roleRef, subjects)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "viewer", result.ServiceName)
	testutil.AssertEqual(t, "templates/viewer-rolebinding.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.viewer.roleBinding", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: RoleBinding")
}

func TestProcessRoleBinding_GeneratesTemplate(t *testing.T) {
	p := NewRoleBindingProcessor()
	ctx := newTestProcessorContext()

	roleRef := map[string]interface{}{
		"apiGroup": "rbac.authorization.k8s.io",
		"kind":     "Role",
		"name":     "tmpl-role",
	}
	subjects := []interface{}{
		map[string]interface{}{
			"kind": "ServiceAccount",
			"name": "tmpl-sa",
		},
	}
	obj := makeRoleBindingObj("tmpl-rb", "default",
		map[string]interface{}{"app": "tmpl"}, roleRef, subjects)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "rbac.authorization.k8s.io/v1")
	testutil.AssertContains(t, tmpl, "RoleBinding")
	testutil.AssertContains(t, tmpl, "roleRef")
	testutil.AssertContains(t, tmpl, "subjects")
	if !strings.Contains(tmpl, "test-chart") {
		t.Error("Expected template to reference chart name")
	}
}
