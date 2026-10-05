package generator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
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
	Provider SpotProvider
	// GracePeriod becomes the pods' terminationGracePeriodSeconds, so in-flight
	// work can drain before a reclaimed node goes away.
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

// GenerateSpotValues returns the `spot` values section the injected templates
// read; `spot.enabled: false` switches everything off at install time.
func GenerateSpotValues(config SpotConfig) map[string]interface{} {
	tolerations := make([]interface{}, 0)
	for _, t := range GenerateSpotTolerations(config.Provider) {
		tolerations = append(tolerations, t)
	}
	return map[string]interface{}{
		"enabled":                       config.Enabled,
		"provider":                      string(config.Provider),
		"terminationGracePeriodSeconds": config.GracePeriod,
		"tolerations":                   tolerations,
	}
}

// spotPDBCondition guards the PodDisruptionBudgets added for spot.
const spotPDBCondition = "and $.Values.spot $.Values.spot.enabled"

// valuesTolerationsBlock matches the pod-spec tolerations block dhg's workload
// templates emit (tolerations taken from values).
var valuesTolerationsBlock = regexp.MustCompile(
	`(?m)^([ \t]*)\{\{- with \.tolerations \}\}\n[ \t]*tolerations:\n[ \t]*\{\{- toYaml \. \| nindent (\d+) \}\}\n[ \t]*\{\{- end \}\}\n`)

// InjectSpotConfig makes the chart's Deployments and StatefulSets schedulable
// on spot/preemptible nodes: spot tolerations are added to the tolerations from
// values, terminationGracePeriodSeconds is set, and each workload without a
// PodDisruptionBudget gets one, selecting the pods by the workload's own
// spec.selector. Everything is controlled by the `spot` values
// section added to values.yaml. Jobs and CronJobs are left unmodified since
// spot termination handling is typically not appropriate for batch workloads.
// The original chart is not mutated.
func InjectSpotConfig(chart *types.GeneratedChart, config SpotConfig) (*types.GeneratedChart, error) {
	if chart == nil {
		return nil, nil
	}

	out := cloneChart(chart)
	name := chartNameOf(chart)
	injected := false

	for path, content := range chart.Templates {
		if kind := extractKind(content); kind != "Deployment" && kind != "StatefulSet" {
			continue
		}
		patched, ok := injectSpotIntoPodSpec(content)
		if !ok {
			continue
		}
		out.Templates[path] = patched
		injected = true

		wl, ok := parseResourceTemplate(path, content)
		if !ok || hasPDBForWorkload(chart, wl) {
			continue
		}
		if pdb, ok := workloadPDBTemplate(name, wl, "spot", spotPDBCondition); ok {
			out.Templates[spotPDBTemplateKey(path)] = pdb
		}
	}

	if !injected {
		return out, nil
	}
	values, err := appendTopLevelValues(out.ValuesYAML, "spot", GenerateSpotValues(config))
	if err != nil {
		return nil, err
	}
	out.ValuesYAML = values
	return out, nil
}

// injectSpotIntoPodSpec rewrites the values-driven tolerations block of a
// workload template so that spot tolerations are appended when spot is
// enabled. It reports false when the template has no such block.
func injectSpotIntoPodSpec(template string) (string, bool) {
	loc := valuesTolerationsBlock.FindStringSubmatchIndex(template)
	if loc == nil {
		return template, false
	}
	indent := template[loc[2]:loc[3]]
	nindent := template[loc[4]:loc[5]]

	lines := []string{
		`{{- $dhgTolerations := .tolerations | default list }}`,
		`{{- if and $.Values.spot $.Values.spot.enabled }}`,
		`{{- $dhgTolerations = concat $dhgTolerations $.Values.spot.tolerations }}`,
		`terminationGracePeriodSeconds: {{ $.Values.spot.terminationGracePeriodSeconds }}`,
		`{{- end }}`,
		`{{- with $dhgTolerations }}`,
		`tolerations:`,
		`  {{- toYaml . | nindent ` + nindent + ` }}`,
		`{{- end }}`,
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(indent + l + "\n")
	}
	return template[:loc[0]] + sb.String() + template[loc[1]:], true
}

// spotPDBTemplateKey derives a PDB template map key from the source template key.
// For example, "templates/deployment.yaml" → "templates/deployment-spot-pdb.yaml".
func spotPDBTemplateKey(templateKey string) string {
	ext := ".yaml"
	base := strings.TrimSuffix(templateKey, ext)
	return base + "-spot-pdb" + ext
}

// extractReplicas parses the `replicas:` value from a Kubernetes workload template.
// Returns defaultVal if the field is absent or uses a Helm template expression.
func extractReplicas(content string, defaultVal int) int {
	re := regexp.MustCompile(`replicas:\s*(\d+)`)
	m := re.FindStringSubmatch(content)
	if m == nil {
		return defaultVal
	}
	var n int
	if _, err := fmt.Sscanf(m[1], "%d", &n); err != nil {
		return defaultVal
	}
	return n
}
