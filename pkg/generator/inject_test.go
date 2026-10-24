package generator

import (
	"reflect"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestAddWorkloadInject(t *testing.T) {
	tests := []struct {
		name         string
		specs        []WorkloadInjectSpec
		expectedYAML string
	}{
		{
			name: "single spec",
			specs: []WorkloadInjectSpec{
				{
					ContainerName: "app",
					Env: []InjectedEnvVar{
						{Name: "VAR1", Value: "val1"},
					},
					Volumes: []InjectedVolume{
						{Name: "vol1"},
					},
				},
			},
			expectedYAML: `feat1:
  inject:
    Deployment/a:
      containers:
        app:
          env:
          - name: VAR1
            value: val1
      volumes:
      - emptyDir: {}
        name: vol1
`,
		},
		{
			name: "merge and dedup",
			specs: []WorkloadInjectSpec{
				{
					ContainerName: "app",
					Env: []InjectedEnvVar{
						{Name: "VAR1", Value: "val1"},
					},
					Volumes: []InjectedVolume{
						{Name: "vol1"},
					},
				},
				{
					ContainerName: "app",
					Env: []InjectedEnvVar{
						{Name: "VAR1", Value: "ignored_duplicate"},
						{Name: "VAR2", Value: "val2"},
					},
					Volumes: []InjectedVolume{
						{Name: "vol2"},
					},
				},
			},
			expectedYAML: `feat1:
  inject:
    Deployment/a:
      containers:
        app:
          env:
          - name: VAR1
            value: val1
          - name: VAR2
            value: val2
      volumes:
      - emptyDir: {}
        name: vol1
      - emptyDir: {}
        name: vol2
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := make(map[string]interface{})
			for _, spec := range tt.specs {
				AddWorkloadInject(values, "feat1", "Deployment/a", spec)
			}
			out, err := yaml.Marshal(values)
			if err != nil {
				t.Fatalf("failed to marshal values: %v", err)
			}
			if string(out) != tt.expectedYAML {
				t.Errorf("yaml mismatch:\nexpected:\n%s\ngot:\n%s", tt.expectedYAML, string(out))
			}
		})
	}
}

func TestInjectedFeatures(t *testing.T) {
	values := map[string]interface{}{
		"feat2": map[string]interface{}{
			"inject": map[string]interface{}{
				"Deployment/a": map[string]interface{}{},
			},
		},
		"feat1": map[string]interface{}{
			"inject": map[string]interface{}{
				"Deployment/b": map[string]interface{}{},
			},
		},
		"ignored1": map[string]interface{}{},
		"ignored2": map[string]interface{}{
			"inject": map[string]interface{}{},
		},
	}
	features := InjectedFeatures(values)
	if !reflect.DeepEqual(features, []string{"feat1", "feat2"}) {
		t.Errorf("unexpected features: %v", features)
	}
}
