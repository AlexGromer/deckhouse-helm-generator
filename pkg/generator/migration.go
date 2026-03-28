package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"sigs.k8s.io/yaml"
)

// DriftCategory classifies the type of change detected between charts.
type DriftCategory string

const (
	DriftAdded   DriftCategory = "added"
	DriftRemoved DriftCategory = "removed"
	DriftChanged DriftCategory = "changed"
)

// DriftItem describes a single difference between an existing and new chart.
type DriftItem struct {
	Category DriftCategory
	Path     string
	Detail   string
}

// DriftReport holds all detected differences between two charts.
type DriftReport struct {
	Templates []DriftItem
	Values    []DriftItem
	Helpers   []DriftItem
}

// HasDrift returns true if any differences were detected.
func (r DriftReport) HasDrift() bool {
	return len(r.Templates) > 0 || len(r.Values) > 0 || len(r.Helpers) > 0
}

// TotalItems returns the total number of drift items across all categories.
func (r DriftReport) TotalItems() int {
	return len(r.Templates) + len(r.Values) + len(r.Helpers)
}

// DetectDrift compares an existing chart with a newly generated chart and returns
// a structured report of all differences: templates (added/removed/changed),
// values (added/removed/changed keys), and helpers (diff).
func DetectDrift(existingChart, newChart *types.GeneratedChart) DriftReport {
	var report DriftReport

	if existingChart == nil && newChart == nil {
		return report
	}

	existingTemplates := make(map[string]string)
	newTemplates := make(map[string]string)
	if existingChart != nil {
		existingTemplates = existingChart.Templates
	}
	if newChart != nil {
		newTemplates = newChart.Templates
	}

	// Compare templates
	report.Templates = diffStringMaps(existingTemplates, newTemplates)

	// Compare values keys
	report.Values = diffValuesYAML(
		safeValuesYAML(existingChart),
		safeValuesYAML(newChart),
	)

	// Compare helpers
	existingHelpers := ""
	newHelpers := ""
	if existingChart != nil {
		existingHelpers = existingChart.Helpers
	}
	if newChart != nil {
		newHelpers = newChart.Helpers
	}
	if existingHelpers != newHelpers {
		if existingHelpers == "" {
			report.Helpers = append(report.Helpers, DriftItem{
				Category: DriftAdded,
				Path:     "_helpers.tpl",
				Detail:   "helpers added",
			})
		} else if newHelpers == "" {
			report.Helpers = append(report.Helpers, DriftItem{
				Category: DriftRemoved,
				Path:     "_helpers.tpl",
				Detail:   "helpers removed",
			})
		} else {
			report.Helpers = append(report.Helpers, DriftItem{
				Category: DriftChanged,
				Path:     "_helpers.tpl",
				Detail:   "helpers content changed",
			})
		}
	}

	return report
}

func safeValuesYAML(chart *types.GeneratedChart) string {
	if chart == nil {
		return ""
	}
	return chart.ValuesYAML
}

// diffStringMaps compares two maps of path->content and returns drift items.
func diffStringMaps(existing, new map[string]string) []DriftItem {
	var items []DriftItem

	allKeys := make(map[string]bool)
	for k := range existing {
		allKeys[k] = true
	}
	for k := range new {
		allKeys[k] = true
	}

	sorted := make([]string, 0, len(allKeys))
	for k := range allKeys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	for _, key := range sorted {
		oldContent, inOld := existing[key]
		newContent, inNew := new[key]

		switch {
		case !inOld && inNew:
			items = append(items, DriftItem{
				Category: DriftAdded,
				Path:     key,
				Detail:   "new template",
			})
		case inOld && !inNew:
			items = append(items, DriftItem{
				Category: DriftRemoved,
				Path:     key,
				Detail:   "template removed",
			})
		case oldContent != newContent:
			items = append(items, DriftItem{
				Category: DriftChanged,
				Path:     key,
				Detail:   "content changed",
			})
		}
	}

	return items
}

