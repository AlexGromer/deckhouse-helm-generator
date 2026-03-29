package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// PostRendererEnv represents a deployment environment.
type PostRendererEnv string

const (
	PostRendererEnvDev     PostRendererEnv = "dev"
	PostRendererEnvStaging PostRendererEnv = "staging"
	PostRendererEnvProd    PostRendererEnv = "prod"
)

// PostRendererOptions configures a post-renderer layout generation.
type PostRendererOptions struct {
	ChartName string
	Namespace string
	Envs      []PostRendererEnv
}

// StrategicMergePatch represents a strategic merge patch definition.
type StrategicMergePatch struct {
	FileName string
	Content  string
}

// JSON6902Target identifies the target resource for a JSON6902 patch.
type JSON6902Target struct {
	Group     string
	Version   string
	Kind      string
	Name      string
	Namespace string
}

// JSON6902Op represents a single JSON Patch operation.
type JSON6902Op struct {
	Op    string
	Path  string
	Value interface{}
}

// JSON6902Patch represents a JSON6902 patch definition.
type JSON6902Patch struct {
	FileName string
	Target   JSON6902Target
	Ops      []JSON6902Op
}

// PostRendererOverlay holds a per-environment Kustomize overlay.
type PostRendererOverlay struct {
	Env                   PostRendererEnv
	ChartName             string
	Namespace             string
	StrategicMergePatches []StrategicMergePatch
	JSON6902Patches       []JSON6902Patch
}

// PostRendererOutput holds the result of a post-renderer layout generation.
type PostRendererOutput struct {
	BaseDir  string
	Overlays []*PostRendererOverlay
}

// ValidatePostRendererOptions validates options and returns a list of errors.
func ValidatePostRendererOptions(opts PostRendererOptions) []error {
	var errs []error
	if opts.ChartName == "" {
		errs = append(errs, fmt.Errorf("ChartName is required"))
	}
	return errs
}

// BuildDefaultPostRendererOptions returns default PostRendererOptions for a chart and namespace.
func BuildDefaultPostRendererOptions(chart *types.GeneratedChart, namespace string) PostRendererOptions {
	name := ""
	if chart != nil {
		name = chart.Name
	}
	return PostRendererOptions{
		ChartName: name,
		Namespace: namespace,
		Envs:      []PostRendererEnv{PostRendererEnvDev, PostRendererEnvStaging, PostRendererEnvProd},
	}
}

// GeneratePostRendererLayout generates a Kustomize overlay layout for multi-environment deployments.
func GeneratePostRendererLayout(chart *types.GeneratedChart, opts PostRendererOptions) (*PostRendererOutput, error) {
	if chart == nil {
		return nil, fmt.Errorf("chart is nil")
	}

	baseDir := opts.ChartName + "-overlays"
	if baseDir == "" {
		baseDir = "chart-overlays"
	}

	overlays := make([]*PostRendererOverlay, 0, len(opts.Envs))
	for _, env := range opts.Envs {
		ns := opts.Namespace
		if ns == "" {
			ns = string(env)
		}
		overlays = append(overlays, &PostRendererOverlay{
			Env:       env,
			ChartName: opts.ChartName,
			Namespace: ns,
		})
	}

	return &PostRendererOutput{
		BaseDir:  baseDir,
		Overlays: overlays,
	}, nil
}

// RenderOverlayKustomization renders a Kustomize kustomization.yaml for a given overlay.
func RenderOverlayKustomization(overlay *PostRendererOverlay) (string, error) {
	if overlay == nil {
		return "", fmt.Errorf("overlay is nil")
	}

	var sb strings.Builder
	sb.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	sb.WriteString("kind: Kustomization\n")

	if overlay.Namespace != "" {
		sb.WriteString(fmt.Sprintf("namespace: %s\n", overlay.Namespace))
	}

	hasPatches := len(overlay.StrategicMergePatches) > 0 || len(overlay.JSON6902Patches) > 0
	if hasPatches {
		sb.WriteString("patches:\n")
		for _, p := range overlay.StrategicMergePatches {
			sb.WriteString(fmt.Sprintf("- path: %s\n", p.FileName))
		}
		for _, p := range overlay.JSON6902Patches {
			sb.WriteString(fmt.Sprintf("- path: %s\n", p.FileName))
			sb.WriteString("  target:\n")
			if p.Target.Group != "" {
				sb.WriteString(fmt.Sprintf("    group: %s\n", p.Target.Group))
			}
			if p.Target.Version != "" {
				sb.WriteString(fmt.Sprintf("    version: %s\n", p.Target.Version))
			}
			if p.Target.Kind != "" {
				sb.WriteString(fmt.Sprintf("    kind: %s\n", p.Target.Kind))
			}
			if p.Target.Name != "" {
				sb.WriteString(fmt.Sprintf("    name: %s\n", p.Target.Name))
			}
		}
	}

	return sb.String(), nil
}

// InjectPostRenderer injects post-renderer overlay files into a chart's ExternalFiles.
func InjectPostRenderer(chart *types.GeneratedChart, opts PostRendererOptions) (*types.GeneratedChart, int, error) {
	if chart == nil {
		return nil, 0, fmt.Errorf("chart is nil")
	}

	output, err := GeneratePostRendererLayout(chart, opts)
	if err != nil {
		return nil, 0, err
	}

	result := copyChartTemplates(chart)
	count := 0

	for _, overlay := range output.Overlays {
		kustomization, err := RenderOverlayKustomization(overlay)
		if err != nil {
			return nil, 0, fmt.Errorf("render overlay %q: %w", overlay.Env, err)
		}
		path := fmt.Sprintf("overlays/%s/kustomization.yaml", overlay.Env)
		result.ExternalFiles = append(result.ExternalFiles, types.ExternalFileInfo{
			Path:    path,
			Content: kustomization,
		})
		count++
	}

	return result, count, nil
}
