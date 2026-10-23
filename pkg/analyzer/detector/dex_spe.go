package detector

import (
	"context"
	"net/url"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DexAndSPEDetector detects relationships for DexAuthenticator and SecurityPolicyException.
type DexAndSPEDetector struct {
	priority int
}

// NewDexAndSPEDetector creates a new Dex and SPE detector.
func NewDexAndSPEDetector() *DexAndSPEDetector {
	return &DexAndSPEDetector{
		priority: 75,
	}
}

// Name returns the detector name.
func (d *DexAndSPEDetector) Name() string {
	return "dex_spe"
}

// Priority returns the detector priority.
func (d *DexAndSPEDetector) Priority() int {
	return d.priority
}

// Detect detects relationships for DexAuthenticator and SecurityPolicyException.
func (d *DexAndSPEDetector) Detect(ctx context.Context, resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	annotations := obj.GetAnnotations()

	// 1. DexAuthenticator detection
	if obj.GetKind() == "Ingress" || obj.GetKind() == "HTTPRoute" {
		for key, value := range annotations {
			if key == "nginx.ingress.kubernetes.io/auth-url" ||
				key == "nginx.ingress.kubernetes.io/auth-signin" ||
				key == "alb.network.deckhouse.io/auth-url" {

				name, ns, ok := d.dexAuthenticatorFromAuthURL(value)
				if ok {
					if ns == "" {
						ns = obj.GetNamespace()
					}

					targetKeyV1 := types.ResourceKey{
						GVK:       schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "DexAuthenticator"},
						Namespace: ns,
						Name:      name,
					}
					targetKeyV2 := types.ResourceKey{
						GVK:       schema.GroupVersionKind{Group: "deckhouse.io", Version: "v2alpha1", Kind: "DexAuthenticator"},
						Namespace: ns,
						Name:      name,
					}

					var targetKey *types.ResourceKey
					if _, exists := allResources[targetKeyV1]; exists {
						targetKey = &targetKeyV1
					} else if _, exists := allResources[targetKeyV2]; exists {
						targetKey = &targetKeyV2
					}

					if targetKey != nil {
						relationships = append(relationships, types.Relationship{
							From:  resource.Original.ResourceKey(),
							To:    *targetKey,
							Type:  types.RelationDeckhouse,
							Field: "metadata.annotations[" + key + "]",
							Details: map[string]string{
								"annotation":      key,
								"annotationValue": value,
							},
						})
					}
				}
			}
		}
	}

	// 2. SecurityPolicyException detection
	relationships = append(relationships, d.detectSecurityPolicyExceptionReferences(resource, allResources)...)

	return relationships
}

func (d *DexAndSPEDetector) dexAuthenticatorFromAuthURL(raw string) (string, string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", false
	}

	host := u.Host
	if !strings.Contains(host, ".svc") {
		return "", "", false
	}

	parts := strings.Split(host, ".")
	if len(parts) < 3 {
		return "", "", false
	}

	serviceName := parts[0]
	ns := parts[1]

	if strings.HasSuffix(serviceName, "-dex-authenticator") {
		name := strings.TrimSuffix(serviceName, "-dex-authenticator")
		return name, ns, true
	}

	return "", "", false
}

func (d *DexAndSPEDetector) detectSecurityPolicyExceptionReferences(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	kind := obj.GetKind()
	namespace := obj.GetNamespace()

	var labels map[string]string
	var found bool

	// Check for pod template labels
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		if tmplLabels, ok, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels"); ok {
			labels = tmplLabels
			found = true
		}
	case "CronJob":
		if tmplLabels, ok, _ := unstructured.NestedStringMap(obj.Object, "spec", "jobTemplate", "spec", "template", "metadata", "labels"); ok {
			labels = tmplLabels
			found = true
		}
	case "Pod":
		labels = obj.GetLabels()
		found = true
	}

	if !found {
		// DKP says "на PodTemplate или метаданных workload'а". Check metadata labels too.
		if len(labels) == 0 {
			labels = obj.GetLabels()
		}
	}

	for key, value := range labels {
		if key == "security.deckhouse.io/security-policy-exception" || strings.HasPrefix(key, "security.deckhouse.io/security-policy-exception.container.") {
			targetKey := types.ResourceKey{
				GVK:       schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1alpha1", Kind: "SecurityPolicyException"},
				Namespace: namespace,
				Name:      value,
			}

			if _, exists := allResources[targetKey]; exists {
				field := "spec.template.metadata.labels[" + key + "]"
				if kind == "CronJob" {
					field = "spec.jobTemplate.spec.template.metadata.labels[" + key + "]"
				} else if kind == "Pod" {
					field = "metadata.labels[" + key + "]"
				} else if !found {
					field = "metadata.labels[" + key + "]"
				}

				relationships = append(relationships, types.Relationship{
					From:  resource.Original.ResourceKey(),
					To:    targetKey,
					Type:  types.RelationNameReference,
					Field: field,
					Details: map[string]string{
						"exception": value,
					},
				})
			}
		}
	}

	return relationships
}
