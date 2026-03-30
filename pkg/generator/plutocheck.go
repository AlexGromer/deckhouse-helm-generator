package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// PlutoCheckOptions configures deprecated API detection.
type PlutoCheckOptions struct {
	// TargetVersion is the target Kubernetes version to check against (e.g., "1.22").
	TargetVersion string
	// ShowAll includes non-deprecated resources in the report.
	ShowAll bool
}

// APIDeprecation describes a single deprecated API usage found in a template.
type APIDeprecation struct {
	// TemplatePath is the path of the template containing the deprecated API.
	TemplatePath string
	// Kind is the resource kind (e.g., "Ingress").
	Kind string
	// APIVersion is the deprecated API version (e.g., "extensions/v1beta1").
	APIVersion string
	// ReplacementAPI is the recommended replacement API version.
	ReplacementAPI string
	// DeprecatedIn is the K8s version where this API was deprecated.
	DeprecatedIn string
	// RemovedIn is the K8s version where this API was removed.
	RemovedIn string
	// Message is a human-readable description of the deprecation.
	Message string
}

// PlutoCheckResult contains the result of deprecated API scanning.
type PlutoCheckResult struct {
	// Deprecations is the list of deprecated API usages found.
	Deprecations []APIDeprecation
	// Report is a human-readable summary of the results.
	Report string
	// HasDeprecations indicates whether any deprecated APIs were found.
	HasDeprecations bool
	// NOTESTxt contains additional notes for developers.
	NOTESTxt string
}

// CheckDeprecatedAPIs scans chart templates for deprecated Kubernetes API versions
// using the apiMigrations table from apimigration.go.
// Returns nil if chart is nil.
func CheckDeprecatedAPIs(chart *types.GeneratedChart, opts PlutoCheckOptions) *PlutoCheckResult {
	if chart == nil {
		return nil
	}

	var deprecations []APIDeprecation
	// Track all resources for ShowAll report
	type resourceEntry struct {
		path       string
		apiVersion string
		kind       string
		deprecated bool
	}
	var allResources []resourceEntry

	for path, content := range chart.Templates {
		apiVersion, kind := extractAPIVersionAndKind(content)
		if apiVersion == "" {
			continue
		}

		migration := GetMigrationInfo(apiVersion, kind)
		if migration != nil {
			msg := fmt.Sprintf("Use %s instead of %s for %s", migration.NewAPIVersion, migration.OldAPIVersion, migration.OldKind)
			if migration.NewAPIVersion == "" {
				msg = fmt.Sprintf("%s/%s has been removed; %s", migration.OldAPIVersion, migration.OldKind, migration.Notes)
			}
			dep := APIDeprecation{
				TemplatePath:   path,
				Kind:           kind,
				APIVersion:     apiVersion,
				ReplacementAPI: migration.NewAPIVersion,
				DeprecatedIn:   migration.DeprecatedIn,
				RemovedIn:      migration.RemovedIn,
				Message:        msg,
			}
			deprecations = append(deprecations, dep)
			allResources = append(allResources, resourceEntry{path: path, apiVersion: apiVersion, kind: kind, deprecated: true})
		} else {
			allResources = append(allResources, resourceEntry{path: path, apiVersion: apiVersion, kind: kind, deprecated: false})
		}
	}

	hasDeprecations := len(deprecations) > 0

	// Build report
	var reportSb strings.Builder
	if hasDeprecations {
		reportSb.WriteString(fmt.Sprintf("Found %d deprecated API(s):\n\n", len(deprecations)))
		for _, d := range deprecations {
			reportSb.WriteString(fmt.Sprintf("  [DEPRECATED] %s/%s in %s\n", d.APIVersion, d.Kind, d.TemplatePath))
			if d.ReplacementAPI != "" {
				reportSb.WriteString(fmt.Sprintf("    Replacement: %s\n", d.ReplacementAPI))
			}
			if d.DeprecatedIn != "" {
				reportSb.WriteString(fmt.Sprintf("    Deprecated in: v%s\n", d.DeprecatedIn))
			}
			if d.RemovedIn != "" {
				reportSb.WriteString(fmt.Sprintf("    Removed in: v%s\n", d.RemovedIn))
			}
			reportSb.WriteString(fmt.Sprintf("    Message: %s\n", d.Message))
			reportSb.WriteString("\n")
		}
	} else {
		reportSb.WriteString("No deprecated APIs found.\n")
	}

	if opts.ShowAll && len(allResources) > 0 {
		reportSb.WriteString("\nAll resources:\n")
		for _, r := range allResources {
			status := "OK"
			if r.deprecated {
				status = "DEPRECATED"
			}
			reportSb.WriteString(fmt.Sprintf("  [%s] %s/%s in %s\n", status, r.apiVersion, r.kind, r.path))
		}
	}

	targetVersionNote := ""
	if opts.TargetVersion != "" {
		targetVersionNote = fmt.Sprintf(" (target: K8s %s)", opts.TargetVersion)
	}
	notesTxt := fmt.Sprintf("Pluto-style deprecated API check%s\nDeprecations found: %d\nUse CheckDeprecatedAPIs() to scan Helm chart templates.\n",
		targetVersionNote, len(deprecations))

	return &PlutoCheckResult{
		Deprecations:    deprecations,
		Report:          reportSb.String(),
		HasDeprecations: hasDeprecations,
		NOTESTxt:        notesTxt,
	}
}

// extractAPIVersionAndKind parses a YAML template string for apiVersion and kind fields.
func extractAPIVersionAndKind(content string) (apiVersion, kind string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "apiVersion:") {
			apiVersion = strings.TrimSpace(strings.TrimPrefix(line, "apiVersion:"))
		}
		if strings.HasPrefix(line, "kind:") {
			kind = strings.TrimSpace(strings.TrimPrefix(line, "kind:"))
		}
		if apiVersion != "" && kind != "" {
			break
		}
	}
	return apiVersion, kind
}
