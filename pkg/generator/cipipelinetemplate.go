package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// CIPipelineOptions configures CI pipeline template generation.
type CIPipelineOptions struct {
	// Platform is the CI platform: "github" or "gitlab". Defaults to "github".
	Platform string
	// ChartName is the name of the Helm chart.
	ChartName string
	// IncludeLint enables the helm lint stage.
	IncludeLint bool
	// IncludeValidate enables the kubeconform validation stage.
	IncludeValidate bool
	// IncludeTest enables the helm-unittest stage.
	IncludeTest bool
	// IncludeSecurity enables the conftest security stage.
	IncludeSecurity bool
	// IncludeIntegration enables the ct install integration stage.
	IncludeIntegration bool
}

// CIPipelineResult contains generated CI pipeline workflow files.
type CIPipelineResult struct {
	// Workflows maps filename to content (e.g., ".github/workflows/chart-ci.yml").
	Workflows map[string]string
	// NOTESTxt contains instructions for developers.
	NOTESTxt string
}

// GenerateCIPipelineTemplate generates CI pipeline workflow files for the given chart.
// Returns nil if chart is nil.
func GenerateCIPipelineTemplate(chart *types.GeneratedChart, opts CIPipelineOptions) *CIPipelineResult {
	if chart == nil {
		return nil
	}

	platform := opts.Platform
	if platform == "" {
		platform = "github"
	}

	chartName := opts.ChartName
	if chartName == "" && chart != nil {
		chartName = chart.Name
	}

	workflows := make(map[string]string)

	switch platform {
	case "gitlab":
		workflows[".gitlab-ci.yml"] = generateGitLabCI(chartName, opts)
	default: // github
		workflows[".github/workflows/chart-ci.yml"] = generateGitHubWorkflow(chartName, opts)
	}

	notesTxt := fmt.Sprintf(
		"CI pipeline template for chart %q (platform: %s)\n"+
			"Generated workflow file(s):\n",
		chartName, platform)
	for k := range workflows {
		notesTxt += fmt.Sprintf("  - %s\n", k)
	}
	notesTxt += "\nCopy the workflow file(s) to your repository and commit.\n"

	return &CIPipelineResult{
		Workflows: workflows,
		NOTESTxt:  notesTxt,
	}
}

func generateGitHubWorkflow(chartName string, opts CIPipelineOptions) string {
	var sb strings.Builder
	sb.WriteString("# CI pipeline for chart: " + chartName + "\n")
	sb.WriteString("on:\n")
	sb.WriteString("  push:\n")
	sb.WriteString("    branches: [main, master]\n")
	sb.WriteString("  pull_request:\n")
	sb.WriteString("    branches: [main, master]\n")
	sb.WriteString("\n")
	sb.WriteString("jobs:\n")

	if opts.IncludeLint {
		sb.WriteString("  lint:\n")
		sb.WriteString("    name: Helm Lint\n")
		sb.WriteString("    runs-on: ubuntu-latest\n")
		sb.WriteString("    steps:\n")
		sb.WriteString("      - uses: actions/checkout@v4\n")
		sb.WriteString("      - name: helm lint " + chartName + "\n")
		sb.WriteString("        run: helm lint ./charts/" + chartName + "\n")
		sb.WriteString("\n")
	}

	if opts.IncludeValidate {
		sb.WriteString("  validate:\n")
		sb.WriteString("    name: Kubeconform Validate\n")
		sb.WriteString("    runs-on: ubuntu-latest\n")
		sb.WriteString("    steps:\n")
		sb.WriteString("      - uses: actions/checkout@v4\n")
		sb.WriteString("      - name: kubeconform validate\n")
		sb.WriteString("        run: kubeconform -kubernetes-version=1.29.0 charts/" + chartName + "/templates/\n")
		sb.WriteString("\n")
	}

	if opts.IncludeTest {
		sb.WriteString("  unittest:\n")
		sb.WriteString("    name: Helm Unit Tests\n")
		sb.WriteString("    runs-on: ubuntu-latest\n")
		sb.WriteString("    steps:\n")
		sb.WriteString("      - uses: actions/checkout@v4\n")
		sb.WriteString("      - name: helm unittest " + chartName + "\n")
		sb.WriteString("        run: helm unittest ./charts/" + chartName + "\n")
		sb.WriteString("\n")
	}

	if opts.IncludeSecurity {
		sb.WriteString("  security:\n")
		sb.WriteString("    name: Conftest Security\n")
		sb.WriteString("    runs-on: ubuntu-latest\n")
		sb.WriteString("    steps:\n")
		sb.WriteString("      - uses: actions/checkout@v4\n")
		sb.WriteString("      - name: conftest security check\n")
		sb.WriteString("        run: conftest test charts/" + chartName + "/templates/\n")
		sb.WriteString("\n")
	}

	if opts.IncludeIntegration {
		sb.WriteString("  integration:\n")
		sb.WriteString("    name: Chart Testing Integration\n")
		sb.WriteString("    runs-on: ubuntu-latest\n")
		sb.WriteString("    steps:\n")
		sb.WriteString("      - uses: actions/checkout@v4\n")
		sb.WriteString("      - name: ct install " + chartName + "\n")
		sb.WriteString("        run: ct install --charts charts/" + chartName + "\n")
		sb.WriteString("\n")
	}

	return sb.String()
}