// diffValuesYAML compares two values.yaml strings by flattening them to dot-notation keys.
func diffValuesYAML(oldYAML, newYAML string) []DriftItem {
	oldKeys := flattenYAML(oldYAML)
	newKeys := flattenYAML(newYAML)

	var items []DriftItem

	allKeys := make(map[string]bool)
	for k := range oldKeys {
		allKeys[k] = true
	}
	for k := range newKeys {
		allKeys[k] = true
	}

	sorted := make([]string, 0, len(allKeys))
	for k := range allKeys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	for _, key := range sorted {
		oldVal, inOld := oldKeys[key]
		newVal, inNew := newKeys[key]

		switch {
		case !inOld && inNew:
			items = append(items, DriftItem{
				Category: DriftAdded,
				Path:     key,
				Detail:   fmt.Sprintf("new value: %v", newVal),
			})
		case inOld && !inNew:
			items = append(items, DriftItem{
				Category: DriftRemoved,
				Path:     key,
				Detail:   fmt.Sprintf("removed value: %v", oldVal),
			})
		case fmt.Sprintf("%v", oldVal) != fmt.Sprintf("%v", newVal):
			items = append(items, DriftItem{
				Category: DriftChanged,
				Path:     key,
				Detail:   fmt.Sprintf("changed: %v → %v", oldVal, newVal),
			})
		}
	}

	return items
}

// flattenYAML parses YAML and returns a flat map of dot-notation paths to values.
func flattenYAML(yamlStr string) map[string]interface{} {
	if yamlStr == "" {
		return make(map[string]interface{})
	}
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &data); err != nil {
		return make(map[string]interface{})
	}
	flat := make(map[string]interface{})
	flattenMap("", data, flat)
	return flat
}

func flattenMap(prefix string, data map[string]interface{}, result map[string]interface{}) {
	for k, v := range data {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]interface{}:
			flattenMap(key, val, result)
		default:
			result[key] = v
			_ = val
		}
	}
}

