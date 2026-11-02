package generator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func init() {
	RegisterFeature(Feature{
		Name:        "workload-scaling",
		Description: "Scale workloads with KEDA, generate PDBs and manage PVC retention policies",
		Params: map[string]string{
			"pvc-when-deleted":    "Retain",
			"pvc-when-scaled":     "Retain",
			"pdb":                 "true",
			"pdb-min-available":   "",
			"pdb-max-unavailable": "1",
			"keda":                "false",
			"keda-min-replicas":   "1",
			"keda-max-replicas":   "10",
			"keda-cpu-target":     "80",
		},
		Apply: applyWorkloadScalingFeature,
	})
}

func applyWorkloadScalingFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	out := cloneChart(chart)

	// minAvailable is opt-in: minAvailable: 1 on a single-replica workload
	// leaves no pod to evict and blocks node drains, so the PDB defaults to
	// maxUnavailable.
	pdbMinAvailable := -1
	if raw := fc.Param("pdb-min-available"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid pdb-min-available %q: %w", raw, err)
		}
		pdbMinAvailable = n
	}
	pdbMaxUnavailable := 1
	if raw := fc.Param("pdb-max-unavailable"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid pdb-max-unavailable %q: %w", raw, err)
		}
		pdbMaxUnavailable = n
	}
	kedaMinReplicas, err := strconv.Atoi(fc.Param("keda-min-replicas"))
	if err != nil {
		return nil, fmt.Errorf("invalid keda-min-replicas %q: %w", fc.Param("keda-min-replicas"), err)
	}
	kedaMaxReplicas, err := strconv.Atoi(fc.Param("keda-max-replicas"))
	if err != nil {
		return nil, fmt.Errorf("invalid keda-max-replicas %q: %w", fc.Param("keda-max-replicas"), err)
	}

	pdbValues := map[string]interface{}{
		"enabled":        fc.BoolParam("pdb"),
		"maxUnavailable": pdbMaxUnavailable,
	}
	if pdbMinAvailable >= 0 {
		pdbValues["minAvailable"] = pdbMinAvailable
	}

	valuesBlock := map[string]interface{}{
		"enabled": true,
		"pvcRetentionPolicy": map[string]interface{}{
			"whenDeleted": fc.Param("pvc-when-deleted"),
			"whenScaled":  fc.Param("pvc-when-scaled"),
		},
		"pdb": pdbValues,
		"keda": map[string]interface{}{
			"enabled":         fc.BoolParam("keda"),
			"minReplicaCount": kedaMinReplicas,
			"maxReplicaCount": kedaMaxReplicas,
			"cpuUtilization":  fc.Param("keda-cpu-target"),
		},
	}

	newValues, err := appendTopLevelValues(out.ValuesYAML, "workloadScaling", valuesBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to append workloadScaling values: %w", err)
	}
	out.ValuesYAML = newValues

	hasWorkload := false

	for path, content := range out.Templates {
		kind := opsTemplateKind(content)
		if kind == "Deployment" || kind == "StatefulSet" {
			hasWorkload = true
		}

		if kind == "StatefulSet" {
			if strings.Contains(content, "volumeClaimTemplates:") && !strings.Contains(content, "persistentVolumeClaimRetentionPolicy:") {
				// Inject the block
				// We need to inject it at the same indentation level as volumeClaimTemplates.
				lines := strings.Split(content, "\n")
				var newLines []string
				for _, line := range lines {
					if strings.HasPrefix(strings.TrimSpace(line), "volumeClaimTemplates:") {
						indent := opsIndentWidth(line)
						prefix := strings.Repeat(" ", indent)
						newLines = append(newLines,
							prefix+`{{- with (.Values.workloadScaling).pvcRetentionPolicy }}`,
							prefix+`persistentVolumeClaimRetentionPolicy:`,
							prefix+`  whenDeleted: {{ .whenDeleted | default "Retain" }}`,
							prefix+`  whenScaled: {{ .whenScaled | default "Retain" }}`,
							prefix+`{{- end }}`,
						)
					}
					newLines = append(newLines, line)
				}
				out.Templates[path] = strings.Join(newLines, "\n")
			}
		}
	}

	if hasWorkload {
		helperPrefix := opsHelperPrefix(out)

		pdbContent := fmt.Sprintf(`{{- $ws := .Values.workloadScaling | default dict }}
{{- $pdb := $ws.pdb | default dict }}
{{- if and $ws.enabled $pdb.enabled }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "%s.fullname" . }}
  labels:
    {{- include "%s.labels" . | nindent 4 }}
spec:
  {{- if hasKey $pdb "minAvailable" }}
  minAvailable: {{ $pdb.minAvailable }}
  {{- else }}
  maxUnavailable: {{ $pdb.maxUnavailable | default 1 }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "%s.selectorLabels" . | nindent 6 }}
{{- end }}
`, helperPrefix, helperPrefix, helperPrefix)
		out.Templates["templates/workload-scaling-pdb.yaml"] = pdbContent
		if fc.BoolParam("pdb") && pdbMinAvailable < 0 {
			fc.Note("workload-scaling: the PodDisruptionBudget uses maxUnavailable (default 1) so single-replica workloads can still be drained; set workloadScaling.pdb.minAvailable to require availability (it must stay below the replica count)")
		}

		kedaContent := fmt.Sprintf(`{{- if and (.Values.workloadScaling).enabled ((.Values.workloadScaling).keda).enabled }}
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: {{ include "%s.fullname" . }}
  labels:
    {{- include "%s.labels" . | nindent 4 }}
spec:
  scaleTargetRef:
    name: {{ include "%s.fullname" . }}
  minReplicaCount: {{ ((.Values.workloadScaling).keda).minReplicaCount | default 1 }}
  maxReplicaCount: {{ ((.Values.workloadScaling).keda).maxReplicaCount | default 10 }}
  triggers:
  - type: cpu
    metadata:
      value: {{ ((.Values.workloadScaling).keda).cpuUtilization | default "80" | quote }}
{{- end }}
`, helperPrefix, helperPrefix, helperPrefix)
		out.Templates["templates/workload-scaling-keda.yaml"] = kedaContent
	}

	return out, nil
}
