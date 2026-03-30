package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// WorkloadIdentityProcessor detects cloud provider workload identity annotations
// on ServiceAccount resources. It supports:
//   - AWS IRSA: eks.amazonaws.com/role-arn
//   - GKE Workload Identity: iam.gke.io/gcp-service-account
//   - Azure Workload Identity: azure.workload.identity/client-id
//
// Values produced: workloadIdentity.enabled, .provider, .aws.roleArn / .gcp.serviceAccount / .azure.clientId
type WorkloadIdentityProcessor struct {
	processor.BaseProcessor
}

// NewWorkloadIdentityProcessor creates a new WorkloadIdentityProcessor.
func NewWorkloadIdentityProcessor() *WorkloadIdentityProcessor {
	return &WorkloadIdentityProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"workloadidentity",
			50,
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceAccount"},
		),
	}
}

// Process inspects the object for workload identity annotations and returns values.
func (p *WorkloadIdentityProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("workloadidentity: object is nil")
	}

	// Retrieve annotations from the object (works for any resource kind).
	annotations := obj.GetAnnotations()

	wiValues := map[string]interface{}{
		"enabled":  false,
		"provider": "",
	}

	// Check providers in priority order: AWS > GCP > Azure.
	if roleArn, ok := annotations["eks.amazonaws.com/role-arn"]; ok && roleArn != "" {
		wiValues["enabled"] = true
		wiValues["provider"] = "aws"
		wiValues["aws"] = map[string]interface{}{
			"roleArn": roleArn,
		}
	} else if gcpSA, ok := annotations["iam.gke.io/gcp-service-account"]; ok && gcpSA != "" {
		wiValues["enabled"] = true
		wiValues["provider"] = "gcp"
		wiValues["gcp"] = map[string]interface{}{
			"serviceAccount": gcpSA,
		}
	} else if clientID, ok := annotations["azure.workload.identity/client-id"]; ok && clientID != "" {
		wiValues["enabled"] = true
		wiValues["provider"] = "azure"
		wiValues["azure"] = map[string]interface{}{
			"clientId": clientID,
		}
	}

	values := map[string]interface{}{
		"workloadIdentity": wiValues,
	}

	return &processor.Result{
		Processed:   true,
		ServiceName: processor.SanitizeServiceName(processor.ServiceNameFromResource(obj)),
		Values:      values,
	}, nil
}
