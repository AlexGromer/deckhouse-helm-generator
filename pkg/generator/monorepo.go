package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// MonorepoLayout represents a multi-chart monorepo project structure with
// shared tooling (Makefile, chart-testing config, .helmignore).
type MonorepoLayout struct {
	RootDir    string
	Charts     []*types.GeneratedChart
	Makefile   string
	HelmIgnore string
	CIWorkflow string
	CTConfig   string
}

// GenerateMonorepoLayout produces a MonorepoLayout for the given charts and
// project name. It returns an error when the charts slice is empty.
func GenerateMonorepoLayout(charts []*types.GeneratedChart, projectName string) (*MonorepoLayout, error) {
	if len(charts) == 0 {
		return nil, fmt.Errorf("monorepo layout requires at least one chart")
	}

	chartNames := make([]string, 0, len(charts))
	for _, c := range charts {
		chartNames = append(chartNames, c.Name)
	}

	return &MonorepoLayout{
		RootDir:    projectName,
		Charts:     charts,
		Makefile:   generateMonorepoMakefile(chartNames),
		HelmIgnore: generateMonorepoHelmIgnore(),
		CIWorkflow: generateMonorepoCIWorkflow(chartNames),
		CTConfig:   generateMonorepoCTConfig(),
	}, nil
}

// sanitizeChartName normalises a chart name for use in Makefile targets:
// lowercase, spaces replaced with hyphens.
func sanitizeChartName(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "-")
}

// generateMonorepoMakefile produces a Makefile that orchestrates lint, test,
// package, template, and dependency-update operations across all charts.
func generateMonorepoMakefile(chartNames []string) string {
	var b strings.Builder

	// Collect sanitized names.
	sanitized := make([]string, 0, len(chartNames))
	for _, n := range chartNames {
		sanitized = append(sanitized, sanitizeChartName(n))
	}

	b.WriteString("# Auto-generated monorepo Makefile\n")
	b.WriteString("CHARTS_DIR := charts\n\n")

	// Aggregate targets.
	targets := []struct {
		name string
		cmd  string
	}{
		{"lint-all", "helm lint"},
		{"test-all", "helm test"},
		{"package-all", "helm package"},
		{"template-all", "helm template"},
		{"deps-all", "helm dependency update"},
	}

	for _, tgt := range targets {
		b.WriteString(fmt.Sprintf(".PHONY: %s\n", tgt.name))
		b.WriteString(fmt.Sprintf("%s:", tgt.name))
		for _, s := range sanitized {
			b.WriteString(fmt.Sprintf(" %s-%s", strings.TrimSuffix(tgt.name, "-all"), s))
		}
		b.WriteString("\n\n")

		// Per-chart sub-targets.
		for _, s := range sanitized {
			subTarget := fmt.Sprintf("%s-%s", strings.TrimSuffix(tgt.name, "-all"), s)
			b.WriteString(fmt.Sprintf(".PHONY: %s\n", subTarget))
			b.WriteString(fmt.Sprintf("%s:\n", subTarget))
			b.WriteString(fmt.Sprintf("\t%s $(CHARTS_DIR)/%s\n\n", tgt.cmd, s))
		}
	}

	return b.String()
}

// generateMonorepoHelmIgnore returns common .helmignore patterns for a
// monorepo chart.
func generateMonorepoHelmIgnore() string {
	var b strings.Builder
	b.WriteString("# Common patterns to ignore when packaging Helm charts\n")
	b.WriteString(".git\n")
	b.WriteString(".gitignore\n")
	b.WriteString(".DS_Store\n")
	b.WriteString("*.swp\n")
	b.WriteString("*.bak\n")
	b.WriteString("*.tmp\n")
	b.WriteString("*.orig\n")
	b.WriteString(".idea/\n")
	b.WriteString(".vscode/\n")
	b.WriteString("*.md\n")
	b.WriteString("tests/\n")
	b.WriteString("ci/\n")
	return b.String()
}

// generateMonorepoCIWorkflow returns a simple CI workflow skeleton for the
// monorepo (stored in CIWorkflow field, informational only).
func generateMonorepoCIWorkflow(chartNames []string) string {
	var b strings.Builder
	b.WriteString("# CI workflow for monorepo Helm charts\n")
	b.WriteString("steps:\n")
	for _, n := range chartNames {
		s := sanitizeChartName(n)
		b.WriteString(fmt.Sprintf("  - name: lint-%s\n", s))
		b.WriteString(fmt.Sprintf("    run: helm lint charts/%s\n", s))
	}
	return b.String()
}

// generateMonorepoCTConfig produces a chart-testing (ct) configuration file
// in YAML format, pointing at the charts/ directory.
func generateMonorepoCTConfig() string {
	var b strings.Builder
	b.WriteString("# chart-testing configuration\n")
	b.WriteString("chart-dirs:\n")
	b.WriteString("  - charts/\n")
	b.WriteString("chart-repos: []\n")
	b.WriteString("helm-extra-args: --timeout 600s\n")
	return b.String()
}
