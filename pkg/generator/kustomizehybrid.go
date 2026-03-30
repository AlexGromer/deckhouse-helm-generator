package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// FluxSourceRef identifies the Flux source for a HelmRelease.
type FluxSourceRef struct {
	Kind      string
	Name      string
	Namespace string
}

// FluxSubstitutionSource references a ConfigMap or Secret for postBuild substituteFrom.
type FluxSubstitutionSource struct {
	Kind string
	Name string
}

// FluxPostBuildOptions configures Flux HelmRelease + Kustomization generation.
type FluxPostBuildOptions struct {
	ReleaseName       string
	Namespace         string
	Interval          string
	SourceRef         FluxSourceRef
	Substitutions     map[string]string
	SubstitutionsFrom []FluxSubstitutionSource
}

// FluxPostBuildResult holds the generated Flux resources.
type FluxPostBuildResult struct {
	// HelmRelease maps key → HelmRelease YAML.
	HelmRelease map[string]string
	// Kustomization maps key → Kustomization YAML.
	Kustomization map[string]string
	// NOTESTxt is a human-readable notes string.
	NOTESTxt string
}

// GenerateFluxPostBuild generates a Flux HelmRelease and Kustomization with
// postBuild variable substitution support. Returns nil if ReleaseName is empty.
func GenerateFluxPostBuild(chart *types.GeneratedChart, opts FluxPostBuildOptions) *FluxPostBuildResult {
	// Validate: ReleaseName is required.
	if opts.ReleaseName == "" {
		return &FluxPostBuildResult{
			HelmRelease:   map[string]string{},
			Kustomization: map[string]string{},
			NOTESTxt:      "Flux postBuild: ReleaseName is required.",
		}
	}

	interval := opts.Interval
	if interval == "" {
		interval = "5m"
	}
	ns := opts.Namespace
	if ns == "" {
		ns = "default"
	}

	sourceKind := opts.SourceRef.Kind
	if sourceKind == "" {
		sourceKind = "HelmRepository"
	}
	sourceName := opts.SourceRef.Name
	if sourceName == "" {
		sourceName = opts.ReleaseName
	}
	sourceNS := opts.SourceRef.Namespace
	if sourceNS == "" {
		sourceNS = ns
	}

	helmReleaseYAML := buildHelmReleaseYAML(opts.ReleaseName, ns, interval, sourceKind, sourceName, sourceNS)
	kustomizationYAML := buildKustomizationYAML(opts.ReleaseName, ns, interval, opts.Substitutions, opts.SubstitutionsFrom)

	return &FluxPostBuildResult{
		HelmRelease: map[string]string{
			opts.ReleaseName + "-helmrelease": helmReleaseYAML,
		},
		Kustomization: map[string]string{
			opts.ReleaseName + "-kustomization": kustomizationYAML,
		},
		NOTESTxt: buildFluxNotes(opts.ReleaseName, ns),
	}
}

// fluxSanitizeScalar strips newline and carriage-return characters from a string
// to prevent YAML structure injection when values are interpolated directly.
func fluxSanitizeScalar(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

// buildHelmReleaseYAML renders a Flux HelmRelease CRD as YAML.
func buildHelmReleaseYAML(releaseName, namespace, interval, sourceKind, sourceName, sourceNamespace string) string {
	var sb strings.Builder

	// Sanitize all user-supplied scalar values before YAML interpolation.
	releaseName = fluxSanitizeScalar(releaseName)
	namespace = fluxSanitizeScalar(namespace)
	interval = fluxSanitizeScalar(interval)
	sourceKind = fluxSanitizeScalar(sourceKind)
	sourceName = fluxSanitizeScalar(sourceName)
	sourceNamespace = fluxSanitizeScalar(sourceNamespace)

	sb.WriteString("apiVersion: helm.toolkit.fluxcd.io/v2\n")
	sb.WriteString("kind: HelmRelease\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", releaseName))
	sb.WriteString(fmt.Sprintf("  namespace: %s\n", namespace))
	sb.WriteString("spec:\n")
	sb.WriteString(fmt.Sprintf("  interval: %s\n", interval))
	sb.WriteString("  chart:\n")
	sb.WriteString("    spec:\n")
	sb.WriteString(fmt.Sprintf("      chart: %s\n", releaseName))
	sb.WriteString("      sourceRef:\n")
	sb.WriteString(fmt.Sprintf("        kind: %s\n", sourceKind))
	sb.WriteString(fmt.Sprintf("        name: %s\n", sourceName))
	sb.WriteString(fmt.Sprintf("        namespace: %s\n", sourceNamespace))

	return sb.String()
}

