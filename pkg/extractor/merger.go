package extractor

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Deduplicate drops resources whose key (GVK, namespace, name) was already
// seen, keeping the first occurrence and the original order. The same object
// in two input files would otherwise produce two templates for one object,
// the later one silently replacing the earlier. It returns the kept
// resources and the dropped duplicates.
func Deduplicate(resources []*types.ExtractedResource) (kept, duplicates []*types.ExtractedResource) {
	seen := make(map[types.ResourceKey]bool, len(resources))
	for _, r := range resources {
		if r == nil || r.Object == nil {
			continue
		}
		key := r.ResourceKey()
		if seen[key] {
			duplicates = append(duplicates, r)
			continue
		}
		seen[key] = true
		kept = append(kept, r)
	}
	return kept, duplicates
}
