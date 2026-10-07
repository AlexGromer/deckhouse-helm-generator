package generator

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
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

// LoadTemplateOverrides reads all .yaml, .yml, .tpl and .txt files below dir
// (recursively) and returns them keyed like chart templates:
// <dir>/hooks/job.yaml → "templates/hooks/job.yaml". An empty dir returns an
// empty map without error; a non-existent directory is an error.
func LoadTemplateOverrides(dir string) (map[string]string, error) {
	if dir == "" {
		return map[string]string{}, nil
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("template overrides: %w", err)
	}

	overrides := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch filepath.Ext(path) {
		case ".yaml", ".yml", ".tpl", ".txt":
		default:
			return nil
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		overrides["templates/"+filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("template overrides: read %q: %w", dir, err)
	}
	return overrides, nil
}

// ApplyTemplateOverrides merges overrides into a chart's templates.
// templates/_helpers.tpl and templates/NOTES.txt address the chart's Helpers
// and Notes, which are stored separately from Templates. The input chart is
// not mutated.
func ApplyTemplateOverrides(chart *types.GeneratedChart, overrides map[string]string, strategy string) *types.GeneratedChart {
	out := cloneChart(chart)
	special := map[string]*string{
		"templates/_helpers.tpl": &out.Helpers,
		"templates/NOTES.txt":    &out.Notes,
	}
	regular := make(map[string]string, len(overrides))
	for key, content := range overrides {
		target, ok := special[key]
		if !ok {
			regular[key] = content
			continue
		}
		merged := MergeTemplateOverrides(map[string]string{key: *target}, map[string]string{key: content}, strategy)
		*target = merged[key]
	}
	out.Templates = MergeTemplateOverrides(out.Templates, regular, strategy)
	return out
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
