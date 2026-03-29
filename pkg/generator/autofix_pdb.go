package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// PDBSmallReplicaThreshold is the replica count at or below which minAvailable:1 is used.
const PDBSmallReplicaThreshold = 3

// PDBMaxUnavailablePercent is the maxUnavailable value used for larger replica counts.
const PDBMaxUnavailablePercent = "25%"

// AutoPDBResult tracks the result of GenerateAutoPDB.
type AutoPDBResult struct {
	Generated int
	Skipped   int
	Details   map[string]string // template key → PDB key
}

// pdbKeyForTemplate derives the PDB template key from the source template key.
// e.g. "templates/deployment.yaml" → "templates/deployment-pdb.yaml"
func pdbKeyForTemplate(templateKey string) string {
	const ext = ".yaml"
	base := strings.TrimSuffix(templateKey, ext)
	return base + "-pdb" + ext
}

// GenerateAutoPDB generates PodDisruptionBudget templates for workload templates
// that do not already have an associated PDB key. Uses copy-on-write.
func GenerateAutoPDB(chart *types.GeneratedChart) (*types.GeneratedChart, AutoPDBResult) {
	result := copyChartTemplates(chart)
	res := AutoPDBResult{
		Details: make(map[string]string),
	}

	for path, content := range chart.Templates {
		// Only process Deployments and StatefulSets.
		if !strings.Contains(content, "kind: Deployment") && !strings.Contains(content, "kind: StatefulSet") {
			continue
		}
		// Skip CronJob/Job that might also contain kind: Job.
		if strings.Contains(content, "kind: CronJob") || strings.Contains(content, "kind: Job") {
			continue
		}

		// Derive PDB key.
		pdbKey := pdbKeyForTemplate(path)

		// Idempotency: skip if PDB already exists.
		if _, exists := result.Templates[pdbKey]; exists {
			res.Skipped++
			continue
		}

		// Skip if replicas is a Helm expression (cannot parse).
		if strings.Contains(content, "replicas: {{") || strings.Contains(content, "replicas:{{") {
			res.Skipped++
			continue
		}

		// Parse replicas.
		replicas := extractReplicas(content, -1)
		if replicas < 0 {
			// Not found or parse error — skip.
			res.Skipped++
			continue
		}
		if replicas <= 1 {
			res.Skipped++
			continue
		}

		// Build PDB YAML.
		var pdbContent string
		if replicas <= PDBSmallReplicaThreshold {
			pdbContent = fmt.Sprintf(`apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "%s.fullname" . }}-pdb
spec:
  minAvailable: 1
  selector:
    matchLabels:
      {{- include "%s.selectorLabels" . | nindent 6 }}
`, chart.Name, chart.Name)
		} else {
			pdbContent = fmt.Sprintf(`apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "%s.fullname" . }}-pdb
spec:
  maxUnavailable: "%s"
  selector:
    matchLabels:
      {{- include "%s.selectorLabels" . | nindent 6 }}
`, chart.Name, PDBMaxUnavailablePercent, chart.Name)
		}

		result.Templates[pdbKey] = pdbContent
		res.Generated++
		res.Details[path] = pdbKey
	}

	return result, res
}
