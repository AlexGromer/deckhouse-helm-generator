package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// PluginPriority puts plugins ahead of every built-in processor, so a plugin
// can replace the handling of a built-in kind.
const PluginPriority = 1000

// PluginProcessor delegates resources of the given GVKs to an external
// executable using a JSON protocol:
//
// stdin:  {"name", "namespace", "kind", "apiVersion", "chartName", "object"}
// stdout: {"serviceName", "templatePath", "templateContent", "valuesPath", "values"}
//
// An empty templateContent means "not handled": the next processor (or the
// generic fallback) processes the resource.
type PluginProcessor struct {
	BaseProcessor
	path    string
	timeout time.Duration
}

// NewPluginProcessor creates a processor running the executable at path for
// the given GVKs.
func NewPluginProcessor(path string, timeout time.Duration, gvks ...schema.GroupVersionKind) *PluginProcessor {
	return &PluginProcessor{
		BaseProcessor: NewBaseProcessor("plugin:"+path, PluginPriority, gvks...),
		path:          path,
		timeout:       timeout,
	}
}

// ParsePluginSpec parses "<apiVersion>/<Kind>[,<apiVersion>/<Kind>...]=<path>",
// e.g. "example.com/v1/Widget=./bin/widget" or "v1/ConfigMap=./cm-plugin".
func ParsePluginSpec(spec string) (string, []schema.GroupVersionKind, error) {
	kinds, path, ok := strings.Cut(spec, "=")
	if !ok || path == "" || kinds == "" {
		return "", nil, fmt.Errorf("invalid plugin %q: expected <apiVersion>/<Kind>=<path>", spec)
	}
	var gvks []schema.GroupVersionKind
	for _, k := range strings.Split(kinds, ",") {
		i := strings.LastIndex(k, "/")
		if i <= 0 || i == len(k)-1 {
			return "", nil, fmt.Errorf("invalid plugin kind %q: expected <apiVersion>/<Kind>, e.g. example.com/v1/Widget", k)
		}
		gv, err := schema.ParseGroupVersion(strings.TrimSpace(k[:i]))
		if err != nil {
			return "", nil, fmt.Errorf("invalid plugin apiVersion in %q: %w", k, err)
		}
		gvks = append(gvks, gv.WithKind(strings.TrimSpace(k[i+1:])))
	}
	return path, gvks, nil
}

type pluginInput struct {
	Name       string                 `json:"name"`
	Namespace  string                 `json:"namespace"`
	Kind       string                 `json:"kind"`
	APIVersion string                 `json:"apiVersion"`
	ChartName  string                 `json:"chartName"`
	Object     map[string]interface{} `json:"object"`
}

type pluginOutput struct {
	ServiceName     string                 `json:"serviceName"`
	TemplatePath    string                 `json:"templatePath"`
	TemplateContent string                 `json:"templateContent"`
	ValuesPath      string                 `json:"valuesPath"`
	Values          map[string]interface{} `json:"values"`
}

// Process runs the plugin for one resource.
func (p *PluginProcessor) Process(ctx Context, obj *unstructured.Unstructured) (*Result, error) {
	stdin, err := json.Marshal(pluginInput{
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		Kind:       obj.GetKind(),
		APIVersion: obj.GetAPIVersion(),
		ChartName:  ctx.ChartName,
		Object:     obj.Object,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin %s: marshal input: %w", p.path, err)
	}

	parent := ctx.Ctx
	if parent == nil {
		parent = context.Background()
	}
	runCtx, cancel := context.WithTimeout(parent, p.timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, p.path) //nolint:gosec // the user configures the plugin
	cmd.Stdin = bytes.NewReader(stdin)
	// Children of a killed plugin may keep its pipes open; don't wait for them.
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			return nil, fmt.Errorf("plugin %s: timeout after %v", p.path, p.timeout)
		}
		return nil, fmt.Errorf("plugin %s: %w (stderr: %s)", p.path, err, strings.TrimSpace(stderr.String()))
	}

	var out pluginOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("plugin %s: invalid output: %w", p.path, err)
	}
	if out.TemplateContent == "" {
		return &Result{Processed: false}, nil
	}
	if out.ServiceName == "" || out.TemplatePath == "" {
		return nil, fmt.Errorf("plugin %s: output must set serviceName and templatePath", p.path)
	}
	return &Result{
		Processed:       true,
		ServiceName:     out.ServiceName,
		TemplatePath:    out.TemplatePath,
		TemplateContent: out.TemplateContent,
		ValuesPath:      out.ValuesPath,
		Values:          out.Values,
	}, nil
}
