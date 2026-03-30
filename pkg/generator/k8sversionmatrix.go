package generator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// K8sVersionOptions configures Kubernetes version matrix validation.
type K8sVersionOptions struct {
	// MinVersion is the minimum Kubernetes version to check (e.g., "1.27").
	MinVersion string
	// MaxVersion is the maximum Kubernetes version to check (e.g., "1.32").
	MaxVersion string
	// TargetVersions is an explicit list of K8s versions to validate against.
	// If both MinVersion/MaxVersion and TargetVersions are set, both are used.
	TargetVersions []string
}

// VersionCompatibility holds compatibility info for a single K8s version.
type VersionCompatibility struct {
	// Version is the Kubernetes version (e.g., "1.29").
	Version string
	// Compatible indicates whether the chart is fully compatible with this version.
	Compatible bool
	// Issues is a list of compatibility problems for this version.
	Issues []string
}

// K8sVersionResult contains the result of version matrix validation.
type K8sVersionResult struct {
	// Compatibility maps K8s version string to its VersionCompatibility.
	Compatibility map[string]VersionCompatibility
	// Warnings contains global warnings not tied to a specific version.
	Warnings []string
	// NOTESTxt contains summary notes for developers.
	NOTESTxt string
}

// apiVersionGAMatrix maps apiVersion+kind to the minimum K8s minor version where it became GA/stable.
// Format: key = "apiVersion/Kind", value = minimum minor version (e.g., 23 for 1.23).
var apiVersionGAMatrix = map[string]int{
	// autoscaling
	"autoscaling/v2/HorizontalPodAutoscaler": 23,
	// networking
	"networking.k8s.io/v1/Ingress": 19,
	// policy
	"policy/v1/PodDisruptionBudget": 21,
	// batch
	"batch/v1/CronJob": 21,
	// apps
	"apps/v1/Deployment":  9,
	"apps/v1/StatefulSet": 9,
	"apps/v1/DaemonSet":   9,
	"apps/v1/ReplicaSet":  9,
	// core
	"v1/Pod":                   1,
	"v1/Service":               1,
	"v1/ConfigMap":             1,
	"v1/Secret":                1,
	"v1/ServiceAccount":        1,
	"v1/PersistentVolumeClaim": 1,
	// rbac
	"rbac.authorization.k8s.io/v1/ClusterRole":        8,
	"rbac.authorization.k8s.io/v1/ClusterRoleBinding": 8,
	"rbac.authorization.k8s.io/v1/Role":               8,
	"rbac.authorization.k8s.io/v1/RoleBinding":        8,
}

// apiVersionRemovedMatrix maps apiVersion+kind to the K8s minor version where it was REMOVED.
var apiVersionRemovedMatrix = map[string]int{
	"extensions/v1beta1/Ingress":                       22,
	"networking.k8s.io/v1beta1/Ingress":                22,
	"extensions/v1beta1/NetworkPolicy":                 16,
	"policy/v1beta1/PodDisruptionBudget":               25,
	"policy/v1beta1/PodSecurityPolicy":                 25,
	"rbac.authorization.k8s.io/v1beta1/ClusterRole":    22,
	"rbac.authorization.k8s.io/v1beta1/ClusterRoleBinding": 22,
	"rbac.authorization.k8s.io/v1beta1/Role":           22,
	"rbac.authorization.k8s.io/v1beta1/RoleBinding":    22,
	"autoscaling/v2beta1/HorizontalPodAutoscaler":      26,
	"autoscaling/v2beta2/HorizontalPodAutoscaler":      26,
	"batch/v1beta1/CronJob":                            25,
}

// parseMinorVersion parses a version string like "1.29" or "1.29.0" into minor int (29).
func parseMinorVersion(v string) (int, error) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid version: %s", v)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid minor version in %s: %w", v, err)
	}
	return minor, nil
}

// expandVersionRange generates all minor versions between min and max inclusive.
func expandVersionRange(minVer, maxVer string) []string {
	minMinor, err1 := parseMinorVersion(minVer)
	maxMinor, err2 := parseMinorVersion(maxVer)
	if err1 != nil || err2 != nil {
		return nil
	}
	var versions []string
	for i := minMinor; i <= maxMinor; i++ {
		versions = append(versions, fmt.Sprintf("1.%d", i))
	}
	return versions
}

