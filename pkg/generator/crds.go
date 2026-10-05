package generator

import (
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// isCRD reports whether r is a CustomResourceDefinition. CRDs are not
// templated: Helm installs the chart's crds/ directory before rendering
// templates, so custom resources of the same release can be created on the
// first install (a CRD template would only exist after the resources that
// need it were rejected with "no matches for kind"). Helm never upgrades or
// deletes files in crds/, which is the safe default for CRDs.
func isCRD(r *types.ProcessedResource) bool {
	return r != nil && r.Original != nil && r.Original.GVK.Kind == "CustomResourceDefinition"
}

// crdFiles returns crds/<name>.yaml files for the CRDs among resources, with
// server-populated fields removed.
func crdFiles(resources []*types.ProcessedResource) []types.ExternalFileInfo {
	var files []types.ExternalFileInfo
	for _, r := range resources {
		if !isCRD(r) || r.Original.Object == nil {
			continue
		}
		obj := r.Original.Object.DeepCopy()
		unstructured.RemoveNestedField(obj.Object, "status")
		for _, field := range []string{"uid", "resourceVersion", "generation", "creationTimestamp", "managedFields"} {
			unstructured.RemoveNestedField(obj.Object, "metadata", field)
		}
		data, err := yaml.Marshal(obj.Object)
		if err != nil {
			continue
		}
		files = append(files, types.ExternalFileInfo{Path: "crds/" + obj.GetName() + ".yaml", Content: string(data)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}
