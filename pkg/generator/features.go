package generator

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	yaml3 "gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
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
	// Notes is an optional sink for non-deducible observations.
	Notes *[]string
}

// Note adds a user-facing observation to the feature's notes sink,
// deduplicating exact matches and ignoring empty strings. It does nothing
// if the context has no sink.
func (fc FeatureContext) Note(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" || fc.Notes == nil {
		return
	}
	for _, existing := range *fc.Notes {
		if existing == msg {
			return
		}
	}
	*fc.Notes = append(*fc.Notes, msg)
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

// MapParam parses a semicolon-delimited key=value parameter into a map.
func (fc FeatureContext) MapParam(key string) (map[string]string, error) {
	val := fc.Param(key)
	if val == "" {
		return map[string]string{}, nil
	}

	out := map[string]string{}
	for _, pair := range strings.Split(val, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("parameter %s: %q is not key=value", key, pair)
		}
		k, v := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if _, exists := out[k]; exists {
			return nil, fmt.Errorf("parameter %s: duplicate key %q", key, k)
		}
		if errs := validation.IsQualifiedName(k); len(errs) > 0 {
			return nil, fmt.Errorf("parameter %s: key %q is invalid: %s", key, k, strings.Join(errs, ", "))
		}
		if errs := validation.IsValidLabelValue(v); len(errs) > 0 {
			return nil, fmt.Errorf("parameter %s: value %q is invalid: %s", key, v, strings.Join(errs, ", "))
		}
		out[k] = v
	}
	return out, nil
}

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
// It delegates to ApplyFeaturesWithNotes and discards the notes.
func ApplyFeatures(charts []*types.GeneratedChart, names []string, options map[string]map[string]string, graph *types.ResourceGraph) ([]*types.GeneratedChart, error) {
	out, _, err := ApplyFeaturesWithNotes(charts, names, options, graph)
	return out, err
}

// ApplyFeaturesWithNotes applies the named features, in the given order, to
// every chart that can carry templates, and returns the aggregated unique notes.
func ApplyFeaturesWithNotes(charts []*types.GeneratedChart, names []string, options map[string]map[string]string, graph *types.ResourceGraph) ([]*types.GeneratedChart, []string, error) {
	enabled := map[string]bool{}
	for _, name := range names {
		enabled[name] = true
	}
	for feature := range options {
		if !enabled[feature] {
			return nil, nil, fmt.Errorf("--feature-opt given for %q, which is not enabled with --with", feature)
		}
	}

	var allNotes []string
	result := append([]*types.GeneratedChart(nil), charts...)
	for _, name := range names {
		f, ok := LookupFeature(name)
		if !ok {
			return nil, nil, fmt.Errorf("unknown feature %q (see `dhg features`)", name)
		}
		params := make(map[string]string, len(f.Params))
		for k, v := range f.Params {
			params[k] = v
		}
		for k, v := range options[name] {
			if _, known := f.Params[k]; !known {
				return nil, nil, fmt.Errorf("feature %q has no parameter %q (see `dhg features`)", name, k)
			}
			params[k] = v
		}
		fc := FeatureContext{Graph: graph, Params: params, Notes: &allNotes}
		for i, chart := range result {
			if isLibraryChart(chart) {
				continue
			}
			out, err := f.Apply(chart, fc)
			if err != nil {
				return nil, nil, fmt.Errorf("feature %s on chart %s: %w", name, chart.Name, err)
			}
			result[i] = out
		}
	}
	return result, allNotes, nil
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

// setGlobalValue uses yaml.v3 to parse valuesYAML into a yaml.Node, find or create
// the top-level "global" map, and insert "key: value" into it without dropping comments.
// It returns an error if "global" already contains "key".
func setGlobalValue(valuesYAML, key string, value interface{}) (string, error) {
	var doc yaml3.Node
	if err := yaml3.Unmarshal([]byte(valuesYAML), &doc); err != nil {
		return "", fmt.Errorf("parsing values.yaml: %w", err)
	}

	if doc.Kind == 0 {
		// Empty document, create mapping
		doc.Kind = yaml3.DocumentNode
		doc.Content = []*yaml3.Node{{Kind: yaml3.MappingNode}}
	} else if len(doc.Content) == 0 {
		doc.Content = append(doc.Content, &yaml3.Node{Kind: yaml3.MappingNode})
	}

	root := doc.Content[0]
	if root.Kind != yaml3.MappingNode {
		return "", fmt.Errorf("expected values.yaml to be a mapping")
	}

	var globalNode *yaml3.Node
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "global" {
			globalNode = root.Content[i+1]
			break
		}
	}

	if globalNode == nil {
		// create "global" map at the end
		keyNode := &yaml3.Node{Kind: yaml3.ScalarNode, Value: "global"}
		globalNode = &yaml3.Node{Kind: yaml3.MappingNode}
		root.Content = append(root.Content, keyNode, globalNode)
	} else if globalNode.Kind != yaml3.MappingNode {
		return "", fmt.Errorf("values.yaml global is not a mapping")
	}

	// Check if key already exists
	for i := 0; i < len(globalNode.Content); i += 2 {
		if globalNode.Content[i].Value == key {
			return "", fmt.Errorf("values.yaml already has global.%s", key)
		}
	}

	// Convert value to node
	var valNode yaml3.Node
	valBytes, err := yaml3.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshaling value: %w", err)
	}
	if err := yaml3.Unmarshal(valBytes, &valNode); err != nil {
		return "", fmt.Errorf("unmarshaling value to node: %w", err)
	}
	if len(valNode.Content) == 0 {
		return "", fmt.Errorf("empty value node")
	}

	// append key and value to global
	keyNode := &yaml3.Node{Kind: yaml3.ScalarNode, Value: key}
	globalNode.Content = append(globalNode.Content, keyNode, valNode.Content[0])

	var buf bytes.Buffer
	enc := yaml3.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf("encoding values.yaml: %w", err)
	}
	_ = enc.Close()

	return buf.String(), nil
}