// collectVersions builds the deduplicated list of versions to check.
func collectVersions(opts K8sVersionOptions) []string {
	seen := make(map[string]bool)
	var versions []string

	addVersion := func(v string) {
		// Normalize to "1.XX" format
		v = strings.TrimPrefix(v, "v")
		parts := strings.Split(v, ".")
		if len(parts) >= 2 {
			key := fmt.Sprintf("%s.%s", parts[0], parts[1])
			if !seen[key] {
				seen[key] = true
				versions = append(versions, key)
			}
		}
	}

	if opts.MinVersion != "" && opts.MaxVersion != "" {
		for _, v := range expandVersionRange(opts.MinVersion, opts.MaxVersion) {
			addVersion(v)
		}
	}

	for _, v := range opts.TargetVersions {
		addVersion(v)
	}

	return versions
}

// ValidateK8sVersionMatrix validates chart templates against multiple Kubernetes versions.
// Returns nil if chart is nil.
func ValidateK8sVersionMatrix(chart *types.GeneratedChart, opts K8sVersionOptions) *K8sVersionResult {
	if chart == nil {
		return nil
	}

	versions := collectVersions(opts)
	if len(versions) == 0 {
		// No versions to check — return empty result
		return &K8sVersionResult{
			Compatibility: map[string]VersionCompatibility{},
			NOTESTxt:      "No target versions specified.",
		}
	}

	// Collect all apiVersion+kind pairs from templates
	type resourceRef struct {
		path       string
		apiVersion string
		kind       string
	}
	var resources []resourceRef
	for path, content := range chart.Templates {
		av, kind := extractAPIVersionAndKind(content)
		if av == "" {
			continue
		}
		resources = append(resources, resourceRef{path: path, apiVersion: av, kind: kind})
	}

	compatibility := make(map[string]VersionCompatibility, len(versions))
	var globalWarnings []string

	for _, ver := range versions {
		minor, err := parseMinorVersion(ver)
		if err != nil {
			continue
		}

		var issues []string

		for _, res := range resources {
			key := fmt.Sprintf("%s/%s", res.apiVersion, res.kind)

			// Check if removed in this version
			if removedIn, ok := apiVersionRemovedMatrix[key]; ok {
				if minor >= removedIn {
					issues = append(issues, fmt.Sprintf(
						"%s/%s was removed in K8s 1.%d (template: %s)",
						res.apiVersion, res.kind, removedIn, res.path))
					// Also add to global warnings
					globalWarnings = appendUnique(globalWarnings, fmt.Sprintf(
						"%s/%s removed in K8s 1.%d", res.apiVersion, res.kind, removedIn))
				}
			}

			// Check GA availability — only for known APIs
			if gaVersion, ok := apiVersionGAMatrix[key]; ok {
				if minor < gaVersion {
					issues = append(issues, fmt.Sprintf(
						"%s/%s is not available in K8s 1.%d (available since 1.%d, template: %s)",
						res.apiVersion, res.kind, minor, gaVersion, res.path))
				}
			} else {
				// Unknown API — check if it's a deprecated one that's also not yet available as new
				// For unrecognized APIs that are not in removal list, we skip (unknown = assume available)
				_ = key
			}
		}

		compatibility[ver] = VersionCompatibility{
			Version:    ver,
			Compatible: len(issues) == 0,
			Issues:     issues,
		}
	}

	// Build NOTESTxt summary
	versionList := strings.Join(versions, ", ")
	incompatible := 0
	for _, c := range compatibility {
		if !c.Compatible {
			incompatible++
		}
	}

	notesTxt := fmt.Sprintf(
		"K8s version matrix validation for chart %q\nVersions checked: %s\nIncompatible versions: %d/%d\n",
		chart.Name, versionList, incompatible, len(versions))

	return &K8sVersionResult{
		Compatibility: compatibility,
		Warnings:      globalWarnings,
		NOTESTxt:      notesTxt,
	}
}

// appendUnique appends s to slice only if not already present.
func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}
