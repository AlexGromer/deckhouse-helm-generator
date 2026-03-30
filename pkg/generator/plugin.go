package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// PluginInfo describes a discovered external processor plugin.
type PluginInfo struct {
	// Name is the basename of the plugin executable.
	Name string `json:"name"`

	// Path is the absolute path to the plugin executable.
	Path string `json:"path"`

	// Supports lists resource kinds the plugin claims to handle (optional).
	Supports []string `json:"supports,omitempty"`

	// Version is the plugin version string (optional).
	Version string `json:"version,omitempty"`
}

// pluginInput is the JSON payload sent to the plugin via stdin.
type pluginInput struct {
	Name       string                 `json:"name"`
	Namespace  string                 `json:"namespace"`
	Kind       string                 `json:"kind"`
	APIVersion string                 `json:"apiVersion"`
	Source     string                 `json:"source"`
	SourcePath string                 `json:"sourcePath"`
	Object     map[string]interface{} `json:"object"`
}

// pluginOutput is the JSON payload read from the plugin via stdout.
type pluginOutput struct {
	ServiceName     string                 `json:"serviceName"`
	TemplatePath    string                 `json:"templatePath"`
	TemplateContent string                 `json:"templateContent"`
	ValuesPath      string                 `json:"valuesPath"`
	Values          map[string]interface{} `json:"values"`
}

// RunExternalProcessor executes a plugin binary using JSON stdin/stdout protocol.
// It marshals the input ExtractedResource as JSON, pipes it to the subprocess,
// and parses the stdout as a ProcessedResource. The process is killed if it
// does not finish within timeout.
func RunExternalProcessor(cmd string, input *types.ExtractedResource, timeout time.Duration) (*types.ProcessedResource, error) {
	if input == nil {
		return nil, fmt.Errorf("plugin: input ExtractedResource must not be nil")
	}

	// Build a portable input payload so the plugin receives a clean JSON object.
	payload := pluginInput{
		Source:     string(input.Source),
		SourcePath: input.SourcePath,
	}
	if input.Object != nil {
		payload.Name = input.Object.GetName()
		payload.Namespace = input.Object.GetNamespace()
		payload.Kind = input.Object.GetKind()
		payload.APIVersion = input.Object.GetAPIVersion()
		payload.Object = input.Object.Object
	}

	stdin, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("plugin: marshal input: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	proc := exec.CommandContext(ctx, cmd) //nolint:gosec
	proc.Stdin = bytes.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	proc.Stdout = &stdout
	proc.Stderr = &stderr

	if runErr := proc.Run(); runErr != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("plugin %q: timeout after %v: %w", cmd, timeout, ctx.Err())
		}
		return nil, fmt.Errorf("plugin %q: run: %w (stderr: %s)", cmd, runErr, stderr.String())
	}

	var out pluginOutput
	if decErr := json.NewDecoder(&stdout).Decode(&out); decErr != nil {
		return nil, fmt.Errorf("plugin %q: decode stdout: %w", cmd, decErr)
	}

	return &types.ProcessedResource{
		Original:        input,
		ServiceName:     out.ServiceName,
		TemplatePath:    out.TemplatePath,
		TemplateContent: out.TemplateContent,
		ValuesPath:      out.ValuesPath,
		Values:          out.Values,
	}, nil
}

// DiscoverPlugins scans pluginDir for executable files and returns a PluginInfo
// slice. Non-executable files are skipped. An empty directory returns an empty
// slice without error. A non-existent directory returns an error.
func DiscoverPlugins(pluginDir string) ([]PluginInfo, error) {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil, fmt.Errorf("plugin discover: read dir %q: %w", pluginDir, err)
	}

	var plugins []PluginInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}

		// On Unix, check that at least one executable bit is set.
		if info.Mode()&0o111 == 0 {
			continue
		}

		absPath := filepath.Join(pluginDir, entry.Name())
		plugins = append(plugins, PluginInfo{
			Name: entry.Name(),
			Path: absPath,
		})
	}

	return plugins, nil
}
