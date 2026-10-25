package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestWorkloadScaling(t *testing.T) {
	fc := FeatureContext{
		Params: map[string]string{
			"pvc-when-deleted":  "Retain",
			"pvc-when-scaled":   "Retain",
			"pdb":               "true",
			"pdb-min-available": "1",
			"keda":              "false",
			"keda-min-replicas": "1",
			"keda-max-replicas": "10",
			"keda-cpu-target":   "80",
		},
	}

	tests := []struct {
		name              string
		baseChart         *types.GeneratedChart
		expectValues      string
		expectTemplates   []string
		expectNoTemplates []string
		expectPDB         bool
		expectKEDA        bool
		expectInjected    bool
	}{
		{
			name: "Deployment triggers PDB and KEDA",
			baseChart: &types.GeneratedChart{
				ValuesYAML: "someKey: someValue\n",
				Templates: map[string]string{
					"templates/deployment.yaml": "kind: Deployment\nmetadata:\n  name: app\n",
				},
				Helpers: `{{- define "app.fullname" -}}app{{- end -}}`,
			},
			expectPDB:  true,
			expectKEDA: true,
		},
		{
			name: "StatefulSet with volume claims injects retention policy",
			baseChart: &types.GeneratedChart{
				ValuesYAML: "someKey: someValue\n",
				Templates: map[string]string{
					"templates/statefulset.yaml": `kind: StatefulSet
metadata:
  name: db
spec:
  volumeClaimTemplates:
  - metadata:
      name: data
`,
				},
				Helpers: `{{- define "db.fullname" -}}db{{- end -}}`,
			},
			expectPDB:      true,
			expectKEDA:     true,
			expectInjected: true,
		},
		{
			name: "StatefulSet with existing retention policy does not inject",
			baseChart: &types.GeneratedChart{
				ValuesYAML: "someKey: someValue\n",
				Templates: map[string]string{
					"templates/statefulset.yaml": `kind: StatefulSet
metadata:
  name: db
spec:
  persistentVolumeClaimRetentionPolicy:
    whenDeleted: Retain
  volumeClaimTemplates:
  - metadata:
      name: data
`,
				},
				Helpers: `{{- define "db.fullname" -}}db{{- end -}}`,
			},
			expectPDB:      true,
			expectKEDA:     true,
			expectInjected: false,
		},
		{
			name: "No workload no PDB or KEDA",
			baseChart: &types.GeneratedChart{
				ValuesYAML: "someKey: someValue\n",
				Templates: map[string]string{
					"templates/configmap.yaml": "kind: ConfigMap\nmetadata:\n  name: config\n",
				},
				Helpers: `{{- define "app.fullname" -}}app{{- end -}}`,
			},
			expectPDB:  false,
			expectKEDA: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := applyWorkloadScalingFeature(tt.baseChart, fc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !strings.Contains(out.ValuesYAML, "workloadScaling:") {
				t.Errorf("missing workloadScaling in values.yaml: %s", out.ValuesYAML)
			}
			if !strings.Contains(out.ValuesYAML, "whenDeleted: Retain") {
				t.Errorf("missing pvc retention in values.yaml: %s", out.ValuesYAML)
			}

			if tt.expectPDB {
				if _, ok := out.Templates["templates/workload-scaling-pdb.yaml"]; !ok {
					t.Error("expected workload-scaling-pdb.yaml but not found")
				}
			} else {
				if _, ok := out.Templates["templates/workload-scaling-pdb.yaml"]; ok {
					t.Error("did not expect workload-scaling-pdb.yaml")
				}
			}

			if tt.expectKEDA {
				if _, ok := out.Templates["templates/workload-scaling-keda.yaml"]; !ok {
					t.Error("expected workload-scaling-keda.yaml but not found")
				}
			} else {
				if _, ok := out.Templates["templates/workload-scaling-keda.yaml"]; ok {
					t.Error("did not expect workload-scaling-keda.yaml")
				}
			}

			if tt.name == "StatefulSet with volume claims injects retention policy" {
				content := out.Templates["templates/statefulset.yaml"]
				if !strings.Contains(content, "persistentVolumeClaimRetentionPolicy:") {
					t.Error("expected persistentVolumeClaimRetentionPolicy to be injected")
				}
				if !strings.Contains(content, "{{- with (.Values.workloadScaling).pvcRetentionPolicy }}") {
					t.Error("expected workloadScaling template block in StatefulSet")
				}
			}

			if tt.name == "StatefulSet with existing retention policy does not inject" {
				content := out.Templates["templates/statefulset.yaml"]
				if strings.Contains(content, "{{- with (.Values.workloadScaling).pvcRetentionPolicy }}") {
					t.Error("did not expect workloadScaling template block in StatefulSet as it already has a policy")
				}
			}
		})
	}
}
