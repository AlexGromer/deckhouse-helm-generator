package generator

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// Feature is an optional chart transformation enabled with
// `dhg generate --with <name>` and tuned with `--feature-opt <name>.<key>=<value>`.
//
// Features are registered from init() functions (see features_*.go). Every
// registered feature is exercised by the golden suite (tests/golden), so a
// feature must always produce charts that pass `helm lint` and `helm template`
// with its default parameters.
type Feature struct {
	// Name is the CLI identifier, e.g. "vault-agent".
	Name string
	// Description is a one-line summary shown by `dhg features`.
	Description string
	// Params maps every accepted parameter to its default value. Unknown
	// parameters are rejected so that typos do not pass silently.
	Params map[string]string
	// Apply transforms one generated chart. It must not mutate its input.
	Apply func(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error)
}

// FeatureContext carries everything a feature may need besides the chart.
type FeatureContext struct {
	// Graph is the analyzed resource graph the charts were generated from.
	Graph *types.ResourceGraph
	// Params holds the feature's parameters, defaults merged with user values.
	Params map[string]string
}

// Param returns a string parameter.
func (fc FeatureContext) Param(key string) string { return fc.Params[key] }

// BoolParam returns a boolean parameter ("true", "1", "yes" are true).
func (fc FeatureContext) BoolParam(key string) bool {
	switch strings.ToLower(fc.Params[key]) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

// IntParam returns an integer parameter, or 0 if it is not a valid integer.
func (fc FeatureContext) IntParam(key string) int {
	n, _ := strconv.Atoi(fc.Params[key])
	return n
}

// ListParam returns a comma-separated parameter as a slice (empty items dropped).
func (fc FeatureContext) ListParam(key string) []string {
	var out []string
	for _, item := range strings.Split(fc.Params[key], ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

var featureRegistry = map[string]Feature{}

// RegisterFeature adds a feature to the registry. It panics on duplicates or
// incomplete definitions; it is meant to be called from init().
func RegisterFeature(f Feature) {
	if f.Name == "" || f.Apply == nil || f.Description == "" {
		panic(fmt.Sprintf("generator: incomplete feature definition %q", f.Name))
	}
	if _, dup := featureRegistry[f.Name]; dup {
		panic(fmt.Sprintf("generator: feature %q registered twice", f.Name))
	}
	featureRegistry[f.Name] = f
}

// Features returns all registered features sorted by name.
func Features() []Feature {
	out := make([]Feature, 0, len(featureRegistry))
	for _, f := range featureRegistry {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LookupFeature returns the feature registered under name.
func LookupFeature(name string) (Feature, bool) {
	f, ok := featureRegistry[name]
	return f, ok
}

// ParseFeatureOptions parses `--feature-opt` values of the form
// "<feature>.<key>=<value>" into a per-feature parameter map.
func ParseFeatureOptions(raw []string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for _, item := range raw {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --feature-opt %q: expected <feature>.<key>=<value>", item)
		}
		feature, param, ok := strings.Cut(key, ".")
		if !ok || feature == "" || param == "" {
			return nil, fmt.Errorf("invalid --feature-opt %q: expected <feature>.<key>=<value>", item)
		}
		if out[feature] == nil {
			out[feature] = map[string]string{}
		}
		out[feature][param] = value
	}
	return out, nil
}

// ApplyFeatures applies the named features, in the given order, to every
// chart that can carry templates. Library charts (helpers only) are skipped.
func ApplyFeatures(charts []*types.GeneratedChart, names []string, options map[string]map[string]string, graph *types.ResourceGraph) ([]*types.GeneratedChart, error) {
	enabled := map[string]bool{}
	for _, name := range names {
		enabled[name] = true
	}
	for feature := range options {
		if !enabled[feature] {
			return nil, fmt.Errorf("--feature-opt given for %q, which is not enabled with --with", feature)
		}
	}

	result := append([]*types.GeneratedChart(nil), charts...)
	for _, name := range names {
		f, ok := LookupFeature(name)
		if !ok {
			return nil, fmt.Errorf("unknown feature %q (see `dhg features`)", name)
		}
		params := make(map[string]string, len(f.Params))
		for k, v := range f.Params {
			params[k] = v
		}
		for k, v := range options[name] {
			if _, known := f.Params[k]; !known {
				return nil, fmt.Errorf("feature %q has no parameter %q (see `dhg features`)", name, k)
			}
			params[k] = v
		}
		fc := FeatureContext{Graph: graph, Params: params}
		for i, chart := range result {
			if isLibraryChart(chart) {
				continue
			}
			out, err := f.Apply(chart, fc)
			if err != nil {
				return nil, fmt.Errorf("feature %s on chart %s: %w", name, chart.Name, err)
			}
			result[i] = out
		}
	}
	return result, nil
}

func isLibraryChart(chart *types.GeneratedChart) bool {
	return strings.Contains(chart.ChartYAML, "\ntype: library")
}

// cloneChart returns a copy of chart whose Templates map and ExternalFiles
// slice can be modified without affecting the original.
func cloneChart(chart *types.GeneratedChart) *types.GeneratedChart {
	out := *chart
	out.Templates = make(map[string]string, len(chart.Templates))
	for k, v := range chart.Templates {
		out.Templates[k] = v
	}
	out.ExternalFiles = append([]types.ExternalFileInfo(nil), chart.ExternalFiles...)
	return &out
}

// appendTopLevelValues appends a new top-level key to a values.yaml document,
// preserving the existing content and comments. It fails if the key already
// exists, so features never silently overwrite generated values.
func appendTopLevelValues(valuesYAML, key string, value interface{}) (string, error) {
	var existing map[string]interface{}
	if err := yaml.Unmarshal([]byte(valuesYAML), &existing); err != nil {
		return "", fmt.Errorf("parsing values.yaml: %w", err)
	}
	if _, ok := existing[key]; ok {
		return "", fmt.Errorf("values.yaml already has top-level key %q", key)
	}
	block, err := yaml.Marshal(map[string]interface{}{key: value})
	if err != nil {
		return "", err
	}
	if valuesYAML != "" && !strings.HasSuffix(valuesYAML, "\n") {
		valuesYAML += "\n"
	}
	return valuesYAML + "\n" + string(block), nil
}
