package generator

import (
	"fmt"
	"os"
	"path/filepath"
)

// MergeStrategy controls how template overrides are applied to generated content.
type MergeStrategy string

const (
	// MergeStrategyOverride replaces the generated content with the override.
	MergeStrategyOverride MergeStrategy = "override"

	// MergeStrategyAppend appends the override content after the generated content.
	MergeStrategyAppend MergeStrategy = "append"

	// MergeStrategyPrepend inserts the override content before the generated content.
	MergeStrategyPrepend MergeStrategy = "prepend"
)

// LoadTemplateOverrides reads all .yaml and .tpl files from dir and returns a
// map of relative file path → content. An empty string dir returns an empty map
// without error. A non-existent directory returns an error.
func LoadTemplateOverrides(dir string) (map[string]string, error) {
	if dir == "" {
		return map[string]string{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("template overrides: read dir %q: %w", dir, err)
	}

	overrides := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".tpl" {
			continue
		}

		fullPath := filepath.Join(dir, name)
		data, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			return nil, fmt.Errorf("template overrides: read file %q: %w", fullPath, readErr)
		}

		overrides[fullPath] = string(data)
	}

	return overrides, nil
}

// MergeTemplateOverrides merges override templates into the generated map using
// the specified strategy. The generated map is not mutated; a new map is returned.
//
//   - override (default): override content replaces generated content.
//   - append: override content is appended after generated content.
//   - prepend: override content is inserted before generated content.
//
// Overrides whose key does not exist in generated are always added as-is.
// If generated is nil it is treated as an empty map.
// If strategy is empty, MergeStrategyOverride is used.
func MergeTemplateOverrides(generated map[string]string, overrides map[string]string, strategy string) map[string]string {
	result := make(map[string]string)

	// Copy all generated entries first.
	for k, v := range generated {
		result[k] = v
	}

	if len(overrides) == 0 {
		return result
	}

	strat := MergeStrategy(strategy)
	if strat == "" {
		strat = MergeStrategyOverride
	}

	for key, overrideContent := range overrides {
		existingContent, exists := result[key]
		if !exists {
			// Key not in generated — add unconditionally.
			result[key] = overrideContent
			continue
		}

		switch strat {
		case MergeStrategyAppend:
			result[key] = existingContent + overrideContent
		case MergeStrategyPrepend:
			result[key] = overrideContent + existingContent
		default: // MergeStrategyOverride and anything unrecognised
			result[key] = overrideContent
		}
	}

	return result
}
