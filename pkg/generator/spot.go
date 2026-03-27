package generator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// SpotProvider identifies a cloud provider for spot/preemptible instance support.
type SpotProvider string

const (
	SpotAWS   SpotProvider = "aws"
	SpotGCP   SpotProvider = "gcp"
	SpotAzure SpotProvider = "azure"
)

// SpotConfig holds the configuration for spot/preemptible instance support.
type SpotConfig struct {
	Provider    SpotProvider
	GracePeriod int
	Enabled     bool
}

// GenerateSpotTolerations returns Kubernetes tolerations for spot/preemptible nodes
// based on the specified cloud provider. Each provider uses its own well-known taint key.
func GenerateSpotTolerations(provider SpotProvider) []map[string]interface{} {
	switch provider {
	case SpotAWS:
		return []map[string]interface{}{
			{
				"key":      "node.kubernetes.io/lifecycle",
				"value":    "spot",
				"effect":   "NoSchedule",
				"operator": "Equal",
			},
		}
	case SpotGCP:
		return []map[string]interface{}{
			{
				"key":      "cloud.google.com/gke-preemptible",
				"value":    "true",
				"effect":   "NoSchedule",
				"operator": "Equal",
			},
		}
	case SpotAzure:
		return []map[string]interface{}{
			{
				"key":      "kubernetes.azure.com/scalesetpriority",
				"value":    "spot",
				"effect":   "NoSchedule",
				"operator": "Equal",
			},
		}
	default:
		return []map[string]interface{}{}
	}
}

// GenerateSpotPreStopHook returns a lifecycle preStop hook configuration that sleeps
// for the specified grace period, allowing in-flight requests to drain before termination.
func GenerateSpotPreStopHook(gracePeriod int) map[string]interface{} {
	return map[string]interface{}{
		"lifecycle": map[string]interface{}{
			"preStop": map[string]interface{}{
				"exec": map[string]interface{}{
					"command": []string{"sh", "-c", fmt.Sprintf("sleep %d", gracePeriod)},
				},
			},
		},
	}
}

// GenerateSpotPDB returns a PodDisruptionBudget YAML string for the given application.
// For low replica counts (<=2), minAvailable is set to 1.
// For higher replica counts (>2), minAvailable is set to "50%".
func GenerateSpotPDB(appName string, replicas int) string {
	var minAvailable string
	if replicas <= 2 {
		minAvailable = "minAvailable: 1"
	} else {
		minAvailable = `minAvailable: "50%"`
	}

	return fmt.Sprintf(`apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: %s-pdb
spec:
  %s
  selector:
    matchLabels:
      app: %s
`, appName, minAvailable, appName)
}

// GenerateSpotValues returns a values map containing spot instance configuration
// suitable for inclusion in a Helm chart's values.yaml.
func GenerateSpotValues(config SpotConfig) map[string]interface{} {
	return map[string]interface{}{
		"spot": map[string]interface{}{
			"enabled":     config.Enabled,
			"provider":    string(config.Provider),
			"gracePeriod": config.GracePeriod,
		},
	}
}

// InjectSpotConfig injects spot/preemptible tolerations into Deployment and StatefulSet
// templates within the chart. Job and CronJob templates are left unmodified since spot
// termination handling is typically not appropriate for batch workloads.
// Returns nil if chart is nil. The original chart is not mutated.
func InjectSpotConfig(chart *types.GeneratedChart, config SpotConfig) *types.GeneratedChart {
	if chart == nil {
		return nil
	}

	tolerations := GenerateSpotTolerations(config.Provider)

	// Copy templates map — do not mutate the original.
	templates := make(map[string]string, len(chart.Templates))
	for k, v := range chart.Templates {
		templates[k] = v
	}

	for name, content := range templates {
		// Only inject into Deployments and StatefulSets.
		if strings.Contains(content, "kind: Deployment") || strings.Contains(content, "kind: StatefulSet") {
			// Skip Jobs and CronJobs that might also match.
			if strings.Contains(content, "kind: Job") || strings.Contains(content, "kind: CronJob") {
				continue
			}
			templates[name] = injectTolerationsIntoTemplate(content, tolerations)
		}
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

// injectTolerationsIntoTemplate inserts a tolerations section into the pod spec
// of a Kubernetes workload template YAML string. It finds the `containers:` key
// inside the pod spec and inserts tolerations just before it at the same indentation.
// If tolerations already exist in the template, the function is idempotent and returns
// the template unchanged. If `containers:` is not found, falls back to appending at the end.
func injectTolerationsIntoTemplate(template string, tolerations []map[string]interface{}) string {
	if len(tolerations) == 0 {
		return template
	}

	// Idempotency: skip if tolerations already present.
	if strings.Contains(template, "tolerations:") {
		return template
	}

	// Find `containers:` in the pod spec to determine insertion point and indentation.
	// The regex captures the newline, leading whitespace, and the containers key.
	// Note: matches the FIRST occurrence, which is correct for Deployment/StatefulSet/DaemonSet
	// (spec.template.spec.containers). `initContainers:` does not match this pattern.
	// For CronJob (nested jobTemplate.spec.template.spec.containers), only the first is patched.
	re := regexp.MustCompile(`(\n)([ \t]+)(containers:\s*\n)`)
	loc := re.FindStringIndex(template)
	match := re.FindStringSubmatch(template)

	if loc != nil && match != nil {
		indent := match[2] // indentation of `containers:`

		// Build tolerations block at the same indentation level.
		var lines []string
		lines = append(lines, indent+"tolerations:")
		for _, tol := range tolerations {
			key, _ := tol["key"].(string)
			value, _ := tol["value"].(string)
			effect, _ := tol["effect"].(string)
			operator, _ := tol["operator"].(string)
			lines = append(lines, indent+"- key: "+key)
			lines = append(lines, indent+"  operator: "+operator)
			lines = append(lines, indent+"  value: "+value)
			lines = append(lines, indent+"  effect: "+effect)
		}

		tolerationsBlock := strings.Join(lines, "\n")
		// Insert before `containers:` (after the preceding newline).
		insertPos := loc[0] + 1 // after the \n
		return template[:insertPos] + tolerationsBlock + "\n" + template[insertPos:]
	}

	// Fallback: containers: not found — append at end (may produce invalid YAML).
	var lines []string
	lines = append(lines, "      # FALLBACK: containers: not found in pod spec")
	lines = append(lines, "      tolerations:")
	for _, tol := range tolerations {
		key, _ := tol["key"].(string)
		value, _ := tol["value"].(string)
		effect, _ := tol["effect"].(string)
		operator, _ := tol["operator"].(string)
		lines = append(lines, fmt.Sprintf("      - key: %s", key))
		lines = append(lines, fmt.Sprintf("        operator: %s", operator))
		lines = append(lines, fmt.Sprintf("        value: %s", value))
		lines = append(lines, fmt.Sprintf("        effect: %s", effect))
	}

	tolerationsBlock := strings.Join(lines, "\n")
	return template + "\n" + tolerationsBlock
}
