package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Task 5.4: ClusterRoleBinding Processor Tests (TDD)
// ============================================================

func makeClusterRoleBindingObj(name string, labels map[string]interface{}, roleRef map[string]interface{}, subjects []interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name": name,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	obj := map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
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

func TestProcessClusterRoleBinding_ExtractsRoleRef(t *testing.T) {
	p := NewClusterRoleBindingProcessor()
	ctx := newTestProcessorContext()

	roleRef := map[string]interface{}{
		"apiGroup": "rbac.authorization.k8s.io",
		"kind":     "ClusterRole",
		"name":     "cluster-admin",
	}
	subjects := []interface{}{
		map[string]interface{}{
			"kind":      "ServiceAccount",
			"name":      "admin-sa",
			"namespace": "kube-system",
		},
	}
	obj := makeClusterRoleBindingObj("admin-binding",
		map[string]interface{}{"app": "admin"}, roleRef, subjects)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	ref, ok := result.Values["roleRef"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected roleRef in values")
	}
	testutil.AssertEqual(t, "ClusterRole", ref["kind"])
	testutil.AssertEqual(t, "cluster-admin", ref["name"])
}

func TestProcessClusterRoleBinding_ExtractsSubjects(t *testing.T) {
	p := NewClusterRoleBindingProcessor()
	ctx := newTestProcessorContext()

	roleRef := map[string]interface{}{
		"apiGroup": "rbac.authorization.k8s.io",
		"kind":     "ClusterRole",
		"name":     "viewer",
	}
	subjects := []interface{}{
		map[string]interface{}{
			"kind":      "ServiceAccount",
			"name":      "monitoring-sa",
			"namespace": "monitoring",
		},
		map[string]interface{}{
			"kind":     "Group",
			"name":     "system:authenticated",
			"apiGroup": "rbac.authorization.k8s.io",
		},
	}
	obj := makeClusterRoleBindingObj("viewer-binding",
		map[string]interface{}{"app": "viewer"}, roleRef, subjects)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	extractedSubjects, ok := result.Values["subjects"].([]interface{})
	if !ok {
		t.Fatal("Expected subjects in values")
	}
	testutil.AssertEqual(t, 2, len(extractedSubjects))
}

func TestProcessClusterRoleBinding_CreatesDependencies(t *testing.T) {
	p := NewClusterRoleBindingProcessor()
	ctx := newTestProcessorContext()

	roleRef := map[string]interface{}{
		"apiGroup": "rbac.authorization.k8s.io",
		"kind":     "ClusterRole",
		"name":     "my-clusterrole",
	}
	subjects := []interface{}{
		map[string]interface{}{
			"kind":      "ServiceAccount",
			"name":      "my-sa",
			"namespace": "default",
		},
	}
	obj := makeClusterRoleBindingObj("my-crb",
		map[string]interface{}{"app": "myapp"}, roleRef, subjects)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	foundCR := false
	foundSA := false
	for _, dep := range result.Dependencies {
		if dep.GVK.Kind == "ClusterRole" && dep.Name == "my-clusterrole" {
			foundCR = true
		}
		if dep.GVK.Kind == "ServiceAccount" && dep.Name == "my-sa" {
			foundSA = true
		}
	}
	if !foundCR {
		t.Error("Expected ClusterRole dependency")
	}
	if !foundSA {
		t.Error("Expected ServiceAccount dependency")
	}
}

func TestProcessClusterRoleBinding_EdgeCases(t *testing.T) {
	p := NewClusterRoleBindingProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilClusterRoleBinding", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil ClusterRoleBinding")
		}
	})

	t.Run("NoSubjects", func(t *testing.T) {
		roleRef := map[string]interface{}{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "ClusterRole",
			"name":     "orphan-cr",
		}
		obj := makeClusterRoleBindingObj("orphan-crb",
			map[string]interface{}{"app": "orphan"}, roleRef, nil)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
	})
}

func TestNewClusterRoleBindingProcessor(t *testing.T) {
	p := NewClusterRoleBindingProcessor()
	testutil.AssertEqual(t, "clusterrolebinding", p.Name())
	testutil.AssertEqual(t, 70, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding",
	}, gvks[0])
}

func TestProcessClusterRoleBinding_ResultMetadata(t *testing.T) {
	p := NewClusterRoleBindingProcessor()
	ctx := newTestProcessorContext()

	roleRef := map[string]interface{}{
		"apiGroup": "rbac.authorization.k8s.io",
		"kind":     "ClusterRole",
		"name":     "viewer",
	}
	subjects := []interface{}{
		map[string]interface{}{
			"kind":      "ServiceAccount",
			"name":      "viewer-sa",
			"namespace": "default",
		},
	}
	obj := makeClusterRoleBindingObj("viewer-crb",
		map[string]interface{}{"app": "viewer"}, roleRef, subjects)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "viewer", result.ServiceName)
	testutil.AssertEqual(t, "templates/viewer-clusterrolebinding.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.viewer.clusterRoleBinding", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: ClusterRoleBinding")
}

func TestProcessClusterRoleBinding_GeneratesTemplate(t *testing.T) {
	p := NewClusterRoleBindingProcessor()
	ctx := newTestProcessorContext()

	roleRef := map[string]interface{}{
		"apiGroup": "rbac.authorization.k8s.io",
		"kind":     "ClusterRole",
		"name":     "tmpl-cr",
	}
	subjects := []interface{}{
		map[string]interface{}{
			"kind": "ServiceAccount",
			"name": "tmpl-sa",
		},
	}
	obj := makeClusterRoleBindingObj("tmpl-crb",
		map[string]interface{}{"app": "tmpl"}, roleRef, subjects)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "rbac.authorization.k8s.io/v1")
	testutil.AssertContains(t, tmpl, "ClusterRoleBinding")
	testutil.AssertContains(t, tmpl, "roleRef")
	testutil.AssertContains(t, tmpl, "subjects")
	if !strings.Contains(tmpl, "test-chart") {
		t.Error("Expected template to reference chart name")
	}
}
