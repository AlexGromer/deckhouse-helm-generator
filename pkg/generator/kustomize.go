package generator

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// KustomizeOutput holds a Kustomize directory layout built from the input
// manifests: a single base directory and one overlay per target environment.
// It is an alternative to the Helm chart for teams that deploy with
// kustomize, so the base holds plain manifests rather than Helm templates.
type KustomizeOutput struct {
	// Base contains the input resources and their kustomization.yaml.
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
}

// overlaySpec describes how a single overlay environment should be constructed.
type overlaySpec struct {
	name     string
	replicas int
}

// defaultOverlays defines the three standard environments and their parameters.
var defaultOverlays = []overlaySpec{
	{name: "dev", replicas: 1},
	{name: "staging", replicas: 2},
	{name: "prod", replicas: 3},
}

// scaledKinds are the workloads whose replica count the overlays set.
var scaledKinds = map[string]bool{"Deployment": true, "StatefulSet": true}

// GenerateKustomizeLayout builds a Kustomize layout from the given objects:
// the base lists one manifest per object, and every overlay patches the
// replica count of each Deployment and StatefulSet, addressed by kind, name
// and namespace. The objects are not mutated.
//
// Returns an error if there are no objects.
func GenerateKustomizeLayout(objects []*unstructured.Unstructured) (*KustomizeOutput, error) {
	if len(objects) == 0 {
		return nil, fmt.Errorf("no resources")
	}

	resources := make(map[string]string, len(objects))
	resourceNames := make([]string, 0, len(objects))
	var workloads []*unstructured.Unstructured

	for _, obj := range objects {
		name := resourceFileName(obj, resources)
		if strings.Contains(name, "/") {
			return nil, fmt.Errorf("invalid resource %s/%s: name must not contain '/'", obj.GetKind(), obj.GetName())
		}
		if err := validateResourceName(name); err != nil {
			return nil, fmt.Errorf("invalid resource %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}
		manifest := obj.DeepCopy()
		unstructured.RemoveNestedField(manifest.Object, "status")
		data, err := yaml.Marshal(manifest.Object)
		if err != nil {
			return nil, fmt.Errorf("marshal %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}
		resources[name] = string(data)
		resourceNames = append(resourceNames, name)
		if scaledKinds[obj.GetKind()] {
			workloads = append(workloads, obj)
		}
	}
	sort.Strings(resourceNames)
	sort.Slice(workloads, func(i, j int) bool {
		return workloadID(workloads[i]) < workloadID(workloads[j])
	})

	base := &KustomizeDir{
		Path:          "base",
		Kustomization: generateBaseKustomization(resourceNames),
		Resources:     resources,
	}

	overlays := make(map[string]*KustomizeDir, len(defaultOverlays))
	for _, spec := range defaultOverlays {
		overlays[spec.name] = &KustomizeDir{
			Path:          "overlays/" + spec.name,
			Kustomization: generateOverlayKustomization(workloads, spec.replicas),
		}
	}

	return &KustomizeOutput{
		Base:     base,
		Overlays: overlays,
	}, nil
}

// resourceFileName returns "<kind>-<name>.yaml", prefixed with the namespace
// when another namespace already holds an object of that kind and name.
func resourceFileName(obj *unstructured.Unstructured, taken map[string]string) string {
	name := strings.ToLower(obj.GetKind()) + "-" + obj.GetName() + ".yaml"
	if _, exists := taken[name]; exists && obj.GetNamespace() != "" {
		name = obj.GetNamespace() + "-" + name
	}
	return name
}

func workloadID(obj *unstructured.Unstructured) string {
	return obj.GetKind() + "/" + obj.GetNamespace() + "/" + obj.GetName()
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

// generateOverlayKustomization renders an overlay kustomization.yaml that
// references ../../base and sets the replica count of each workload with an
// inline JSON 6902 patch. "add" replaces an existing field and creates a
// missing one, so it works whether or not the input set replicas.
func generateOverlayKustomization(workloads []*unstructured.Unstructured, replicas int) string {
	var b strings.Builder
	b.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	b.WriteString("kind: Kustomization\n")
	b.WriteString("resources:\n")
	b.WriteString("  - ../../base\n")
	if len(workloads) > 0 {
		b.WriteString("patches:\n")
		for _, w := range workloads {
			b.WriteString("  - target:\n")
			fmt.Fprintf(&b, "      kind: %s\n", w.GetKind())
			fmt.Fprintf(&b, "      name: %s\n", w.GetName())
			if ns := w.GetNamespace(); ns != "" {
				fmt.Fprintf(&b, "      namespace: %s\n", ns)
			}
			b.WriteString("    patch: |-\n")
			b.WriteString("      - op: add\n")
			b.WriteString("        path: /spec/replicas\n")
			fmt.Fprintf(&b, "        value: %d\n", replicas)
		}
	}
	return b.String()
}
