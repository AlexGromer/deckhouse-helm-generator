package detector

import (
	"context"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// DeckhouseDetector detects relationships between Deckhouse CRDs.
// It identifies resources from the deckhouse.io API group and creates
// relationships between them based on shared module context.
type DeckhouseDetector struct {
	priority int
}

// NewDeckhouseDetector creates a new Deckhouse detector.
func NewDeckhouseDetector() *DeckhouseDetector {
	return &DeckhouseDetector{
		priority: 80,
	}
}

// Name returns the detector name.
func (d *DeckhouseDetector) Name() string {
	return "deckhouse"
}

// Priority returns the detector priority.
func (d *DeckhouseDetector) Priority() int {
	return d.priority
}

// Detect detects relationships between Deckhouse CRDs.
func (d *DeckhouseDetector) Detect(ctx context.Context, resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	// Only process deckhouse.io resources
	if !isDeckhouseResource(resource) {
		return nil
	}

	var relationships []types.Relationship
	fromKey := resource.Original.ResourceKey()

	// Create relationships to all other deckhouse.io resources in the input
	for key, other := range allResources {
		if key == fromKey {
			continue
		}
		if !isDeckhouseResource(other) {
			continue
		}

		relationships = append(relationships, types.Relationship{
			From:  fromKey,
			To:    key,
			Type:  types.RelationDeckhouse,
			Field: "apiGroup",
			Details: map[string]string{
				"deckhouse_detected": "true",
			},
		})
	}

	return relationships
}

func isDeckhouseResource(res *types.ProcessedResource) bool {
	if res == nil || res.Original == nil || res.Original.Object == nil {
		return false
	}
	group := res.Original.GVK.Group
	return strings.HasSuffix(group, "deckhouse.io") || group == "deckhouse.io"
}
