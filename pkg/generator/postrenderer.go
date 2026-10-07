package generator

import (
	"fmt"
	"path"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// postRendererScript is a Helm post-renderer: Helm pipes the rendered
// manifests to its stdin and installs what it prints. It applies the Kustomize
// overlay of one environment on top of the rendered release.
const postRendererScript = `#!/bin/sh
# Helm post-renderer: applies a Kustomize overlay to the manifests Helm rendered.
#
#   helm install REL CHART --post-renderer ./post-renderer/kustomize.sh --post-renderer-args prod
#
# Without --post-renderer-args, DHG_OVERLAY (default: dev) selects the overlay.
# Requires kustomize or kubectl in PATH.
set -eu
overlay="${1:-${DHG_OVERLAY:-dev}}"
here="$(cd "$(dirname "$0")" && pwd)"
if [ ! -d "$here/overlays/$overlay" ]; then
  echo "post-renderer: unknown overlay '$overlay' (see $here/overlays)" >&2
  exit 1
fi
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cp -R "$here/base" "$here/overlays" "$work/"
cat > "$work/base/rendered.yaml"
if command -v kustomize >/dev/null 2>&1; then
  kustomize build "$work/overlays/$overlay"
else
  kubectl kustomize "$work/overlays/$overlay"
fi
`

const postRendererBase = `# Helm's rendered manifests are written to rendered.yaml by kustomize.sh.
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - rendered.yaml
`

const postRendererOverlay = `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
# Environment-specific changes Helm values cannot express, e.g.:
# patches:
#   - target:
#       kind: Deployment
#     patch: |-
#       - op: add
#         path: /metadata/annotations/environment
#         value: %s
`

// GeneratePostRenderer returns the files of a Helm post-renderer layout
// (post-renderer/kustomize.sh, base/ and one overlay per environment), to be
// written next to the chart.
func GeneratePostRenderer(envs []string) []types.ExternalFileInfo {
	files := []types.ExternalFileInfo{
		{Path: "post-renderer/kustomize.sh", Content: postRendererScript},
		{Path: "post-renderer/base/kustomization.yaml", Content: postRendererBase},
	}
	for _, env := range envs {
		files = append(files, types.ExternalFileInfo{
			Path:    path.Join("post-renderer/overlays", env, "kustomization.yaml"),
			Content: fmt.Sprintf(postRendererOverlay, env),
		})
	}
	return files
}

// InjectPostRenderer adds the post-renderer layout to the chart's external
// files. The input chart is not mutated.
func InjectPostRenderer(chart *types.GeneratedChart, envs []string) *types.GeneratedChart {
	out := cloneChart(chart)
	out.ExternalFiles = append(out.ExternalFiles, GeneratePostRenderer(envs)...)
	return out
}
