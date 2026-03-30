package k8s

// ============================================================
// Test Plan: Workload Identity Detection Processor (Task 5.10.1)
// ============================================================
//
// | #  | Test Name                                                        | Category    | Input                                                      | Expected Output                                                       |
// |----|------------------------------------------------------------------|-------------|-------------------------------------------------------------|-----------------------------------------------------------------------|
// |  1 | TestWorkloadIdentityProcessor_AWSIRSADetected                    | happy       | SA with eks.amazonaws.com/role-arn annotation               | provider="aws", aws.roleArn=value, enabled=true                       |
// |  2 | TestWorkloadIdentityProcessor_GKEWIDetected                      | happy       | SA with iam.gke.io/gcp-service-account annotation           | provider="gcp", gcp.serviceAccount=value, enabled=true                |
// |  3 | TestWorkloadIdentityProcessor_AzureWIDetected                    | happy       | SA with azure.workload.identity/client-id annotation        | provider="azure", azure.clientId=value, enabled=true                  |
// |  4 | TestWorkloadIdentityProcessor_NoAnnotations_Disabled             | happy       | SA with no workload-identity annotations                    | enabled=false                                                         |
// |  5 | TestWorkloadIdentityProcessor_NilObject                          | error       | nil unstructured object                                     | error returned, no panic                                              |
// |  6 | TestWorkloadIdentityProcessor_SupportsServiceAccountGVK          | happy       | call Supports()                                             | contains ServiceAccount v1 GVK                                        |
// |  7 | TestWorkloadIdentityProcessor_ValuesStructureCorrect             | happy       | SA with AWS annotation                                      | workloadIdentity top-level key with enabled, provider, aws sub-map    |
// |  8 | TestWorkloadIdentityProcessor_MultipleAnnotationsFirstWins       | edge        | SA with both AWS and GKE annotations                        | provider="aws" (first matched annotation wins)                        |
// |  9 | TestWorkloadIdentityProcessor_NameAndPriority                    | happy       | constructor                                                 | Name()="workloadidentity", Priority()>0                               |
// | 10 | TestWorkloadIdentityProcessor_NonServiceAccountObject            | edge        | Deployment object passed to Process                         | Processed=true, enabled=false (processor is permissive on any object) |

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helpers
// ============================================================

// makeServiceAccountForWI builds an unstructured ServiceAccount
// with the provided annotations map. Labels default to {app: name}.
func makeServiceAccountForWI(name, namespace string, annotations map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
		"labels":    map[string]interface{}{"app": name},
	}
	if len(annotations) > 0 {
		metadata["annotations"] = annotations
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata":   metadata,
		},
	}
}

// getWIValues extracts the "workloadIdentity" sub-map from Result.Values.
func getWIValues(t *testing.T, result *processor.Result) map[string]interface{} {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	wi, ok := result.Values["workloadIdentity"]
	if !ok {
		t.Fatal("result.Values missing 'workloadIdentity' key")
	}
	wiMap, ok := wi.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Values['workloadIdentity'] is not map[string]interface{}, got %T", wi)
	}
	return wiMap
}

// ============================================================
// Test 1: AWS IRSA annotation detected
// ============================================================

