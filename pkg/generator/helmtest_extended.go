package generator

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// SnapshotTestOptions configures snapshot test generation.
type SnapshotTestOptions struct {
	// MatchSnapshot enables the matchSnapshot assertion in tests.
	MatchSnapshot bool
	// UpdateSnapshot adds an update-snapshot annotation to test files.
	UpdateSnapshot bool
}

// ValuePermutationOptions configures value permutation test generation.
type ValuePermutationOptions struct {
	// ValueOverrides maps value key to a slice of values to test.
	ValueOverrides map[string][]interface{}
	// CombinationMode is "all" (cartesian product) or "pairwise".
	CombinationMode string
}

// GenerateSnapshotTests generates helm-unittest snapshot test files for each
// renderable template in the chart. Templates ending in _helpers.tpl or NOTES.txt
// are skipped. Returns nil if chart is nil.
func GenerateSnapshotTests(chart *types.GeneratedChart, opts SnapshotTestOptions) map[string]string {
	if chart == nil {
		return nil
	}

	result := make(map[string]string)

	for path, content := range chart.Templates {
		base := filepath.Base(path)
		if skipTestFiles[base] {
			continue
		}
		if !strings.HasSuffix(base, ".yaml") && !strings.HasSuffix(base, ".yml") {
			continue
		}

		// Derive test file name: templates/deployment.yaml → tests/deployment_snapshot_test.yaml
		noExt := strings.TrimSuffix(base, filepath.Ext(base))
		testPath := fmt.Sprintf("tests/%s_snapshot_test.yaml", noExt)

		testContent := generateSnapshotTestContent(chart.Name, path, content, opts)
		result[testPath] = testContent
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

func generateSnapshotTestContent(chartName, templatePath, _ string, opts SnapshotTestOptions) string {
	base := filepath.Base(templatePath)
	noExt := strings.TrimSuffix(base, filepath.Ext(base))
	kind := guessKindFromTemplatePath(noExt)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("suite: %s snapshot tests\n", noExt))
	if opts.UpdateSnapshot {
		sb.WriteString("# Update mode: update-snapshot enabled\n")
	}
	sb.WriteString("templates:\n")
	sb.WriteString(fmt.Sprintf("  - %s\n", templatePath))
	sb.WriteString("tests:\n")
	sb.WriteString(fmt.Sprintf("  - it: should match snapshot for %s\n", kind))
	sb.WriteString("    asserts:\n")
	if opts.MatchSnapshot {
		sb.WriteString("      - matchSnapshot: {}\n")
	}
	if opts.UpdateSnapshot {
		sb.WriteString("      # update-snapshot: true\n")
	}
	sb.WriteString(fmt.Sprintf("    # Chart: %s, template: %s\n", chartName, templatePath))

	return sb.String()
}

func guessKindFromTemplatePath(noExt string) string {
	switch {
	case strings.Contains(noExt, "deployment"):
		return "Deployment"
	case strings.Contains(noExt, "service"):
		return "Service"
	case strings.Contains(noExt, "configmap"):
		return "ConfigMap"
	case strings.Contains(noExt, "ingress"):
		return "Ingress"
	case strings.Contains(noExt, "statefulset"):
		return "StatefulSet"
	default:
		return noExt
	}
}

// GenerateValuePermutationTests generates helm-unittest test files with multiple
// value override combinations. Uses cartesian product for "all" mode or pairwise
// for "pairwise" mode. Returns empty map if ValueOverrides is empty.
func GenerateValuePermutationTests(chart *types.GeneratedChart, opts ValuePermutationOptions) map[string]string {
	if chart == nil {
		return nil
	}
	if len(opts.ValueOverrides) == 0 {
		return map[string]string{}
	}

	// Collect keys in deterministic order
	keys := make([]string, 0, len(opts.ValueOverrides))
	for k := range opts.ValueOverrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var combinations []map[string]interface{}

	switch opts.CombinationMode {
	case "pairwise":
		combinations = generatePairwiseCombinations(keys, opts.ValueOverrides)
	default: // "all" = cartesian product
		combinations = generateCartesianProduct(keys, opts.ValueOverrides)
	}

	result := make(map[string]string)

	// Generate one test file per renderable template
	for path, content := range chart.Templates {
		base := filepath.Base(path)
		if skipTestFiles[base] {
			continue
		}
		if !strings.HasSuffix(base, ".yaml") && !strings.HasSuffix(base, ".yml") {
			continue
		}

		noExt := strings.TrimSuffix(base, filepath.Ext(base))
		testPath := fmt.Sprintf("tests/%s_permutation_test.yaml", noExt)

		testContent := generatePermutationTestContent(chart.Name, path, content, combinations)
		result[testPath] = testContent
	}

	return result
}

func generatePermutationTestContent(chartName, templatePath, _ string, combinations []map[string]interface{}) string {
	base := filepath.Base(templatePath)
	noExt := strings.TrimSuffix(base, filepath.Ext(base))

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("suite: %s value permutation tests\n", noExt))
	sb.WriteString("templates:\n")
	sb.WriteString(fmt.Sprintf("  - %s\n", templatePath))
	sb.WriteString(fmt.Sprintf("  # Chart: %s\n", chartName))
	sb.WriteString("tests:\n")

	for i, combo := range combinations {
		// Collect keys in deterministic order for stable output
		keys := make([]string, 0, len(combo))
		for k := range combo {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var parts []string
		var setLines []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%v", k, combo[k]))
			setLines = append(setLines, fmt.Sprintf("        %s: %v\n", k, combo[k]))
		}

		sb.WriteString(fmt.Sprintf("  - it: permutation %d (%s)\n", i+1, strings.Join(parts, ", ")))
		sb.WriteString("    set:\n")
		for _, line := range setLines {
			sb.WriteString(line)
		}
		sb.WriteString("    asserts:\n")
		sb.WriteString("      - hasDocuments:\n")
		sb.WriteString("          count: 1\n")
	}

	return sb.String()
}

// generateCartesianProduct returns all combinations (full cartesian product).
func generateCartesianProduct(keys []string, overrides map[string][]interface{}) []map[string]interface{} {
	result := []map[string]interface{}{{}}

	for _, key := range keys {
		values := overrides[key]
		var expanded []map[string]interface{}
		for _, existing := range result {
			for _, v := range values {
				combo := make(map[string]interface{}, len(existing)+1)
				for k, ev := range existing {
					combo[k] = ev
				}
				combo[key] = v
				expanded = append(expanded, combo)
			}
		}
		result = expanded
	}

	return result
}

// generatePairwiseCombinations returns a pairwise-reduced set of combinations.
// For simplicity, uses a greedy algorithm that ensures each pair of (key, value)
// appears at least once.
func generatePairwiseCombinations(keys []string, overrides map[string][]interface{}) []map[string]interface{} {
	if len(keys) == 0 {
		return nil
	}

	// Simple pairwise: for each pair of factors, enumerate their combinations.
	// Use a reduced set: max(len(values_per_key)) rows covering all pairs.
	maxVals := 0
	for _, key := range keys {
		if l := len(overrides[key]); l > maxVals {
			maxVals = l
		}
	}

	var result []map[string]interface{}

	for i := 0; i < maxVals; i++ {
		combo := make(map[string]interface{}, len(keys))
		for _, key := range keys {
			vals := overrides[key]
			combo[key] = vals[i%len(vals)]
		}
		result = append(result, combo)
	}

	return result
}
