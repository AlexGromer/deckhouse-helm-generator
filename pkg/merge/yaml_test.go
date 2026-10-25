package merge

import (
	"strings"
	"testing"
)

func TestMergeYAML_CleanMerge(t *testing.T) {
	base := []byte(`
k1: v1
k2: v2
`)
	ours := []byte(`
k1: v1_ours
k2: v2
`)
	theirs := []byte(`
k1: v1
k2: v2
k3: v3
`)

	res, err := MergeYAML(base, ours, theirs, YAMLOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Conflict {
		t.Errorf("expected no conflicts")
	}

	content := res.Content
	if !strings.Contains(content, "k1: v1_ours") || !strings.Contains(content, "k3: v3") {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestMergeYAML_ArrayAtomic(t *testing.T) {
	base := []byte(`
arr:
  - a
  - b
`)
	ours := []byte(`
arr:
  - a
  - b
  - c
`)
	theirs := []byte(`
arr:
  - a
  - b
`)

	res, err := MergeYAML(base, ours, theirs, YAMLOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Conflict {
		t.Errorf("expected no conflicts")
	}
	if !strings.Contains(res.Content, "- c") {
		t.Errorf("expected - c in array: %s", res.Content)
	}

	// both change -> conflict
	theirs2 := []byte(`
arr:
  - a
  - d
`)
	res2, _ := MergeYAML(base, ours, theirs2, YAMLOptions{})
	if !res2.Conflict {
		t.Errorf("expected conflict")
	}
}

func TestMergeYAML_KeyedArray(t *testing.T) {
	base := []byte(`
containers:
  - name: c1
    image: img1
`)
	ours := []byte(`
containers:
  - name: c1
    image: img1_ours
`)
	theirs := []byte(`
containers:
  - name: c1
    image: img1
    port: 80
  - name: c2
    image: img2
`)

	res, err := MergeYAML(base, ours, theirs, YAMLOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Conflict {
		t.Errorf("expected no conflicts")
	}

	if !strings.Contains(res.Content, "image: img1_ours") || !strings.Contains(res.Content, "port: 80") || !strings.Contains(res.Content, "name: c2") {
		t.Errorf("unexpected content: %s", res.Content)
	}
}

func TestMergeYAML_Conflict(t *testing.T) {
	base := []byte(`
replicas: 3
`)
	ours := []byte(`
replicas: 5
`)
	theirs := []byte(`
replicas: 4
`)

	res, err := MergeYAML(base, ours, theirs, YAMLOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Conflict {
		t.Fatalf("expected conflict")
	}

	if res.ConflictCount != 1 {
		t.Errorf("expected 1 conflict, got %d", res.ConflictCount)
	}

	if !strings.Contains(res.Content, "# dhg-conflict: generator changed this value from 3 to 4; your value 5 is kept (see MERGE.md)") {
		t.Errorf("expected dhg-conflict comment, got %s", res.Content)
	}
}

func TestMergeYAML_Fallback(t *testing.T) {
	// Not an object at root
	base := []byte(`- a`)
	ours := []byte(`- a\n- b`)
	theirs := []byte(`- a\n- c`)

	res, err := MergeYAML(base, ours, theirs, YAMLOptions{ShowBase: true})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Should fallback to Merge3
	if !strings.Contains(res.Content, "<<<<<<< user") {
		t.Errorf("expected text merge fallback with conflict markers")
	}
}