// GenerateMigrationPlan produces a markdown step-by-step migration plan from a DriftReport.
// Changes are ordered by risk: additions first, modifications second, removals last.
func GenerateMigrationPlan(drift DriftReport) string {
	if !drift.HasDrift() {
		return "# Migration Plan\n\nNo changes detected. Charts are identical.\n"
	}

	var b strings.Builder
	b.WriteString("# Migration Plan\n\n")
	b.WriteString(fmt.Sprintf("Total changes: %d\n\n", drift.TotalItems()))

	// Phase 1: Additions (safe)
	additions := collectByCategory(drift, DriftAdded)
	if len(additions) > 0 {
		b.WriteString("## Phase 1: Additions (low risk)\n\n")
		for i, item := range additions {
			b.WriteString(fmt.Sprintf("%d. Add `%s` — %s\n", i+1, item.Path, item.Detail))
		}
		b.WriteString("\n")
	}

	// Phase 2: Modifications (medium risk)
	changes := collectByCategory(drift, DriftChanged)
	if len(changes) > 0 {
		b.WriteString("## Phase 2: Modifications (medium risk)\n\n")
		for i, item := range changes {
			b.WriteString(fmt.Sprintf("%d. Update `%s` — %s\n", i+1, item.Path, item.Detail))
		}
		b.WriteString("\n")
	}

	// Phase 3: Removals (high risk)
	removals := collectByCategory(drift, DriftRemoved)
	if len(removals) > 0 {
		b.WriteString("## Phase 3: Removals (high risk — verify before applying)\n\n")
		for i, item := range removals {
			b.WriteString(fmt.Sprintf("%d. Remove `%s` — %s\n", i+1, item.Path, item.Detail))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// collectByCategory gathers all drift items of a given category across all sections.
func collectByCategory(drift DriftReport, category DriftCategory) []DriftItem {
	var result []DriftItem
	for _, items := range [][]DriftItem{drift.Templates, drift.Values, drift.Helpers} {
		for _, item := range items {
			if item.Category == category {
				result = append(result, item)
			}
		}
	}
	return result
}

// GenerateValuesMigration produces a _migrate.tpl helper that maps old values paths
// to new paths using coalesce, enabling backward compatibility during migration.
// It detects renamed keys by matching value types and positions in the flattened
// values hierarchy.
func GenerateValuesMigration(oldValues, newValues string) string {
	oldFlat := flattenYAML(oldValues)
	newFlat := flattenYAML(newValues)

	// Find keys only in old (candidates for rename source)
	removedKeys := make(map[string]interface{})
	for k, v := range oldFlat {
		if _, exists := newFlat[k]; !exists {
			removedKeys[k] = v
		}
	}

	// Find keys only in new (candidates for rename target)
	addedKeys := make(map[string]interface{})
	for k, v := range newFlat {
		if _, exists := oldFlat[k]; !exists {
			addedKeys[k] = v
		}
	}

	// Match renamed keys: same type and similar leaf name
	type keyMapping struct {
		oldKey string
		newKey string
	}
	var mappings []keyMapping

	// Track used keys to avoid duplicate matches
	usedOld := make(map[string]bool)
	usedNew := make(map[string]bool)

	// First pass: exact leaf name match with same type
	for newKey, newVal := range addedKeys {
		newLeaf := leafName(newKey)
		newType := fmt.Sprintf("%T", newVal)
		for oldKey, oldVal := range removedKeys {
			if usedOld[oldKey] || usedNew[newKey] {
				continue
			}
			oldLeaf := leafName(oldKey)
			oldType := fmt.Sprintf("%T", oldVal)
			if oldLeaf == newLeaf && oldType == newType {
				mappings = append(mappings, keyMapping{oldKey: oldKey, newKey: newKey})
				usedOld[oldKey] = true
				usedNew[newKey] = true
				break
			}
		}
	}

	// Second pass: same type, different leaf but same depth
	for newKey, newVal := range addedKeys {
		if usedNew[newKey] {
			continue
		}
		newDepth := strings.Count(newKey, ".")
		newType := fmt.Sprintf("%T", newVal)
		for oldKey, oldVal := range removedKeys {
			if usedOld[oldKey] {
				continue
			}
			oldDepth := strings.Count(oldKey, ".")
			oldType := fmt.Sprintf("%T", oldVal)
			if oldDepth == newDepth && oldType == newType {
				mappings = append(mappings, keyMapping{oldKey: oldKey, newKey: newKey})
				usedOld[oldKey] = true
				usedNew[newKey] = true
				break
			}
		}
	}

	sort.Slice(mappings, func(i, j int) bool {
		return mappings[i].newKey < mappings[j].newKey
	})

	if len(mappings) == 0 {
		return "{{/* No value migrations needed — no renamed keys detected. */}}\n"
	}

	var b strings.Builder
	b.WriteString("{{/*\n")
	b.WriteString("  _migrate.tpl — backward-compatible value mappings.\n")
	b.WriteString("  Generated by dhg migrate. Remove after migration is complete.\n")
	b.WriteString("*/}}\n\n")
	b.WriteString("{{- define \"migrate.values\" -}}\n")

	for _, m := range mappings {
		newPath := toValuesPath(m.newKey)
		oldPath := toValuesPath(m.oldKey)
		b.WriteString(fmt.Sprintf("{{- $_%s := coalesce %s %s -}}\n",
			sanitizeVarName(m.newKey), newPath, oldPath))
	}

	b.WriteString("{{- end -}}\n")

	return b.String()
}

// leafName returns the last segment of a dot-notation path.
func leafName(path string) string {
	parts := strings.Split(path, ".")
	return parts[len(parts)-1]
}

// toValuesPath converts "image.repository" to ".Values.image.repository".
func toValuesPath(dotPath string) string {
	return ".Values." + dotPath
}

// sanitizeVarName converts a dot-notation path to a valid Go template variable name.
func sanitizeVarName(path string) string {
	return strings.ReplaceAll(path, ".", "_")
}
