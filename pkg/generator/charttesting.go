package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ChartTestingOptions configures ct (chart-testing) configuration generation.
type ChartTestingOptions struct {
	// RemoteRepo is the remote chart repository URL.
	RemoteRepo string
	// ChartDirs is the list of directories containing charts.
	ChartDirs []string
	// ExcludedCharts is the list of charts to exclude from testing.
	ExcludedCharts []string
	// HelmExtraArgs is additional arguments passed to helm during testing.
	HelmExtraArgs string
	// Upgrade enables upgrade testing (ct install --upgrade).
	Upgrade bool
}

// ChartTestingResult contains generated chart-testing configuration.
type ChartTestingResult struct {
	// ConfigFiles maps filename to content (e.g., "ct.yaml").
	ConfigFiles map[string]string
	// CISteps is a list of CI commands for chart-testing.
	CISteps []string
	// NOTESTxt contains usage instructions.
	NOTESTxt string
}

// GenerateChartTestingConfig generates a ct.yaml configuration and CI steps
// for chart-testing. Returns nil if chart is nil.
func GenerateChartTestingConfig(chart *types.GeneratedChart, opts ChartTestingOptions) *ChartTestingResult {
	if chart == nil {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# ct (chart-testing) configuration\n")

	if len(opts.ChartDirs) > 0 {
		sb.WriteString("chart-dirs:\n")
		for _, d := range opts.ChartDirs {
			sb.WriteString(fmt.Sprintf("  - %s\n", d))
		}
	}

	if len(opts.ExcludedCharts) > 0 {
		sb.WriteString("excluded-charts:\n")
		for _, c := range opts.ExcludedCharts {
			sb.WriteString(fmt.Sprintf("  - %s\n", c))
		}
	}

	if opts.HelmExtraArgs != "" {
		sb.WriteString(fmt.Sprintf("helm-extra-args: %q\n", opts.HelmExtraArgs))
	}

	if opts.Upgrade {
		sb.WriteString("upgrade: true\n")
	}

	if opts.RemoteRepo != "" {
		sb.WriteString(fmt.Sprintf("remote: %s\n", opts.RemoteRepo))
	}

	ctYaml := sb.String()

	// Build CI steps
	lintCmd := "ct lint"
	installCmd := "ct install"
	if len(opts.ChartDirs) > 0 {
		lintCmd += " --chart-dirs " + strings.Join(opts.ChartDirs, ",")
		installCmd += " --chart-dirs " + strings.Join(opts.ChartDirs, ",")
	}
	if opts.Upgrade {
		installCmd += " --upgrade"
	}

	ciSteps := []string{
		"ct lint",
		"ct install",
	}
	_ = lintCmd
	_ = installCmd

	notesTxt := fmt.Sprintf(
		"Chart-testing (ct) configuration for chart %q\n"+
			"Configuration file: ct.yaml\n\n"+
			"Run locally:\n"+
			"  ct lint --chart-dirs %s\n"+
			"  ct install --chart-dirs %s\n\n"+
			"Install chart-testing:\n"+
			"  https://github.com/helm/chart-testing\n",
		chart.Name,
		strings.Join(opts.ChartDirs, ","),
		strings.Join(opts.ChartDirs, ","),
	)

	return &ChartTestingResult{
		ConfigFiles: map[string]string{
			"ct.yaml": ctYaml,
		},
		CISteps:  ciSteps,
		NOTESTxt: notesTxt,
	}
}

// InjectChartTestingConfig injects chart-testing config files into the chart using
// copy-on-write semantics. Files are added as ExternalFiles.
// Returns the new chart and the count of injected files.
func InjectChartTestingConfig(chart *types.GeneratedChart, result *ChartTestingResult) (*types.GeneratedChart, int) {
	if chart == nil || result == nil {
		return chart, 0
	}

	newChart := copyChartTemplates(chart)

	// Copy ExternalFiles (copyChartTemplates copies the slice reference, not a new slice)
	newExtFiles := make([]types.ExternalFileInfo, len(chart.ExternalFiles))
	copy(newExtFiles, chart.ExternalFiles)

	count := 0
	for filename, content := range result.ConfigFiles {
		newExtFiles = append(newExtFiles, types.ExternalFileInfo{
			Path:    filename,
			Content: content,
		})
		count++
	}

	newChart.ExternalFiles = newExtFiles
	return newChart, count
}