func TestWorkloadIdentityProcessor_AWSIRSADetected(t *testing.T) {
	proc := NewWorkloadIdentityProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	const roleArn = "arn:aws:iam::123456789012:role/my-role"
	obj := makeServiceAccountForWI("my-sa", "default", map[string]interface{}{
		"eks.amazonaws.com/role-arn": roleArn,
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	wiVals := getWIValues(t, result)

	enabled, _ := wiVals["enabled"].(bool)
	if !enabled {
		t.Error("expected workloadIdentity.enabled=true for AWS IRSA annotation")
	}

	provider, _ := wiVals["provider"].(string)
	testutil.AssertEqual(t, "aws", provider, "workloadIdentity.provider")

	awsMap, ok := wiVals["aws"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workloadIdentity.aws to be map[string]interface{}, got %T", wiVals["aws"])
	}
	testutil.AssertEqual(t, roleArn, awsMap["roleArn"], "workloadIdentity.aws.roleArn")
}

// ============================================================
// Test 2: GKE Workload Identity annotation detected
// ============================================================

func TestWorkloadIdentityProcessor_GKEWIDetected(t *testing.T) {
	proc := NewWorkloadIdentityProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	const gcpSA = "my-ksa@my-project.iam.gserviceaccount.com"
	obj := makeServiceAccountForWI("gke-sa", "prod", map[string]interface{}{
		"iam.gke.io/gcp-service-account": gcpSA,
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	wiVals := getWIValues(t, result)

	enabled, _ := wiVals["enabled"].(bool)
	if !enabled {
		t.Error("expected workloadIdentity.enabled=true for GKE WI annotation")
	}

	provider, _ := wiVals["provider"].(string)
	testutil.AssertEqual(t, "gcp", provider, "workloadIdentity.provider")

	gcpMap, ok := wiVals["gcp"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workloadIdentity.gcp to be map[string]interface{}, got %T", wiVals["gcp"])
	}
	testutil.AssertEqual(t, gcpSA, gcpMap["serviceAccount"], "workloadIdentity.gcp.serviceAccount")
}

// ============================================================
// Test 3: Azure Workload Identity annotation detected
// ============================================================

func TestWorkloadIdentityProcessor_AzureWIDetected(t *testing.T) {
	proc := NewWorkloadIdentityProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	const clientID = "11111111-2222-3333-4444-555555555555"
	obj := makeServiceAccountForWI("azure-sa", "default", map[string]interface{}{
		"azure.workload.identity/client-id": clientID,
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	wiVals := getWIValues(t, result)

	enabled, _ := wiVals["enabled"].(bool)
	if !enabled {
		t.Error("expected workloadIdentity.enabled=true for Azure WI annotation")
	}

	provider, _ := wiVals["provider"].(string)
	testutil.AssertEqual(t, "azure", provider, "workloadIdentity.provider")

	azureMap, ok := wiVals["azure"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workloadIdentity.azure to be map[string]interface{}, got %T", wiVals["azure"])
	}
	testutil.AssertEqual(t, clientID, azureMap["clientId"], "workloadIdentity.azure.clientId")
}

// ============================================================
// Test 4: No workload-identity annotations → enabled=false
// ============================================================

func TestWorkloadIdentityProcessor_NoAnnotations_Disabled(t *testing.T) {
	proc := NewWorkloadIdentityProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeServiceAccountForWI("plain-sa", "default", map[string]interface{}{
		"kubectl.kubernetes.io/last-applied-configuration": "{}",
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	wiVals := getWIValues(t, result)

	enabled, _ := wiVals["enabled"].(bool)
	if enabled {
		t.Error("expected workloadIdentity.enabled=false when no workload-identity annotations present")
	}
}

// ============================================================
// Test 5: nil object → error, no panic
// ============================================================

func TestWorkloadIdentityProcessor_NilObject(t *testing.T) {
	proc := NewWorkloadIdentityProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	result, err := proc.Process(ctx, nil)

	if err == nil {
		t.Error("expected error for nil object, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result for nil object, got %+v", result)
	}
}

// ============================================================
// Test 6: Supports() contains ServiceAccount v1 GVK
// ============================================================

func TestWorkloadIdentityProcessor_SupportsServiceAccountGVK(t *testing.T) {
	proc := NewWorkloadIdentityProcessor()
	gvks := proc.Supports()

	if len(gvks) == 0 {
		t.Fatal("expected at least 1 supported GVK, got 0")
	}

	saGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceAccount"}
	found := false
	for _, gvk := range gvks {
		if gvk == saGVK {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Supports() to include ServiceAccount GVK %v, got %v", saGVK, gvks)
	}
}

// ============================================================
// Test 7: Values structure — workloadIdentity key with required sub-keys
// ============================================================

func TestWorkloadIdentityProcessor_ValuesStructureCorrect(t *testing.T) {
	proc := NewWorkloadIdentityProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeServiceAccountForWI("struct-sa", "default", map[string]interface{}{
		"eks.amazonaws.com/role-arn": "arn:aws:iam::000000000000:role/test",
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	if result == nil {
		t.Fatal("result is nil")
	}
	if !result.Processed {
		t.Error("expected Result.Processed=true")
	}

	wiVals := getWIValues(t, result)

	// required top-level sub-keys
	for _, key := range []string{"enabled", "provider"} {
		if _, ok := wiVals[key]; !ok {
			t.Errorf("expected workloadIdentity.%s key to be present in Values", key)
		}
	}

	// aws provider sub-map must contain roleArn
	awsMap, ok := wiVals["aws"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workloadIdentity.aws map, got %T", wiVals["aws"])
	}
	if _, ok := awsMap["roleArn"]; !ok {
		t.Error("expected workloadIdentity.aws.roleArn key to be present")
	}
}

// ============================================================
// Test 8: Multiple provider annotations — first matched wins (AWS > GCP > Azure)
// ============================================================

func TestWorkloadIdentityProcessor_MultipleAnnotationsFirstWins(t *testing.T) {
	proc := NewWorkloadIdentityProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	// Both AWS and GKE annotations present — AWS must take precedence.
	obj := makeServiceAccountForWI("multi-sa", "default", map[string]interface{}{
		"eks.amazonaws.com/role-arn":      "arn:aws:iam::123:role/r",
		"iam.gke.io/gcp-service-account": "sa@proj.iam.gserviceaccount.com",
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	wiVals := getWIValues(t, result)

	provider, _ := wiVals["provider"].(string)
	testutil.AssertEqual(t, "aws", provider, "first matched provider should be aws")
}

// ============================================================
// Test 9: Name() and Priority()
// ============================================================

func TestWorkloadIdentityProcessor_NameAndPriority(t *testing.T) {
	proc := NewWorkloadIdentityProcessor()

	testutil.AssertEqual(t, "workloadidentity", proc.Name(), "processor name")
	if proc.Priority() <= 0 {
		t.Errorf("expected Priority() > 0, got %d", proc.Priority())
	}
}

// ============================================================
// Test 10: Non-ServiceAccount object (Deployment) — Processed=true, enabled=false
// ============================================================

func TestWorkloadIdentityProcessor_NonServiceAccountObject(t *testing.T) {
	proc := NewWorkloadIdentityProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	// Deployment has no workload-identity annotations; processor should not panic.
	obj := makeDeploymentForIstio("myapp", "default", nil, []interface{}{appContainer("myapp")})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	if result == nil {
		t.Fatal("expected non-nil result for non-SA object")
	}
	// Processor is passed a non-SA object; it should still return Processed=true
	// (it was invoked) and enabled=false (no matching annotations).
	wiVals := getWIValues(t, result)
	enabled, _ := wiVals["enabled"].(bool)
	if enabled {
		t.Error("expected workloadIdentity.enabled=false for object with no WI annotations")
	}
}