// buildKustomizationYAML renders a Flux Kustomization with postBuild as YAML.
func buildKustomizationYAML(releaseName, namespace, interval string, substitutions map[string]string, subsFrom []FluxSubstitutionSource) string {
	var sb strings.Builder

	// Sanitize scalar fields.
	releaseName = fluxSanitizeScalar(releaseName)
	namespace = fluxSanitizeScalar(namespace)
	interval = fluxSanitizeScalar(interval)

	sb.WriteString("apiVersion: kustomize.toolkit.fluxcd.io/v1\n")
	sb.WriteString("kind: Kustomization\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", releaseName))
	sb.WriteString(fmt.Sprintf("  namespace: %s\n", namespace))
	sb.WriteString("spec:\n")
	sb.WriteString(fmt.Sprintf("  interval: %s\n", interval))
	sb.WriteString(fmt.Sprintf("  path: ./charts/%s\n", releaseName))
	sb.WriteString("  prune: true\n")
	sb.WriteString("  postBuild:\n")

	// Inline substitutions — sorted for deterministic output (map iteration is non-deterministic).
	if len(substitutions) > 0 {
		keys := make([]string, 0, len(substitutions))
		for k := range substitutions {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		sb.WriteString("    substitute:\n")
		for _, k := range keys {
			v := fluxSanitizeScalar(substitutions[k])
			k = fluxSanitizeScalar(k)
			sb.WriteString(fmt.Sprintf("      %s: %q\n", k, v))
		}
	}

	// substituteFrom references.
	if len(subsFrom) > 0 {
		sb.WriteString("    substituteFrom:\n")
		for _, src := range subsFrom {
			sb.WriteString(fmt.Sprintf("    - kind: %s\n", fluxSanitizeScalar(src.Kind)))
			sb.WriteString(fmt.Sprintf("      name: %s\n", fluxSanitizeScalar(src.Name)))
		}
	}

	return sb.String()
}

// buildFluxNotes returns a human-readable notes string for Flux postBuild.
func buildFluxNotes(releaseName, namespace string) string {
	return fmt.Sprintf(
		"Flux CD postBuild configured.\n"+
			"HelmRelease '%s' in namespace '%s' will be managed by Flux.\n"+
			"Apply the generated HelmRelease and Kustomization manifests to your cluster.\n"+
			"Ensure the Flux controllers (helm-controller, kustomize-controller) are running.",
		releaseName, namespace,
	)
}

// InjectFluxPostBuild injects Flux HelmRelease and Kustomization YAML files into
// the chart's ExternalFiles. It is copy-on-write and idempotent.
// Returns the updated chart and the count of files injected.
func InjectFluxPostBuild(chart *types.GeneratedChart, result *FluxPostBuildResult) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}
	if result == nil || (len(result.HelmRelease) == 0 && len(result.Kustomization) == 0) {
		return chart, 0
	}

	// Build set of existing external file paths for idempotency.
	existing := make(map[string]struct{}, len(chart.ExternalFiles))
	for _, ef := range chart.ExternalFiles {
		existing[ef.Path] = struct{}{}
	}

	updated := copyChartTemplates(chart)
	// Copy ExternalFiles (copyChartTemplates does not handle this).
	updated.ExternalFiles = make([]types.ExternalFileInfo, len(chart.ExternalFiles))
	copy(updated.ExternalFiles, chart.ExternalFiles)

	count := 0

	injectFile := func(dir, key, content string) {
		path := fmt.Sprintf("flux/%s/%s.yaml", dir, key)
		if _, found := existing[path]; found {
			for i, ef := range updated.ExternalFiles {
				if ef.Path == path {
					updated.ExternalFiles[i].Content = content
					break
				}
			}
		} else {
			updated.ExternalFiles = append(updated.ExternalFiles, types.ExternalFileInfo{
				Path:    path,
				Content: content,
			})
		}
		count++
	}

	for key, yaml := range result.HelmRelease {
		injectFile("helmrelease", key, yaml)
	}
	for key, yaml := range result.Kustomization {
		injectFile("kustomization", key, yaml)
	}

	if result.NOTESTxt != "" {
		updated.Notes = result.NOTESTxt
	}

	return updated, count
}
