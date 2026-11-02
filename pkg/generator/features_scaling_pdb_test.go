package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func scalingDeployChart() *types.GeneratedChart {
	return &types.GeneratedChart{
		ValuesYAML: "someKey: someValue\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment\nmetadata:\n  name: app\n",
		},
		Helpers: `{{- define "app.fullname" -}}app{{- end -}}`,
	}
}

// A PDB with minAvailable: 1 on a single-replica workload blocks node drains,
// so the default is maxUnavailable: 1.
func TestWorkloadScaling_PDBDefaultsToMaxUnavailable(t *testing.T) {
	var notes []string
	fc := FeatureContext{
		Params: map[string]string{
			"pvc-when-deleted":    "Retain",
			"pvc-when-scaled":     "Retain",
			"pdb":                 "true",
			"pdb-max-unavailable": "1",
			"keda-min-replicas":   "1",
			"keda-max-replicas":   "10",
			"keda-cpu-target":     "80",
		},
		Notes: &notes,
	}
	out, err := applyWorkloadScalingFeature(scalingDeployChart(), fc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out.ValuesYAML, "minAvailable") {
		t.Errorf("minAvailable must not be set by default:\n%s", out.ValuesYAML)
	}
	if !strings.Contains(out.ValuesYAML, "maxUnavailable: 1") {
		t.Errorf("expected maxUnavailable: 1 in values:\n%s", out.ValuesYAML)
	}
	pdb := out.Templates["templates/workload-scaling-pdb.yaml"]
	if !strings.Contains(pdb, "maxUnavailable:") {
		t.Errorf("PDB template must render maxUnavailable by default:\n%s", pdb)
	}
	if len(notes) == 0 {
		t.Error("expected a note about the PDB default")
	}
}

// An explicit pdb-min-available is still honoured.
func TestWorkloadScaling_PDBExplicitMinAvailable(t *testing.T) {
	fc := FeatureContext{
		Params: map[string]string{
			"pvc-when-deleted":    "Retain",
			"pvc-when-scaled":     "Retain",
			"pdb":                 "true",
			"pdb-min-available":   "2",
			"pdb-max-unavailable": "1",
			"keda-min-replicas":   "1",
			"keda-max-replicas":   "10",
			"keda-cpu-target":     "80",
		},
	}
	out, err := applyWorkloadScalingFeature(scalingDeployChart(), fc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.ValuesYAML, "minAvailable: 2") {
		t.Errorf("expected minAvailable: 2 in values:\n%s", out.ValuesYAML)
	}
	pdb := out.Templates["templates/workload-scaling-pdb.yaml"]
	if !strings.Contains(pdb, `hasKey $pdb "minAvailable"`) || !strings.Contains(pdb, "minAvailable: {{") {
		t.Errorf("PDB template must render minAvailable when set:\n%s", pdb)
	}
}

func TestWorkloadScaling_PDBInvalidMinAvailable(t *testing.T) {
	fc := FeatureContext{Params: map[string]string{
		"pdb-min-available": "abc", "pdb-max-unavailable": "1",
		"keda-min-replicas": "1", "keda-max-replicas": "10",
	}}
	if _, err := applyWorkloadScalingFeature(scalingDeployChart(), fc); err == nil {
		t.Error("expected an error for a non-numeric pdb-min-available")
	}
}
