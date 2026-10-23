package detector

import (
	"context"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// DexAndSPEDetector detects relationships for DexAuthenticator and SecurityPolicyException.
type DexAndSPEDetector struct {
	priority int
}

func NewDexAndSPEDetector() *DexAndSPEDetector {
	return &DexAndSPEDetector{
		priority: 75,
	}
}

func (d *DexAndSPEDetector) Name() string {
	return "dex-spe"
}

func (d *DexAndSPEDetector) Priority() int {
	return d.priority
}

func (d *DexAndSPEDetector) Detect(ctx context.Context, resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var rels []types.Relationship

	// Detect Ingress -> DexAuthenticator
	if resource.Original.GVK.Kind == "Ingress" || resource.Original.GVK.Kind == "HTTPRoute" {
		rels = append(rels, d.detectDexAuthenticator(resource, allResources)...)
	}

	// Detect Workload -> SecurityPolicyException
	switch resource.Original.GVK.Kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob", "Pod":
		rels = append(rels, d.detectSecurityPolicyExceptionReferences(resource, allResources)...)
	}

	return rels
}

func dexAuthenticatorFromAuthURL(raw string) (name, ns string, ok bool) {
	if raw == "" {
		return "", "", false
	}
	// Example: https://orders-dex-authenticator.shop.svc.cluster.local/dex-authenticator/auth
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	parts := strings.SplitN(raw, "/", 2)
	host := parts[0]

	if !strings.HasSuffix(host, ".svc.cluster.local") && !strings.HasSuffix(host, ".svc") {
		return "", "", false
	}

	hostParts := strings.Split(host, ".")
	if len(hostParts) < 2 {
		return "", "", false
	}

	svcName := hostParts[0]
	svcNs := hostParts[1]

	if strings.HasSuffix(svcName, "-dex-authenticator") {
		return strings.TrimSuffix(svcName, "-dex-authenticator"), svcNs, true
	}

	return "", "", false
}

func (d *DexAndSPEDetector) detectDexAuthenticator(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var rels []types.Relationship
	annotations := resource.Original.Object.GetAnnotations()
	if len(annotations) == 0 {
		return rels
	}

	urls := []string{
		annotations["nginx.ingress.kubernetes.io/auth-url"],
		annotations["nginx.ingress.kubernetes.io/auth-signin"],
		annotations["alb.network.deckhouse.io/auth-url"],
		annotations["alb.network.deckhouse.io/auth-signin"],
	}

	for _, raw := range urls {
		if raw == "" {
			continue
		}

		name, ns, ok := dexAuthenticatorFromAuthURL(raw)
		if !ok {
			continue
		}

		if ns != resource.Original.ResourceKey().Namespace {
			continue
		}

		// Find DexAuthenticator in allResources
		for key := range allResources {
			if key.GVK.Group == "deckhouse.io" && key.GVK.Kind == "DexAuthenticator" && key.Namespace == ns && key.Name == name {
				rels = append(rels, types.Relationship{
					From:  resource.Original.ResourceKey(),
					To:    key,
					Type:  types.RelationDeckhouse,
					Field: "metadata.annotations", // We can't be precise without knowing which URL matched, but this is fine.
					Details: map[string]string{
						"annotation":      "auth-url/auth-signin",
						"annotationValue": raw,
					},
				})
			}
		}
	}

	return rels
}

func (d *DexAndSPEDetector) detectSecurityPolicyExceptionReferences(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var rels []types.Relationship

	labels := getPodTemplateLabels(resource.Original.Object.Object)
	if len(labels) == 0 {
		return rels
	}

	for k, v := range labels {
		if k == "security.deckhouse.io/security-policy-exception" || strings.HasPrefix(k, "security.deckhouse.io/security-policy-exception.container.") {
			// Find SPE in allResources
			for key := range allResources {
				if key.GVK.Group == "deckhouse.io" && key.GVK.Kind == "SecurityPolicyException" && key.Name == v && key.Namespace == resource.Original.ResourceKey().Namespace {
					rels = append(rels, types.Relationship{
						From:  resource.Original.ResourceKey(),
						To:    key,
						Type:  types.RelationNameReference,
						Field: "spec.template.metadata.labels[" + k + "]",
					})
				}
			}
		}
	}

	return rels
}

func getPodTemplateLabels(obj map[string]interface{}) map[string]string {
	res := make(map[string]string)

	// Try extracting from jobTemplate (CronJob)
	if jobTemplate, ok := obj["spec"].(map[string]interface{})["jobTemplate"].(map[string]interface{}); ok {
		if spec, ok := jobTemplate["spec"].(map[string]interface{}); ok {
			if tmpl, ok := spec["template"].(map[string]interface{}); ok {
				if meta, ok := tmpl["metadata"].(map[string]interface{}); ok {
					if l, ok := meta["labels"].(map[string]interface{}); ok {
						for k, v := range l {
							if str, ok := v.(string); ok {
								res[k] = str
							}
						}
					}
					if a, ok := meta["annotations"].(map[string]interface{}); ok {
						for k, v := range a {
							if str, ok := v.(string); ok {
								res[k] = str
							}
						}
					}
				}
			}
		}
	} else if spec, ok := obj["spec"].(map[string]interface{}); ok {
		if tmpl, ok := spec["template"].(map[string]interface{}); ok {
			if meta, ok := tmpl["metadata"].(map[string]interface{}); ok {
				if l, ok := meta["labels"].(map[string]interface{}); ok {
					for k, v := range l {
						if str, ok := v.(string); ok {
							res[k] = str
						}
					}
				}
				if a, ok := meta["annotations"].(map[string]interface{}); ok {
					for k, v := range a {
						if str, ok := v.(string); ok {
							res[k] = str
						}
					}
				}
			}
		}
	}

	// Also check top-level metadata
	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		if l, ok := meta["labels"].(map[string]interface{}); ok {
			for k, v := range l {
				if str, ok := v.(string); ok {
					res[k] = str
				}
			}
		}
		if a, ok := meta["annotations"].(map[string]interface{}); ok {
			for k, v := range a {
				if str, ok := v.(string); ok {
					res[k] = str
				}
			}
		}
	}

	return res
}
