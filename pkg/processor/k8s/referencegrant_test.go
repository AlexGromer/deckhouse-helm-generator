package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create ReferenceGrant unstructured object
// ============================================================

func makeReferenceGrantObj(name, namespace, apiVersion string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       "ReferenceGrant",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

// ============================================================
// Test: ReferenceGrant extraction
// ============================================================

func TestReferenceGrant_Extraction(t *testing.T) {
	proc := NewReferenceGrantProcessor()
	ctx := newTestProcessorContext()

	tests := []struct {
		name       string
		apiVersion string
		wantNotes  bool
	}{
		{
			name:       "v1beta1 API version",
			apiVersion: "gateway.networking.k8s.io/v1beta1",
			wantNotes:  false,
		},
		{
			name:       "v1alpha2 API version",
			apiVersion: "gateway.networking.k8s.io/v1alpha2",
			wantNotes:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := makeReferenceGrantObj("reference-grant", "default", tt.apiVersion, map[string]interface{}{
				"from": []interface{}{
					map[string]interface{}{
						"group":     "gateway.networking.k8s.io",
						"kind":      "HTTPRoute",
						"namespace": "client-ns",
					},
				},
				"to": []interface{}{
					map[string]interface{}{
						"group": "",
						"kind":  "Service",
						"name":  "my-service",
					},
				},
			})

			result, err := proc.Process(ctx, obj)
			testutil.AssertNoError(t, err)
			testutil.AssertEqual(t, true, result.Processed, "should be processed")
			testutil.AssertEqual(t, "reference-grant", result.ServiceName, "serviceName")

			// Check from extracted
			from, ok := result.Values["from"].([]interface{})
			if !ok || len(from) != 1 {
				t.Fatal("Expected 1 from")
			}

			// Check to extracted
			to, ok := result.Values["to"].([]interface{})
			if !ok || len(to) != 1 {
				t.Fatal("Expected 1 to")
			}

			// Check template
			tpl := result.TemplateContent
			testutil.AssertContains(t, tpl, "apiVersion: gateway.networking.k8s.io/v1beta1\n", "api version")
			testutil.AssertContains(t, tpl, "kind: ReferenceGrant", "kind")
			if !strings.Contains(tpl, "from") {
				t.Error("Template should reference from")
			}
			if !strings.Contains(tpl, "to") {
				t.Error("Template should reference to")
			}
		})
	}

	// Check GVK registration
	gvks := proc.Supports()
	if len(gvks) != 2 {
		t.Errorf("Expected 2 GVKs, got %d", len(gvks))
	}
	expectedV1Beta1 := schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1beta1", Kind: "ReferenceGrant"}
	expectedV1Alpha2 := schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Kind: "ReferenceGrant"}
	if gvks[0] != expectedV1Beta1 && gvks[1] != expectedV1Beta1 {
		t.Errorf("Expected to support v1beta1")
	}
	if gvks[0] != expectedV1Alpha2 && gvks[1] != expectedV1Alpha2 {
		t.Errorf("Expected to support v1alpha2")
	}
}

// ============================================================
// Test: ReferenceGrant processor name
// ============================================================

func TestReferenceGrant_Name(t *testing.T) {
	proc := NewReferenceGrantProcessor()
	testutil.AssertEqual(t, "referencegrant", proc.Name(), "processor name")
}
