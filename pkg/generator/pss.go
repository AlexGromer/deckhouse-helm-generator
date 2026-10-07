package generator

import (
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// pssWorkloadKinds lists Kubernetes workload kinds that contain pod templates.
var pssWorkloadKinds = []string{"Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob"}

// restrictedFields are the security fields required for PSS restricted level.
var restrictedFields = []string{
	"runAsNonRoot",
	"readOnlyRootFilesystem",
	"allowPrivilegeEscalation",
	"drop:",
	"seccompProfile",
}

// baselineFields are the security fields required for PSS baseline level.
var baselineFields = []string{
	"runAsNonRoot",
}

// InjectPSSDefaults adds Pod Security Standards fields to workload templates.
// Uses copy-on-write: returns a new chart, original is not modified.
func InjectPSSDefaults(chart *types.GeneratedChart, level string) *types.GeneratedChart {
	// Copy templates map — do not mutate the original.
	templates := make(map[string]string, len(chart.Templates))
	for k, v := range chart.Templates {
		templates[k] = v
	}

	for path, content := range templates {
		if !isWorkloadTemplate(content) {
			continue
		}
		templates[path] = injectSecurityContext(content, level)
	}

	return &types.GeneratedChart{
		Name:          chart.Name,
		Path:          chart.Path,
		ChartYAML:     chart.ChartYAML,
		ValuesYAML:    chart.ValuesYAML,
		Templates:     templates,
		Helpers:       chart.Helpers,
		Notes:         chart.Notes,
		ValuesSchema:  chart.ValuesSchema,
		ExternalFiles: chart.ExternalFiles,
	}
}

// isWorkloadTemplate checks if the template content represents a workload kind.
func isWorkloadTemplate(content string) bool {
	for _, kind := range pssWorkloadKinds {
		if strings.Contains(content, "kind: "+kind) {
			return true
		}
	}
	return false
}

// injectSecurityContext adds missing security fields into a workload template.
func injectSecurityContext(content, level string) string {
	var block string
	switch level {
	case "restricted":
		block = restrictedSecurityBlock
	case "baseline":
		block = baselineSecurityBlock
	default:
		block = restrictedSecurityBlock
	}

	// If all fields from the target block are already present, skip injection.
	fields := restrictedFields
	if level == "baseline" {
		fields = baselineFields
	}
	allPresent := true
	for _, f := range fields {
		if !strings.Contains(content, f) {
			allPresent = false
			break
		}
	}
	if allPresent {
		return content
	}

	// Find the container spec insertion point: after "image:" line.
	lines := strings.Split(content, "\n")
	var result []string
	injected := false

	for i, line := range lines {
		result = append(result, line)
		if !injected && strings.Contains(line, "image:") {
			// Determine indentation from the image line.
			indent := leadingSpaces(line)
			// Inject securityContext at the same indent level as image:.
			blockLines := strings.Split(block, "\n")
			for _, bl := range blockLines {
				if bl == "" {
					continue
				}
				result = append(result, indent+bl)
			}
			injected = true
			_ = i
		}
	}

	return strings.Join(result, "\n")
}

const restrictedSecurityBlock = `securityContext:
  runAsNonRoot: true
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop:
    - ALL
  seccompProfile:
    type: RuntimeDefault`

const baselineSecurityBlock = `securityContext:
  runAsNonRoot: true`

// leadingSpaces returns the whitespace prefix of a string.
func leadingSpaces(s string) string {
	trimmed := strings.TrimLeft(s, " \t")
	return s[:len(s)-len(trimmed)]
}
