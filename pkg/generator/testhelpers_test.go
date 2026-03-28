package generator

import (
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ============================================================
// Shared Test Helpers
// ============================================================

// makeDeploymentWithImage creates a ProcessedResource representing a Deployment
// whose first container uses the given image string.
func makeDeploymentWithImage(name, image string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind("Deployment")
	obj.SetName(name)
	obj.SetAPIVersion("apps/v1")
	obj.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": image,
					},
				},
			},
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK: schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			},
		},
	}
}

// makeDeploymentWithEnv creates a Deployment ProcessedResource with env vars.
func makeDeploymentWithEnv(name, namespace string, envVars map[string]string) *types.ProcessedResource {
	r := makeProcessedResource("Deployment", name, namespace, nil)
	envList := make([]interface{}, 0, len(envVars))
	for k, v := range envVars {
		envList = append(envList, map[string]interface{}{
			"name":  k,
			"value": v,
		})
	}
	r.Original.Object.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": name + ":latest",
						"env":   envList,
					},
				},
			},
		},
	}
	return r
}
