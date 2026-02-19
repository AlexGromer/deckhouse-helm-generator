package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

func makeClusterAuthorizationRuleObj(name string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1",
			"kind":       "ClusterAuthorizationRule",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
	if spec != nil {
		obj.Object["spec"] = spec
	}
	return obj
}

func TestClusterAuthorizationRuleProcessor_Name(t *testing.T) {
	proc := NewClusterAuthorizationRuleProcessor()
	testutil.AssertEqual(t, "clusterauthorizationrule", proc.Name(), "processor name")
}

func TestClusterAuthorizationRuleProcessor_Supports(t *testing.T) {
	proc := NewClusterAuthorizationRuleProcessor()
	gvks := proc.Supports()
	if len(gvks) != 1 {
		t.Fatalf("Expected 1 GVK, got %d", len(gvks))
	}
	expected := schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "ClusterAuthorizationRule"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

func TestClusterAuthorizationRuleProcessor_Subjects(t *testing.T) {
	proc := NewClusterAuthorizationRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterAuthorizationRuleObj("admins", map[string]interface{}{
		"subjects": []interface{}{
			map[string]interface{}{"kind": "User", "name": "admin@example.com"},
			map[string]interface{}{"kind": "Group", "name": "admins"},
		},
		"accessLevel": "Admin",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	subjects, ok := result.Values["subjects"].([]interface{})
	if !ok {
		t.Fatal("Expected subjects as slice in values")
	}
	if len(subjects) != 2 {
		t.Fatalf("Expected 2 subjects, got %d", len(subjects))
	}
}

func TestClusterAuthorizationRuleProcessor_AccessLevel_User(t *testing.T) {
	proc := NewClusterAuthorizationRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterAuthorizationRuleObj("viewers", map[string]interface{}{
		"subjects":    []interface{}{map[string]interface{}{"kind": "User", "name": "viewer@example.com"}},
		"accessLevel": "User",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "User", result.Values["accessLevel"], "accessLevel")
}

func TestClusterAuthorizationRuleProcessor_AccessLevel_Admin(t *testing.T) {
	proc := NewClusterAuthorizationRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterAuthorizationRuleObj("admins", map[string]interface{}{
		"subjects":    []interface{}{map[string]interface{}{"kind": "User", "name": "admin@example.com"}},
		"accessLevel": "Admin",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Admin", result.Values["accessLevel"], "accessLevel")
}

func TestClusterAuthorizationRuleProcessor_AccessLevel_ClusterAdmin(t *testing.T) {
	proc := NewClusterAuthorizationRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterAuthorizationRuleObj("superadmins", map[string]interface{}{
		"subjects":    []interface{}{map[string]interface{}{"kind": "Group", "name": "superadmins"}},
		"accessLevel": "ClusterAdmin",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "ClusterAdmin", result.Values["accessLevel"], "accessLevel")
}

func TestClusterAuthorizationRuleProcessor_LimitNamespaces(t *testing.T) {
	proc := NewClusterAuthorizationRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterAuthorizationRuleObj("dev-team", map[string]interface{}{
		"subjects":        []interface{}{map[string]interface{}{"kind": "Group", "name": "dev-team"}},
		"accessLevel":     "Editor",
		"limitNamespaces": []interface{}{"dev-*", "staging-*"},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	ns, ok := result.Values["limitNamespaces"].([]interface{})
	if !ok {
		t.Fatal("Expected limitNamespaces as slice")
	}
	if len(ns) != 2 {
		t.Fatalf("Expected 2 namespace patterns, got %d", len(ns))
	}
	testutil.AssertEqual(t, "dev-*", ns[0], "first namespace pattern")
}

func TestClusterAuthorizationRuleProcessor_AllowScale(t *testing.T) {
	proc := NewClusterAuthorizationRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterAuthorizationRuleObj("deployers", map[string]interface{}{
		"subjects":    []interface{}{map[string]interface{}{"kind": "Group", "name": "deployers"}},
		"accessLevel": "Editor",
		"allowScale":  true,
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Values["allowScale"], "allowScale")
}

func TestClusterAuthorizationRuleProcessor_Template(t *testing.T) {
	proc := NewClusterAuthorizationRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterAuthorizationRuleObj("admins", map[string]interface{}{
		"subjects":    []interface{}{map[string]interface{}{"kind": "User", "name": "admin@example.com"}},
		"accessLevel": "Admin",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	if tpl == "" {
		t.Fatal("Expected non-empty template")
	}
	testutil.AssertContains(t, tpl, "apiVersion: deckhouse.io/v1", "template apiVersion")
	testutil.AssertContains(t, tpl, "kind: ClusterAuthorizationRule", "template kind")
	testutil.AssertContains(t, tpl, ".accessLevel", "template should reference accessLevel")
}

func TestClusterAuthorizationRuleProcessor_ServiceName(t *testing.T) {
	proc := NewClusterAuthorizationRuleProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterAuthorizationRuleObj("admin-rule", map[string]interface{}{
		"subjects":    []interface{}{map[string]interface{}{"kind": "User", "name": "admin@example.com"}},
		"accessLevel": "Admin",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "admin-rule", result.ServiceName, "ServiceName should be metadata.name")
}
