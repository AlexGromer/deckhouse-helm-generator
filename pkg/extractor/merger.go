package extractor

import (
	"fmt"
	"sort"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ConflictStrategy defines how duplicate resources from different sources are handled.
type ConflictStrategy string

const (
	// ConflictStrategyError returns an error when duplicates are found.
	ConflictStrategyError ConflictStrategy = "error"

	// ConflictStrategyWarn logs a warning and keeps the higher-priority source.
	ConflictStrategyWarn ConflictStrategy = "warn"

	// ConflictStrategyMerge attempts to merge duplicate resources.
	ConflictStrategyMerge ConflictStrategy = "merge"
)

// ValidConflictStrategies returns all valid conflict strategy values.
func ValidConflictStrategies() []ConflictStrategy {
	return []ConflictStrategy{ConflictStrategyError, ConflictStrategyWarn, ConflictStrategyMerge}
}

// IsValidConflictStrategy checks if a string is a valid conflict strategy.
func IsValidConflictStrategy(s string) bool {
	for _, v := range ValidConflictStrategies() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// SourcePriority defines the priority order for resource sources.
// Lower numeric value = higher priority.
type SourcePriority struct {
	priorities map[types.Source]int
}

// DefaultSourcePriority returns the default priority: cluster > file > gitops.
func DefaultSourcePriority() *SourcePriority {
	return &SourcePriority{
		priorities: map[types.Source]int{
			types.SourceCluster: 0,
			types.SourceFile:    1,
			types.SourceGitOps:  2,
		},
	}
}

// NewSourcePriority creates a SourcePriority from an ordered slice of sources.
// The first source has the highest priority.
func NewSourcePriority(sources []types.Source) *SourcePriority {
	p := &SourcePriority{
		priorities: make(map[types.Source]int, len(sources)),
	}
	for i, s := range sources {
		p.priorities[s] = i
	}
	return p
}

// Priority returns the priority for a source. Lower value = higher priority.
// Unknown sources get priority 999.
func (sp *SourcePriority) Priority(source types.Source) int {
	if p, ok := sp.priorities[source]; ok {
		return p
	}
	return 999
}

// Higher returns true if source a has higher priority than source b.
func (sp *SourcePriority) Higher(a, b types.Source) bool {
	return sp.Priority(a) < sp.Priority(b)
}

// DeduplicateConflict describes a conflict found during deduplication.
type DeduplicateConflict struct {
	// Key is the resource key that had duplicates.
	Key types.ResourceKey

	// Sources lists the sources that had this resource.
	Sources []types.Source

	// Kept is the source that was kept.
	Kept types.Source
}

// ResourceDeduplicator removes duplicate resources based on GVK+namespace+name.
type ResourceDeduplicator struct {
	// Strategy determines how conflicts are handled.
	Strategy ConflictStrategy

	// Priority determines which source wins on conflict.
	Priority *SourcePriority

	// Conflicts collects all conflicts found during deduplication.
	Conflicts []DeduplicateConflict
}

// NewResourceDeduplicator creates a deduplicator with default settings (warn + default priority).
func NewResourceDeduplicator() *ResourceDeduplicator {
	return &ResourceDeduplicator{
		Strategy: ConflictStrategyWarn,
		Priority: DefaultSourcePriority(),
	}
}

// Deduplicate removes duplicate resources, keeping the one from the highest-priority source.
// Resources are identified by GVK + namespace + name.
func (d *ResourceDeduplicator) Deduplicate(resources []*types.ExtractedResource) ([]*types.ExtractedResource, error) {
	d.Conflicts = nil // Reset conflicts

	type entry struct {
		resource *types.ExtractedResource
		index    int // original position for stable sort
	}

	seen := make(map[types.ResourceKey]*entry, len(resources))

	for i, r := range resources {
		if r == nil || r.Object == nil {
			continue
		}
		key := r.ResourceKey()

		existing, exists := seen[key]
		if !exists {
			seen[key] = &entry{resource: r, index: i}
			continue
		}

		// Conflict found
		conflict := DeduplicateConflict{
			Key:     key,
			Sources: []types.Source{existing.resource.Source, r.Source},
		}

		switch d.Strategy {
		case ConflictStrategyError:
			return nil, fmt.Errorf("duplicate resource %s found in sources %q and %q",
				key, existing.resource.Source, r.Source)
		case ConflictStrategyWarn, ConflictStrategyMerge:
			// Keep higher priority source
			if d.Priority.Higher(r.Source, existing.resource.Source) {
				conflict.Kept = r.Source
				seen[key] = &entry{resource: r, index: i}
			} else {
				conflict.Kept = existing.resource.Source
			}
		}

		d.Conflicts = append(d.Conflicts, conflict)
	}

	// Collect results maintaining original order (by first appearance of winning entry)
	type sortEntry struct {
		resource *types.ExtractedResource
		index    int
	}
	sorted := make([]sortEntry, 0, len(seen))
	for _, e := range seen {
		sorted = append(sorted, sortEntry{resource: e.resource, index: e.index})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].index < sorted[j].index
	})

	result := make([]*types.ExtractedResource, 0, len(sorted))
	for _, e := range sorted {
		result = append(result, e.resource)
	}

	return result, nil
}
