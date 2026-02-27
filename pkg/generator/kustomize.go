package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// KustomizeOutput holds the full Kustomize directory layout generated from a
// Helm chart: a single base directory and one overlay per target environment.
type KustomizeOutput struct {
	// Base contains the shared resources and their kustomization.yaml.
	Base *KustomizeDir

	// Overlays maps environment names (e.g. "dev", "staging", "prod") to their
	// overlay directories.
	Overlays map[string]*KustomizeDir
}

// KustomizeDir represents a single directory in the Kustomize layout.
type KustomizeDir struct {
	// Path is the logical directory path (e.g. "base" or "overlays/dev").
	Path string

	// Kustomization is the rendered kustomization.yaml content.
	Kustomization string

	// Resources maps resource filenames to their YAML content.
	Resources map[string]string

	// Patches lists strategic-merge patches applied by this directory.
	Patches []KustomizePatch
}

// KustomizePatch describes a single strategic-merge patch file.
type KustomizePatch struct {
	// Target is the patch filename (e.g. "replica-patch.yaml").
	Target string

	// Patch is the rendered YAML content of the patch.
	Patch string
}

// overlaySpec describes how a single overlay environment should be constructed.
type overlaySpec struct {
	name     string
	replicas int
	// addResourceLimits controls whether a resource-limits patch is generated.
	addResourceLimits bool
}

// defaultOverlays defines the three standard environments and their parameters.
var defaultOverlays = []overlaySpec{
	{name: "dev", replicas: 1, addResourceLimits: false},
	{name: "staging", replicas: 2, addResourceLimits: false},
	{name: "prod", replicas: 3, addResourceLimits: true},
}

// GenerateKustomizeLayout converts a Helm GeneratedChart into a Kustomize
// directory layout with a shared base and per-environment overlays.
//
// Returns an error if the chart contains no templates.
func GenerateKustomizeLayout(chart *types.GeneratedChart) (*KustomizeOutput, error) {
	if len(chart.Templates) == 0 {
		return nil, fmt.Errorf("no templates")
	}

	// Build base resources by stripping the "templates/" prefix from each key.
	resources := make(map[string]string, len(chart.Templates))
	resourceNames := make([]string, 0, len(chart.Templates))

	for path, content := range chart.Templates {
		name := strings.TrimPrefix(path, "templates/")
		resources[name] = content
		resourceNames = append(resourceNames, name)
	}
	sort.Strings(resourceNames)

	base := &KustomizeDir{
		Path:          "base",
		Kustomization: generateBaseKustomization(resourceNames),
		Resources:     resources,
	}

	// Build overlays.
	overlays := make(map[string]*KustomizeDir, len(defaultOverlays))
	for _, spec := range defaultOverlays {
		overlays[spec.name] = buildOverlay(spec)
	}

	return &KustomizeOutput{
		Base:     base,
		Overlays: overlays,
	}, nil
}

// buildOverlay constructs a KustomizeDir for the given overlay spec.
func buildOverlay(spec overlaySpec) *KustomizeDir {
	var patches []KustomizePatch
	var patchNames []string

	// Every overlay gets a replica patch.
	replicaPatch := generateReplicaPatch(spec.name, spec.replicas)
	patches = append(patches, KustomizePatch{
		Target: "replica-patch.yaml",
		Patch:  replicaPatch,
	})
	patchNames = append(patchNames, "replica-patch.yaml")

	// Prod gets an additional resource-limits patch.
	if spec.addResourceLimits {
		limitsPatch := generateResourceLimitsPatch()
		patches = append(patches, KustomizePatch{
			Target: "resource-limits-patch.yaml",
			Patch:  limitsPatch,
		})
		patchNames = append(patchNames, "resource-limits-patch.yaml")
	}

	return &KustomizeDir{
		Path:          "overlays/" + spec.name,
		Kustomization: generateOverlayKustomization(spec.name, patchNames),
		Patches:       patches,
	}
}

// generateBaseKustomization renders a kustomization.yaml that lists the given
// resource filenames. resourceNames must already be sorted alphabetically.
func generateBaseKustomization(resourceNames []string) string {
	var b strings.Builder
	b.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	b.WriteString("kind: Kustomization\n")
	b.WriteString("resources:\n")
	for _, name := range resourceNames {
		b.WriteString("  - ")
		b.WriteString(name)
		b.WriteByte('\n')
	}
	return b.String()
}

// generateOverlayKustomization renders a kustomization.yaml for an overlay
// environment that references ../../base and applies the listed patches via
// patchesStrategicMerge.
func generateOverlayKustomization(env string, patches []string) string {
	var b strings.Builder
	b.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	b.WriteString("kind: Kustomization\n")
	b.WriteString("resources:\n")
	b.WriteString("  - ../../base\n")
	if len(patches) > 0 {
		b.WriteString("patchesStrategicMerge:\n")
		for _, p := range patches {
			b.WriteString("  - ")
			b.WriteString(p)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// generateReplicaPatch renders a strategic-merge patch that sets the replica
// count for a Deployment.
func generateReplicaPatch(env string, replicas int) string {
	var b strings.Builder
	b.WriteString("apiVersion: apps/v1\n")
	b.WriteString("kind: Deployment\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: app\n")
	b.WriteString("spec:\n")
	b.WriteString(fmt.Sprintf("  replicas: %d\n", replicas))
	return b.String()
}

// generateResourceLimitsPatch renders a strategic-merge patch that sets
// container resource limits suitable for production workloads.
func generateResourceLimitsPatch() string {
	var b strings.Builder
	b.WriteString("apiVersion: apps/v1\n")
	b.WriteString("kind: Deployment\n")
	b.WriteString("metadata:\n")
	b.WriteString("  name: app\n")
	b.WriteString("spec:\n")
	b.WriteString("  template:\n")
	b.WriteString("    spec:\n")
	b.WriteString("      containers:\n")
	b.WriteString("        - name: app\n")
	b.WriteString("          resources:\n")
	b.WriteString("            limits:\n")
	b.WriteString("              cpu: \"1\"\n")
	b.WriteString("              memory: 512Mi\n")
	b.WriteString("            requests:\n")
	b.WriteString("              cpu: 250m\n")
	b.WriteString("              memory: 128Mi\n")
	return b.String()
}
