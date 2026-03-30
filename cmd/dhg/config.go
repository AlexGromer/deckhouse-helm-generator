package main

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// DHGConfig holds the configuration for the deckhouse-helm-generator tool.
// It is populated from a .dhg.yaml file and may be overridden by CLI flags.
type DHGConfig struct {
	// OutputDir is the directory where Helm charts will be written.
	OutputDir string `yaml:"outputDir" json:"outputDir"`

	// ChartName is the name of the generated Helm chart.
	ChartName string `yaml:"chartName" json:"chartName"`

	// Mode is the chart generation mode (universal, separate, library, umbrella).
	Mode string `yaml:"mode" json:"mode"`

	// Namespace is the default Kubernetes namespace.
	Namespace string `yaml:"namespace" json:"namespace"`

	// IncludeTests controls whether Helm test templates are generated.
	IncludeTests bool `yaml:"includeTests" json:"includeTests"`

	// IncludeSchema controls whether values.schema.json is generated.
	IncludeSchema bool `yaml:"includeSchema" json:"includeSchema"`

	// SecretStrategy controls how Secrets are handled (env, vault, sealed, etc.).
	SecretStrategy string `yaml:"secretStrategy" json:"secretStrategy"`

	// TemplateDir is an optional custom templates directory.
	TemplateDir string `yaml:"templateDir" json:"templateDir"`

	// Plugins lists paths to external processor plugin binaries.
	Plugins []string `yaml:"plugins" json:"plugins"`
}

// LoadConfig reads a .dhg.yaml file at path and unmarshals it into a DHGConfig.
// It returns an error if the file cannot be read or the YAML is malformed.
// An empty file returns a zero-value DHGConfig without error.
func LoadConfig(path string) (*DHGConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %q: %w", path, err)
	}

	cfg := &DHGConfig{}
	if len(data) == 0 {
		return cfg, nil
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %q: %w", path, err)
	}

	return cfg, nil
}

// MergeConfigWithFlags returns a new DHGConfig where non-zero flag values
// override the corresponding config fields. The original cfg is not mutated.
// If flags is nil, a shallow copy of cfg is returned unchanged.
func MergeConfigWithFlags(cfg *DHGConfig, flags map[string]interface{}) *DHGConfig {
	if cfg == nil {
		cfg = &DHGConfig{}
	}

	// Shallow copy so we do not mutate the original.
	out := *cfg

	if flags == nil {
		return &out
	}

	if v, ok := flags["outputDir"]; ok {
		if s, ok := v.(string); ok && s != "" {
			out.OutputDir = s
		}
	}
	if v, ok := flags["chartName"]; ok {
		if s, ok := v.(string); ok && s != "" {
			out.ChartName = s
		}
	}
	if v, ok := flags["mode"]; ok {
		if s, ok := v.(string); ok && s != "" {
			out.Mode = s
		}
	}
	if v, ok := flags["namespace"]; ok {
		if s, ok := v.(string); ok && s != "" {
			out.Namespace = s
		}
	}
	if v, ok := flags["includeTests"]; ok {
		if b, ok := v.(bool); ok {
			out.IncludeTests = b
		}
	}
	if v, ok := flags["includeSchema"]; ok {
		if b, ok := v.(bool); ok {
			out.IncludeSchema = b
		}
	}
	if v, ok := flags["secretStrategy"]; ok {
		if s, ok := v.(string); ok && s != "" {
			out.SecretStrategy = s
		}
	}
	if v, ok := flags["templateDir"]; ok {
		if s, ok := v.(string); ok && s != "" {
			out.TemplateDir = s
		}
	}
	if v, ok := flags["plugins"]; ok {
		if ss, ok := v.([]string); ok && len(ss) > 0 {
			out.Plugins = ss
		}
	}

	return &out
}
