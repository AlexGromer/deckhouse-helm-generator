package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create InstanceClass unstructured object
// ============================================================

func makeInstanceClassObj(kind, name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1",
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": spec,
		},
	}
}

// ============================================================
// Test: OpenStack extraction
// ============================================================

func TestInstanceClass_OpenstackExtraction(t *testing.T) {
	proc := NewOpenstackInstanceClassProcessor()
	ctx := newTestProcessorContext()

	obj := makeInstanceClassObj("OpenStackInstanceClass", "worker", map[string]interface{}{
		"flavorName":   "m1.large",
		"imageName":    "ubuntu-22.04",
		"rootDiskSize": int64(50),
		"mainNetwork":  "kube-network",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "worker", result.ServiceName, "serviceName")

	testutil.AssertEqual(t, "m1.large", result.Values["flavorName"], "flavorName")
	testutil.AssertEqual(t, "ubuntu-22.04", result.Values["imageName"], "imageName")
	testutil.AssertEqual(t, int64(50), result.Values["rootDiskSize"], "rootDiskSize")
	testutil.AssertEqual(t, "kube-network", result.Values["mainNetwork"], "mainNetwork")

	// Check GVK
	gvks := proc.Supports()
	expected := schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "OpenStackInstanceClass"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test: AWS extraction
// ============================================================

func TestInstanceClass_AWSExtraction(t *testing.T) {
	proc := NewAWSInstanceClassProcessor()
	ctx := newTestProcessorContext()

	obj := makeInstanceClassObj("AWSInstanceClass", "worker", map[string]interface{}{
		"instanceType": "m5.xlarge",
		"imageName":    "ami-12345678",
		"rootDiskSize": int64(100),
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	testutil.AssertEqual(t, "m5.xlarge", result.Values["instanceType"], "instanceType")
	testutil.AssertEqual(t, "ami-12345678", result.Values["imageName"], "imageName")
	testutil.AssertEqual(t, int64(100), result.Values["rootDiskSize"], "rootDiskSize")

	// Check GVK
	gvks := proc.Supports()
	expected := schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "AWSInstanceClass"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test: GCP extraction
// ============================================================

func TestInstanceClass_GCPExtraction(t *testing.T) {
	proc := NewGCPInstanceClassProcessor()
	ctx := newTestProcessorContext()

	obj := makeInstanceClassObj("GCPInstanceClass", "worker", map[string]interface{}{
		"machineType":  "n1-standard-4",
		"imageName":    "ubuntu-2204-lts",
		"rootDiskSize": int64(80),
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	testutil.AssertEqual(t, "n1-standard-4", result.Values["machineType"], "machineType")
	testutil.AssertEqual(t, "ubuntu-2204-lts", result.Values["imageName"], "imageName")
	testutil.AssertEqual(t, int64(80), result.Values["rootDiskSize"], "rootDiskSize")

	// Check GVK
	gvks := proc.Supports()
	expected := schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "GCPInstanceClass"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test: Template content
// ============================================================

func TestInstanceClass_Template(t *testing.T) {
	proc := NewOpenstackInstanceClassProcessor()
	ctx := newTestProcessorContext()

	obj := makeInstanceClassObj("OpenStackInstanceClass", "worker", map[string]interface{}{
		"flavorName": "m1.large",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: deckhouse.io/v1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: OpenStackInstanceClass", "kind")
	if !strings.Contains(tpl, "worker") {
		t.Error("Template should contain resource name")
	}
}
