package extractor

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func extracted(kind, ns, name, path string) *types.ExtractedResource {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("v1")
	obj.SetKind(kind)
	obj.SetNamespace(ns)
	obj.SetName(name)
	return &types.ExtractedResource{Object: obj, GVK: obj.GroupVersionKind(), SourcePath: path}
}

func TestDeduplicate(t *testing.T) {
	in := []*types.ExtractedResource{
		extracted("ConfigMap", "a", "x", "1.yaml"),
		extracted("ConfigMap", "b", "x", "1.yaml"), // other namespace: distinct
		extracted("Secret", "a", "x", "1.yaml"),    // other kind: distinct
		extracted("ConfigMap", "a", "x", "2.yaml"), // duplicate of the first
		nil,
	}
	kept, dups := Deduplicate(in)
	if len(kept) != 3 || kept[0].SourcePath != "1.yaml" || kept[1].Object.GetNamespace() != "b" {
		t.Errorf("kept = %v", kept)
	}
	if len(dups) != 1 || dups[0].SourcePath != "2.yaml" {
		t.Errorf("duplicates = %v", dups)
	}
}
