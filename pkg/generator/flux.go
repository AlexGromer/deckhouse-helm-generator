package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func init() {
	RegisterFeature(Feature{
		Name:        "flux",
		Description: "Write flux/helmrelease.yaml: a Flux CD HelmRelease that deploys this chart from a Flux source",
		Params: map[string]string{
			"namespace":        "default",
			"interval":         "10m",
			"source-kind":      "HelmRepository",
			"source-name":      "",
			"source-namespace": "flux-system",
		},
		Apply: applyFluxHelmRelease,
	})
}

// applyFluxHelmRelease adds a Flux HelmRelease manifest for the chart. It is
// written outside templates/, so it is not part of the rendered release.
func applyFluxHelmRelease(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	name := chartNameOf(chart)
	sourceName := fc.Param("source-name")
	if sourceName == "" {
		sourceName = name
	}
	switch kind := fc.Param("source-kind"); kind {
	case "HelmRepository", "GitRepository", "OCIRepository", "Bucket":
	default:
		return nil, fmt.Errorf("unsupported source-kind %q (HelmRepository, GitRepository, OCIRepository, Bucket)", kind)
	}

	out := cloneChart(chart)
	out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{
		Path: "flux/helmrelease.yaml",
		Content: buildHelmReleaseYAML(name, fc.Param("namespace"), fc.Param("interval"),
			fc.Param("source-kind"), sourceName, fc.Param("source-namespace"), chartVersionOf(chart)),
	})
	return out, nil
}

// chartVersionOf returns the version declared in Chart.yaml.
func chartVersionOf(chart *types.GeneratedChart) string {
	for _, line := range strings.Split(chart.ChartYAML, "\n") {
		if strings.HasPrefix(line, "version:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "version:")), `"'`)
		}
	}
	return ""
}

// fluxScalar quotes a value for safe YAML interpolation.
func fluxScalar(s string) string {
	return fmt.Sprintf("%q", strings.NewReplacer("\r", "", "\n", "").Replace(s))
}

// buildHelmReleaseYAML renders a Flux helm.toolkit.fluxcd.io/v2 HelmRelease.
// For a GitRepository/Bucket source the chart is referenced by its path in
// the source; otherwise by name and version.
func buildHelmReleaseYAML(chartName, namespace, interval, sourceKind, sourceName, sourceNamespace, version string) string {
	chartRef := chartName
	if sourceKind == "GitRepository" || sourceKind == "Bucket" {
		chartRef = "./" + chartName
	}
	var sb strings.Builder
	sb.WriteString("apiVersion: helm.toolkit.fluxcd.io/v2\n")
	sb.WriteString("kind: HelmRelease\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: %s\n", fluxScalar(chartName))
	fmt.Fprintf(&sb, "  namespace: %s\n", fluxScalar(namespace))
	sb.WriteString("spec:\n")
	fmt.Fprintf(&sb, "  interval: %s\n", fluxScalar(interval))
	sb.WriteString("  chart:\n")
	sb.WriteString("    spec:\n")
	fmt.Fprintf(&sb, "      chart: %s\n", fluxScalar(chartRef))
	if version != "" && sourceKind == "HelmRepository" {
		fmt.Fprintf(&sb, "      version: %s\n", fluxScalar(version))
	}
	sb.WriteString("      sourceRef:\n")
	fmt.Fprintf(&sb, "        kind: %s\n", sourceKind)
	fmt.Fprintf(&sb, "        name: %s\n", fluxScalar(sourceName))
	fmt.Fprintf(&sb, "        namespace: %s\n", fluxScalar(sourceNamespace))
	sb.WriteString("  install:\n")
	sb.WriteString("    remediation:\n")
	sb.WriteString("      retries: 3\n")
	sb.WriteString("  upgrade:\n")
	sb.WriteString("    remediation:\n")
	sb.WriteString("      retries: 3\n")
	return sb.String()
}
