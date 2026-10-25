package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create ListenerSet unstructured object
// ============================================================

func makeListenerSetObj(name, namespace, apiVersion string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       "ListenerSet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

// ============================================================
// Test: ListenerSet extraction
// ============================================================

func TestListenerSet_Extraction(t *testing.T) {
	proc := NewListenerSetProcessor()
	ctx := newTestProcessorContext()

	tests := []struct {
		name       string
		apiVersion string
		wantNotes  bool
	}{
		{
			name:       "v1 API version",
			apiVersion: "gateway.networking.k8s.io/v1",
			wantNotes:  false,
		},
		{
			name:       "v1alpha1 API version",
			apiVersion: "gateway.networking.x-k8s.io/v1alpha1",
			wantNotes:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := makeListenerSetObj("listener-set", "default", tt.apiVersion, map[string]interface{}{
				"parentRef": map[string]interface{}{
					"name":      "my-gateway",
					"namespace": "gateway-ns",
				},
				"listeners": []interface{}{
					map[string]interface{}{
						"name":     "https",
						"port":     int64(443),
						"protocol": "HTTPS",
					},
				},
			})

			result, err := proc.Process(ctx, obj)
			testutil.AssertNoError(t, err)
			testutil.AssertEqual(t, true, result.Processed, "should be processed")
			testutil.AssertEqual(t, "listener-set", result.ServiceName, "serviceName")

			// Check parentRef extracted
			parentRef, ok := result.Values["parentRef"].(map[string]interface{})
			if !ok || len(parentRef) == 0 {
				t.Fatal("Expected parentRef")
			}
			testutil.AssertEqual(t, "my-gateway", parentRef["name"], "parentRef name")

			// Check listeners extracted
			listeners, ok := result.Values["listeners"].([]interface{})
			if !ok || len(listeners) != 1 {
				t.Fatal("Expected 1 listener")
			}

			// Check template
			tpl := result.TemplateContent
			testutil.AssertContains(t, tpl, "apiVersion: gateway.networking.k8s.io/v1\n", "api version")
			testutil.AssertContains(t, tpl, "kind: ListenerSet", "kind")
			if !strings.Contains(tpl, "parentRef") {
				t.Error("Template should reference parentRef")
			}
			if !strings.Contains(tpl, "listeners") {
				t.Error("Template should reference listeners")
			}
		})
	}

	// Check GVK registration
	gvks := proc.Supports()
	if len(gvks) != 2 {
		t.Errorf("Expected 2 GVKs, got %d", len(gvks))
	}
	expectedV1 := schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "ListenerSet"}
	expectedV1Alpha1 := schema.GroupVersionKind{Group: "gateway.networking.x-k8s.io", Version: "v1alpha1", Kind: "ListenerSet"}
	if gvks[0] != expectedV1 && gvks[1] != expectedV1 {
		t.Errorf("Expected to support v1")
	}
	if gvks[0] != expectedV1Alpha1 && gvks[1] != expectedV1Alpha1 {
		t.Errorf("Expected to support v1alpha1")
	}
}

// ============================================================
// Test: ListenerSet processor name
// ============================================================

func TestListenerSet_Name(t *testing.T) {
	proc := NewListenerSetProcessor()
	testutil.AssertEqual(t, "listenerset", proc.Name(), "processor name")
}
