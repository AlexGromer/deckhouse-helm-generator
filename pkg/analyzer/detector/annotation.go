package detector

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// AnnotationDetector detects relationships based on annotations.
// Example: Ingress with cert-manager annotations referencing ClusterIssuer.
type AnnotationDetector struct {
	priority int
}

// NewAnnotationDetector creates a new annotation detector.
func NewAnnotationDetector() *AnnotationDetector {
	return &AnnotationDetector{
		priority: 70,
	}
}

// Name returns the detector name.
func (d *AnnotationDetector) Name() string {
	return "annotation"
}

// Priority returns the detector priority.
func (d *AnnotationDetector) Priority() int {
	return d.priority
}

// Detect detects annotation-based relationships.
func (d *AnnotationDetector) Detect(ctx context.Context, resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	annotations := obj.GetAnnotations()

	if len(annotations) == 0 {
		return relationships
	}

	// cert-manager annotations
	relationships = append(relationships, d.detectCertManagerReferences(resource, annotations, allResources)...)

	// Prometheus annotations
	relationships = append(relationships, d.detectPrometheusReferences(resource, annotations, allResources)...)

	// Deckhouse-specific annotations
	relationships = append(relationships, d.detectDeckhouseReferences(resource, annotations, allResources)...)

	// Custom dependency annotations (dhg.deckhouse.io/depends-on)
	relationships = append(relationships, d.detectCustomDependencyAnnotations(resource, annotations, allResources)...)

	return relationships
}

// detectCertManagerReferences detects cert-manager annotation-based relationships.
func (d *AnnotationDetector) detectCertManagerReferences(resource *types.ProcessedResource, annotations map[string]string, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	namespace := obj.GetNamespace()

	// cert-manager.io/cluster-issuer
	if clusterIssuer, ok := annotations["cert-manager.io/cluster-issuer"]; ok && clusterIssuer != "" {
		targetKey := types.ResourceKey{
			GVK:  schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "ClusterIssuer"},
			Name: clusterIssuer,
		}

		if _, exists := allResources[targetKey]; exists {
			relationships = append(relationships, types.Relationship{
				From: resource.Original.ResourceKey(),
				To:   targetKey,
				Type: types.RelationAnnotation,
				Field: "metadata.annotations[cert-manager.io/cluster-issuer]",
				Details: map[string]string{
					"clusterIssuer": clusterIssuer,
					"annotation":    "cert-manager.io/cluster-issuer",
				},
			})
		}
	}

	// cert-manager.io/issuer
	if issuer, ok := annotations["cert-manager.io/issuer"]; ok && issuer != "" {
		targetKey := types.ResourceKey{
			GVK:       schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "Issuer"},
			Namespace: namespace,
			Name:      issuer,
		}

		if _, exists := allResources[targetKey]; exists {
			relationships = append(relationships, types.Relationship{
				From: resource.Original.ResourceKey(),
				To:   targetKey,
				Type: types.RelationAnnotation,
				Field: "metadata.annotations[cert-manager.io/issuer]",
				Details: map[string]string{
					"issuer":     issuer,
					"annotation": "cert-manager.io/issuer",
				},
			})
		}
	}

	return relationships
}

// detectPrometheusReferences detects Prometheus annotation-based relationships.
func (d *AnnotationDetector) detectPrometheusReferences(resource *types.ProcessedResource, annotations map[string]string, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	// Check for Prometheus scrape annotations
	// These don't create direct relationships but are metadata
	// that might be useful for grouping

	prometheusAnnotations := []string{
		"prometheus.io/scrape",
		"prometheus.io/path",
		"prometheus.io/port",
		"prometheus.io/scheme",
	}

	hasPrometheusAnnotations := false
	for _, ann := range prometheusAnnotations {
		if _, ok := annotations[ann]; ok {
			hasPrometheusAnnotations = true
			break
		}
	}

	// If resource has Prometheus annotations, we might want to track this
	// for ServiceMonitor generation hints, but no direct relationship
	_ = hasPrometheusAnnotations

	return relationships
}

// detectDeckhouseReferences detects Deckhouse-specific annotation relationships.
func (d *AnnotationDetector) detectDeckhouseReferences(resource *types.ProcessedResource, annotations map[string]string, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	namespace := obj.GetNamespace()

	// Check for Deckhouse ingress controller references
	for key, value := range annotations {
		// nginx.ingress.kubernetes.io/* annotations
		if strings.HasPrefix(key, "nginx.ingress.kubernetes.io/") {
			// Check if there's an IngressNginxController in the cluster
			for targetKey := range allResources {
				if targetKey.GVK.Kind == "IngressNginxController" {
					relationships = append(relationships, types.Relationship{
						From: resource.Original.ResourceKey(),
						To:   targetKey,
						Type: types.RelationDeckhouse,
						Field: "metadata.annotations[" + key + "]",
						Details: map[string]string{
							"annotation":      key,
							"annotationValue": value,
						},
					})
					break // Only add one relationship per annotation prefix
				}
			}
		}

		// deckhouse.io/* annotations
		if strings.HasPrefix(key, "deckhouse.io/") {
			// Try to infer related Deckhouse resources
			// This is heuristic-based
			if strings.Contains(key, "auth") || strings.Contains(key, "dex") {
				for targetKey := range allResources {
					if targetKey.GVK.Kind == "DexAuthenticator" && targetKey.Namespace == namespace {
						relationships = append(relationships, types.Relationship{
							From: resource.Original.ResourceKey(),
							To:   targetKey,
							Type: types.RelationDeckhouse,
							Field: "metadata.annotations[" + key + "]",
							Details: map[string]string{
								"annotation":      key,
								"annotationValue": value,
							},
						})
						break
					}
				}
			}
		}
	}

	return relationships
}

// detectCustomDependencyAnnotations detects custom dependency relationships via
// dhg.deckhouse.io/depends-on annotation. The value should be a resource name in the
// same namespace (format: "kind/name" or just "name" for auto-detection).
func (d *AnnotationDetector) detectCustomDependencyAnnotations(resource *types.ProcessedResource, annotations map[string]string, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	dependsOn, ok := annotations["dhg.deckhouse.io/depends-on"]
	if !ok || dependsOn == "" {
		return relationships
	}

	namespace := resource.Original.Object.GetNamespace()

	// Try to find the referenced resource by name in allResources
	for targetKey := range allResources {
		if targetKey.Name == dependsOn && (targetKey.Namespace == namespace || targetKey.Namespace == "") {
			relationships = append(relationships, types.Relationship{
				From:  resource.Original.ResourceKey(),
				To:    targetKey,
				Type:  types.RelationCustomDependency,
				Field: "metadata.annotations[dhg.deckhouse.io/depends-on]",
				Details: map[string]string{
					"dependsOn": dependsOn,
				},
			})
		}
	}

	return relationships
}
