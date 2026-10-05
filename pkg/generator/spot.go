package generator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
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

// GenerateSpotPDBHelm returns the PodDisruptionBudget template for one
// workload of the chart, rendered only while spot is enabled.
func GenerateSpotPDBHelm(chartName, component string) string {
	return workloadPDBTemplate(chartName, component, "spot", "and .Values.spot .Values.spot.enabled")
}

// workloadPDBTemplate returns a PodDisruptionBudget template for one workload
// of the chart, selected by its app.kubernetes.io/component label (a
// chart-wide selector would put several PDBs on the same pods, which blocks
// eviction). maxUnavailable: 1 keeps node drains possible at any replica
// count. condition, when not empty, guards the template.
func workloadPDBTemplate(chartName, component, nameSuffix, condition string) string {
	body := fmt.Sprintf(`apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "%[1]s.fullname" . }}-%[2]s-%[4]s
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "%[1]s.labels" . | nindent 4 }}
spec:
  maxUnavailable: 1
  selector:
    matchLabels:
      {{- include "%[1]s.selectorLabels" . | nindent 6 }}
      app.kubernetes.io/component: %[3]s
`, chartName, processor.ResourceNameSuffix(component), component, nameSuffix)
	if condition == "" {
		return body
	}
	return "{{- if " + condition + " }}\n" + body + "{{- end }}\n"
}

// valuesTolerationsBlock matches the pod-spec tolerations block dhg's workload
// templates emit (tolerations taken from values).
var valuesTolerationsBlock = regexp.MustCompile(
	`(?m)^([ \t]*)\{\{- with \.tolerations \}\}\n[ \t]*tolerations:\n[ \t]*\{\{- toYaml \. \| nindent (\d+) \}\}\n[ \t]*\{\{- end \}\}\n`)

var componentLabelRegex = regexp.MustCompile(`app\.kubernetes\.io/component: ([A-Za-z0-9._-]+)`)

// InjectSpotConfig makes the chart's Deployments and StatefulSets schedulable
// on spot/preemptible nodes: spot tolerations are added to the tolerations from
// values, terminationGracePeriodSeconds is set, and each workload without a
// PodDisruptionBudget gets one. Everything is controlled by the `spot` values
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

		m := componentLabelRegex.FindStringSubmatch(content)
		if m == nil || hasPDBForComponent(chart.Templates, m[1]) {
			continue
		}
		out.Templates[spotPDBTemplateKey(path)] = GenerateSpotPDBHelm(name, m[1])
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

// hasPDBForComponent reports whether the chart already has a
// PodDisruptionBudget for the given component.
func hasPDBForComponent(templates map[string]string, component string) bool {
	for _, content := range templates {
		if extractKind(content) == "PodDisruptionBudget" &&
			strings.Contains(content, "app.kubernetes.io/component: "+component+"\n") {
			return true
		}
	}
	return false
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
