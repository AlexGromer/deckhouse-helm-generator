package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create TriggerAuthentication unstructured object
// ============================================================

func makeTriggerAuthObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "keda.sh/v1alpha1",
			"kind":       "TriggerAuthentication",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

// ============================================================
// Test 1: Processor name
// ============================================================

func TestTriggerAuthenticationProcessor_Name(t *testing.T) {
	proc := NewTriggerAuthenticationProcessor()
	testutil.AssertEqual(t, "triggerauthentication", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestTriggerAuthenticationProcessor_Supports(t *testing.T) {
	proc := NewTriggerAuthenticationProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "TriggerAuthentication",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: SecretTargetRef
// ============================================================

func TestTriggerAuthenticationProcessor_SecretTargetRef(t *testing.T) {
	proc := NewTriggerAuthenticationProcessor()
	ctx := newTestProcessorContext()

	obj := makeTriggerAuthObj("kafka-auth", "default", map[string]interface{}{
		"secretTargetRef": []interface{}{
			map[string]interface{}{
				"parameter": "sasl_password",
				"name":      "kafka-secrets",
				"key":       "password",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	secretRefs, ok := result.Values["secretTargetRef"].([]interface{})
	if !ok {
		t.Fatal("Expected secretTargetRef slice in values")
	}
	if len(secretRefs) != 1 {
		t.Fatalf("Expected 1 secretTargetRef, got %d", len(secretRefs))
	}

	ref := secretRefs[0].(map[string]interface{})
	testutil.AssertEqual(t, "sasl_password", ref["parameter"], "parameter")
	testutil.AssertEqual(t, "kafka-secrets", ref["name"], "secret name")
	testutil.AssertEqual(t, "password", ref["key"], "secret key")
}

// ============================================================
// Test 4: Env
// ============================================================

func TestTriggerAuthenticationProcessor_Env(t *testing.T) {
	proc := NewTriggerAuthenticationProcessor()
	ctx := newTestProcessorContext()

	obj := makeTriggerAuthObj("env-auth", "default", map[string]interface{}{
		"env": []interface{}{
			map[string]interface{}{
				"parameter":     "connection",
				"name":          "CONNECTION_STRING",
				"containerName": "myapp",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	envRefs, ok := result.Values["env"].([]interface{})
	if !ok {
		t.Fatal("Expected env slice in values")
	}
	if len(envRefs) != 1 {
		t.Fatalf("Expected 1 env ref, got %d", len(envRefs))
	}
}

// ============================================================
// Test 5: PodIdentity
// ============================================================

func TestTriggerAuthenticationProcessor_PodIdentity(t *testing.T) {
	proc := NewTriggerAuthenticationProcessor()
	ctx := newTestProcessorContext()

	obj := makeTriggerAuthObj("azure-auth", "default", map[string]interface{}{
		"podIdentity": map[string]interface{}{
			"provider": "azure-workload",
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	podIdentity, ok := result.Values["podIdentity"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected podIdentity map in values")
	}
	testutil.AssertEqual(t, "azure-workload", podIdentity["provider"], "provider")
}

// ============================================================
// Test 6: Template content
// ============================================================

func TestTriggerAuthenticationProcessor_Template(t *testing.T) {
	proc := NewTriggerAuthenticationProcessor()
	ctx := newTestProcessorContext()

	obj := makeTriggerAuthObj("kafka-auth", "default", map[string]interface{}{
		"secretTargetRef": []interface{}{
			map[string]interface{}{
				"parameter": "password",
				"name":      "my-secret",
				"key":       "pass",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: keda.sh/v1alpha1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: TriggerAuthentication", "kind")
	testutil.AssertContains(t, tpl, "$svc.enabled", "enabled check")
}
