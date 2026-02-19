// Package analyzer provides interfaces and implementations for analyzing
// relationships between Kubernetes resources.
package analyzer

import (
	"context"
	"sort"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// Analyzer analyzes processed resources and builds a resource graph with relationships.
type Analyzer interface {
	// Analyze takes processed resources and returns a resource graph with detected relationships.
	Analyze(ctx context.Context, resources []*types.ProcessedResource) (*types.ResourceGraph, error)

	// AddDetector registers a relationship detector.
	AddDetector(d Detector)
}

// Detector detects relationships between resources.
type Detector interface {
	// Detect analyzes a resource and returns detected relationships.
	Detect(ctx context.Context, resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship

	// Name returns the detector name for logging.
	Name() string

	// Priority returns the detector priority (higher = run first).
	Priority() int
}

// DefaultAnalyzer is the default implementation of the Analyzer interface.
type DefaultAnalyzer struct {
	detectors []Detector
}

// NewDefaultAnalyzer creates a new default analyzer.
func NewDefaultAnalyzer() *DefaultAnalyzer {
	return &DefaultAnalyzer{
		detectors: make([]Detector, 0),
	}
}

// AddDetector adds a detector to the analyzer.
func (a *DefaultAnalyzer) AddDetector(d Detector) {
	a.detectors = append(a.detectors, d)
	// Sort by priority (highest first) using stable full sort.
	sort.Slice(a.detectors, func(i, j int) bool {
		return a.detectors[i].Priority() > a.detectors[j].Priority()
	})
}

// Analyze builds a resource graph with detected relationships.
func (a *DefaultAnalyzer) Analyze(ctx context.Context, resources []*types.ProcessedResource) (*types.ResourceGraph, error) {
	graph := types.NewResourceGraph()

	// Build resource map
	resourceMap := make(map[types.ResourceKey]*types.ProcessedResource)
	for _, r := range resources {
		key := r.Original.ResourceKey()
		resourceMap[key] = r
		graph.AddResource(r)
	}

	// Run detectors on each resource
	for _, resource := range resources {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		for _, detector := range a.detectors {
			relationships := detector.Detect(ctx, resource, resourceMap)
			for _, rel := range relationships {
				graph.AddRelationship(rel)
			}
		}
	}

	// Group resources by service
	if err := a.groupResources(graph); err != nil {
		return nil, err
	}

	return graph, nil
}

// groupResources groups resources into services based on relationships and labels.
func (a *DefaultAnalyzer) groupResources(graph *types.ResourceGraph) error {
	grouped := make(map[string]bool)
	serviceMap := make(map[string]*types.ResourceGroup)

	// First pass: create groups from existing service names
	for key, resource := range graph.Resources {
		if resource.ServiceName != "" {
			if _, exists := serviceMap[resource.ServiceName]; !exists {
				serviceMap[resource.ServiceName] = &types.ResourceGroup{
					Name:      resource.ServiceName,
					Resources: make([]*types.ProcessedResource, 0),
					Namespace: key.Namespace,
				}
			}
			serviceMap[resource.ServiceName].Resources = append(serviceMap[resource.ServiceName].Resources, resource)
			grouped[key.String()] = true
		}
	}

	// Second pass: group remaining resources based on relationships
	for key, resource := range graph.Resources {
		if grouped[key.String()] {
			continue
		}

		// Try to find a related service through relationships
		relatedService := a.findRelatedService(key, graph, grouped)
		if relatedService != "" {
			resource.ServiceName = relatedService
			serviceMap[relatedService].Resources = append(serviceMap[relatedService].Resources, resource)
			grouped[key.String()] = true
		}
	}

	// Third pass: remaining resources are orphans or standalone services
	for key, resource := range graph.Resources {
		if grouped[key.String()] {
			continue
		}

		// Create a standalone service for this resource
		serviceName := resource.ServiceName
		if serviceName == "" {
			serviceName = resource.Original.Object.GetName()
			resource.ServiceName = serviceName
		}

		if _, exists := serviceMap[serviceName]; !exists {
			serviceMap[serviceName] = &types.ResourceGroup{
				Name:      serviceName,
				Resources: make([]*types.ProcessedResource, 0),
				Namespace: key.Namespace,
			}
		}
		serviceMap[serviceName].Resources = append(serviceMap[serviceName].Resources, resource)
		grouped[key.String()] = true
	}

	// Add groups to graph
	for _, group := range serviceMap {
		graph.AddGroup(group)
	}

	return nil
}

// findRelatedService finds a service name related to the given resource through relationships.
func (a *DefaultAnalyzer) findRelatedService(key types.ResourceKey, graph *types.ResourceGraph, grouped map[string]bool) string {
	// Check outgoing relationships
	for _, rel := range graph.GetRelationshipsFrom(key) {
		if targetResource, ok := graph.GetResourceByKey(rel.To); ok {
			if targetResource.ServiceName != "" && grouped[rel.To.String()] {
				return targetResource.ServiceName
			}
		}
	}

	// Check incoming relationships
	for _, rel := range graph.GetRelationshipsTo(key) {
		if sourceResource, ok := graph.GetResourceByKey(rel.From); ok {
			if sourceResource.ServiceName != "" && grouped[rel.From.String()] {
				return sourceResource.ServiceName
			}
		}
	}

	return ""
}

// WithDefaultDetectors returns an analyzer with all default detectors registered.
func WithDefaultDetectors() *DefaultAnalyzer {
	a := NewDefaultAnalyzer()

	// Note: Detectors are imported from the detector package
	// and registered by the caller. This function is kept for
	// backward compatibility but should be used with external
	// detector registration.

	return a
}