func generateGitLabCI(chartName string, opts CIPipelineOptions) string {
	var sb strings.Builder
	sb.WriteString("# CI pipeline for chart: " + chartName + "\n")
	sb.WriteString("stages:\n")

	if opts.IncludeLint {
		sb.WriteString("  - lint\n")
	}
	if opts.IncludeValidate {
		sb.WriteString("  - validate\n")
	}
	if opts.IncludeTest {
		sb.WriteString("  - test\n")
	}
	if opts.IncludeSecurity {
		sb.WriteString("  - security\n")
	}
	if opts.IncludeIntegration {
		sb.WriteString("  - integration\n")
	}
	sb.WriteString("\n")

	if opts.IncludeLint {
		sb.WriteString("helm-lint:\n")
		sb.WriteString("  stage: lint\n")
		sb.WriteString("  script:\n")
		sb.WriteString("    - helm lint ./charts/" + chartName + "\n")
		sb.WriteString("\n")
	}

	if opts.IncludeValidate {
		sb.WriteString("kubeconform:\n")
		sb.WriteString("  stage: validate\n")
		sb.WriteString("  script:\n")
		sb.WriteString("    - kubeconform -kubernetes-version=1.29.0 charts/" + chartName + "/templates/\n")
		sb.WriteString("\n")
	}

	if opts.IncludeTest {
		sb.WriteString("helm-unittest:\n")
		sb.WriteString("  stage: test\n")
		sb.WriteString("  script:\n")
		sb.WriteString("    - helm unittest ./charts/" + chartName + "\n")
		sb.WriteString("\n")
	}

	if opts.IncludeSecurity {
		sb.WriteString("conftest:\n")
		sb.WriteString("  stage: security\n")
		sb.WriteString("  script:\n")
		sb.WriteString("    - conftest test charts/" + chartName + "/templates/\n")
		sb.WriteString("\n")
	}

	if opts.IncludeIntegration {
		sb.WriteString("ct-install:\n")
		sb.WriteString("  stage: integration\n")
		sb.WriteString("  script:\n")
		sb.WriteString("    - ct install --charts charts/" + chartName + "\n")
		sb.WriteString("\n")
	}

	return sb.String()
}

// InjectCIPipeline injects CI pipeline workflow files into the chart using
// copy-on-write semantics. Returns the new chart and the count of injected files.
func InjectCIPipeline(chart *types.GeneratedChart, result *CIPipelineResult) (*types.GeneratedChart, int) {
	if chart == nil || result == nil {
		return chart, 0
	}

	newChart := copyChartTemplates(chart)
	count := 0

	for filename, content := range result.Workflows {
		newChart.Templates[filename] = content
		count++
	}

	return newChart, count
}
