package generator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"sigs.k8s.io/yaml"
)

func init() {
	RegisterFeature(Feature{
		Name:        "multi-env",
		Description: "Generates multi-environment values and delivery manifests",
		Params: map[string]string{
			"environments":  "dev,staging,prod",
			"delivery":      "helmfile,kustomize",
			"strict-schema": "true",
		},
		Apply: applyMultiEnvFeature,
	})
}

func applyMultiEnvFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	out := cloneChart(chart)
	if out.ExternalFiles == nil {
		out.ExternalFiles = make([]types.ExternalFileInfo, 0)
	}

	var baseValues map[string]interface{}
	if out.ValuesYAML != "" {
		if err := yaml.Unmarshal([]byte(out.ValuesYAML), &baseValues); err != nil {
			return nil, fmt.Errorf("parsing ValuesYAML: %w", err)
		}
	} else {
		baseValues = make(map[string]interface{})
	}

	envs := fc.ListParam("environments")
	if len(envs) == 0 {
		return out, nil
	}

	stdEnvVals := GenerateEnvValues(baseValues)

	for _, env := range envs {
		var content []byte
		if val, ok := stdEnvVals["values-"+env+".yaml"]; ok {
			content = val
		} else {
			prof := stagingProfile()
			profData, _ := yaml.Marshal(prof)
			content = append([]byte(fmt.Sprintf("# %s environment overrides\n", env)), profData...)
		}
		out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{
			Path:    "values-" + env + ".yaml",
			Content: string(content),
		})
	}

	delivery := fc.Param("delivery")
	if strings.Contains(delivery, "helmfile") {
		var hf strings.Builder
		hf.WriteString("environments:\n")
		for _, env := range envs {
			fmt.Fprintf(&hf, "  %s:\n", env)
		}
		hf.WriteString("releases:\n")
		fmt.Fprintf(&hf, "  - name: %s\n", chart.Name)
		hf.WriteString("    chart: .\n")
		hf.WriteString("    values:\n")
		hf.WriteString("      - values.yaml\n")
		hf.WriteString("      - values-{{ .Environment.Name }}.yaml\n")

		out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{
			Path:    "helmfile.yaml",
			Content: hf.String(),
		})
	}

	if strings.Contains(delivery, "kustomize") {
		for _, env := range envs {
			var ks strings.Builder
			ks.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
			ks.WriteString("kind: Kustomization\n")
			ks.WriteString("helmCharts:\n")
			fmt.Fprintf(&ks, "- name: %s\n", chart.Name)
			fmt.Fprintf(&ks, "  releaseName: %s\n", chart.Name)
			fmt.Fprintf(&ks, "  valuesFile: ../../values-%s.yaml\n", env)

			out.ExternalFiles = append(out.ExternalFiles, types.ExternalFileInfo{
				Path:    fmt.Sprintf("overlays/%s/kustomization.yaml", env),
				Content: ks.String(),
			})
		}
	}

	if fc.BoolParam("strict-schema") && out.ValuesSchema != "" {
		var schemaMap map[string]interface{}
		if err := json.Unmarshal([]byte(out.ValuesSchema), &schemaMap); err == nil {
			if _, ok := schemaMap["additionalProperties"]; !ok {
				schemaMap["additionalProperties"] = false
				if updated, err := json.MarshalIndent(schemaMap, "", "  "); err == nil {
					out.ValuesSchema = string(updated)
				}
			}
		}
	}

	fc.Note("multi-env: generated environment values and delivery manifests for " + strings.Join(envs, ", "))

	return out, nil
}
