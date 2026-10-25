package generator

import (
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/compliance/pss"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/compliance/sarif"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func init() {
	RegisterFeature(Feature{
		Name:        "ci-workflow",
		Description: "generates CI/CD workflow files for GitHub or GitLab",
		Params: map[string]string{
			"providers":      "github,gitlab",
			"sarif":          "true",
			"benchmark-gate": "true",
		},
		Apply: applyCIWorkflowFeature,
	})
}

func applyCIWorkflowFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	out := cloneChart(chart)
	if out.ExternalFiles == nil {
		out.ExternalFiles = []types.ExternalFileInfo{}
	}

	providers := fc.Param("providers")
	hasGitHub := strings.Contains(providers, "github")
	hasGitLab := strings.Contains(providers, "gitlab")

	if hasGitHub {
		workflow := `name: Helm CI
on:
  push:
    branches: [ "main", "master" ]
  pull_request:

jobs:
  helm-checks:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Set up Helm
        uses: azure/setup-helm@v4
      - name: Helm Lint
        run: helm lint --strict .
      - name: Helm Template
        run: helm template test . > /dev/null
      - name: Helm Unittest
        run: |
          helm plugin install https://github.com/helm-unittest/helm-unittest.git
          helm unittest .
      - name: Install dhg
        run: |
          echo "Assuming dhg is installed or downloading it"
      - name: Run DHG Audit
        run: dhg audit --format sarif -o docs/audit.sarif.json
        continue-on-error: true
      - name: Upload SARIF file
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: docs/audit.sarif.json
`
		out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{
			Path:    ".github/workflows/helm-ci.yaml",
			Content: workflow,
		})
	}

	if hasGitLab {
		gitlabCI := `stages:
  - lint
  - test
  - security-sarif
  - benchmark

helm-lint:
  stage: lint
  image: alpine/helm:latest
  script:
    - helm lint --strict .

helm-test:
  stage: test
  image: alpine/helm:latest
  script:
    - helm plugin install https://github.com/helm-unittest/helm-unittest.git
    - helm unittest .

helm-security:
  stage: security-sarif
  image: alpine:latest
  script:
    - echo "Running dhg audit"
    - dhg audit --format sarif -o docs/audit.sarif.json
  artifacts:
    reports:
      sast: docs/audit.sarif.json
`
		out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{
			Path:    ".gitlab-ci.yml",
			Content: gitlabCI,
		})
	}

	if fc.Graph != nil {
		var resources []*types.ProcessedResource
		for _, r := range fc.Graph.Resources {
			resources = append(resources, r)
		}
		report := pss.EvaluateResources(resources, pss.ProfileRestricted)

		// Convert to array of WorkloadResult expected by sarif package
		var results []sarif.WorkloadResult
		for k, w := range report.Workloads {
			results = append(results, sarif.WorkloadResult{
				Kind:     k.GVK.Kind,
				Name:     k.Name,
				Profile:  w.Profile,
				Findings: w.Findings,
			})
		}

		sarifFindings := sarif.FromPSSResults(results)

		sReport := sarif.Report{
			ToolName:    "dhg",
			ToolVersion: "latest",
			Findings:    sarifFindings,
		}

		if fc.Param("sarif") == "true" {
			sarifBytes, err := sarif.FormatSARIF(sReport)
			if err == nil {
				out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{
					Path:    "docs/audit.sarif.json",
					Content: string(sarifBytes),
				})
			}
		}

		htmlBytes, err := sarif.FormatHTML(sReport)
		if err == nil {
			out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{
				Path:    "docs/audit-report.html",
				Content: string(htmlBytes),
			})
		}
	}

	return out, nil
}
