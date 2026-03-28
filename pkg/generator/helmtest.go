package generator

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// skipTestFiles contains template basenames that should not get test scaffolds.
var skipTestFiles = map[string]bool{
	"_helpers.tpl": true,
	"NOTES.txt":    true,
}

// kindPattern matches `kind: <Kind>` in YAML template content.
var kindPattern = regexp.MustCompile(`(?m)^kind:\s+(\w+)`)

// featureFlagPattern matches `{{- if .Values.<path> }}` at the start of a template.
var featureFlagPattern = regexp.MustCompile(`\{\{-?\s*if\s+\.Values\.(\S+)\s*\}\}`)

// GenerateHelmTests generates helm-unittest test files for each template in the chart.
// It returns a map of test file path (e.g. "tests/deployment_test.yaml") to content.
// Templates named _helpers.tpl and NOTES.txt are skipped.
func GenerateHelmTests(chart *types.GeneratedChart) map[string]string {
	if chart == nil || len(chart.Templates) == 0 {
		return nil
	}

	tests := make(map[string]string)

	for templatePath, content := range chart.Templates {
		baseName := filepath.Base(templatePath)

		// Skip helper and notes files
		if skipTestFiles[baseName] {
			continue
		}

		// Skip partials (files starting with _)
		if strings.HasPrefix(baseName, "_") {
			continue
		}

		testContent := generateTestForTemplate(templatePath, content)
		if testContent == "" {
			continue
		}

		// Build test file path: templates/foo.yaml -> tests/foo_test.yaml
		nameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
		testPath := fmt.Sprintf("tests/%s_test.yaml", nameWithoutExt)

		tests[testPath] = testContent
	}

	if len(tests) == 0 {
		return nil
	}

	return tests
}

// generateTestForTemplate creates helm-unittest YAML content for a single template.
func generateTestForTemplate(templatePath, content string) string {
	baseName := filepath.Base(templatePath)
	nameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))

	// Extract Kind from template content
	kind := extractKindFromTemplate(content)
	if kind == "" {
		kind = "Unknown"
	}

	var sb strings.Builder

	// Suite header
	sb.WriteString(fmt.Sprintf("suite: test %s\n", nameWithoutExt))
	sb.WriteString("templates:\n")
	sb.WriteString(fmt.Sprintf("  - %s\n", templatePath))
	sb.WriteString("tests:\n")

	// Basic render test
	sb.WriteString("  - it: should render\n")
	sb.WriteString("    asserts:\n")
	sb.WriteString("      - isKind:\n")
	sb.WriteString(fmt.Sprintf("          of: %s\n", kind))

	// Check for feature flag guard
	featureFlag := extractFeatureFlag(content)
	if featureFlag != "" {
		sb.WriteString("  - it: should not render when disabled\n")
		sb.WriteString("    set:\n")
		sb.WriteString(fmt.Sprintf("      %s: false\n", featureFlag))
		sb.WriteString("    asserts:\n")
		sb.WriteString("      - hasDocuments:\n")
		sb.WriteString("          count: 0\n")
	}

	return sb.String()
}

// extractKindFromTemplate extracts the Kubernetes resource Kind from template content.
// It looks for `kind: <Kind>` in the YAML.
func extractKindFromTemplate(content string) string {
	matches := kindPattern.FindStringSubmatch(content)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// extractFeatureFlag extracts the Values path from a feature-flag guard
// at the beginning of the template (e.g., `{{- if .Values.features.monitoring }}`).
// Returns the dotted path (e.g., "features.monitoring") or empty string.
func extractFeatureFlag(content string) string {
	// Only consider feature flags near the start of the template
	// (first 200 chars or first line with {{- if .Values.)
	prefix := content
	if len(prefix) > 500 {
		prefix = prefix[:500]
	}

	matches := featureFlagPattern.FindStringSubmatch(prefix)
	if len(matches) >= 2 {
		// Clean trailing }} or whitespace
		flag := strings.TrimSpace(matches[1])
		flag = strings.TrimSuffix(flag, "}}")
		flag = strings.TrimSpace(flag)
		return flag
	}
	return ""
}
