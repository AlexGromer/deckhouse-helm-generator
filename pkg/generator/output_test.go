package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestOutputSet_Add(t *testing.T) {
	set := NewOutputSet()

	// Add valid path
	if err := set.Add("a/b.txt", []byte("content"), 0644); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// Duplicate exact match should be no-op
	if err := set.Add("a/b.txt", []byte("content"), 0644); err != nil {
		t.Fatalf("expected nil for exact duplicate, got %v", err)
	}

	// Duplicate different content should error
	if err := set.Add("a/b.txt", []byte("diff"), 0644); err == nil {
		t.Fatalf("expected error for duplicate path with different content")
	}

	// Invalid paths
	invalidPaths := []string{"../outside", ".", "..", "a/../../outside"}
	for _, p := range invalidPaths {
		if err := set.Add(p, []byte("x"), 0644); err == nil {
			t.Errorf("expected error for path %s", p)
		}
	}
}

func TestOutputSet_ByPath(t *testing.T) {
	set := NewOutputSet()
	_ = set.Add("a.txt", []byte("a"), 0644)
	_ = set.Add("b/c.txt", []byte("c"), 0644)

	m := set.ByPath()
	if string(m["a.txt"]) != "a" || string(m["b/c.txt"]) != "c" {
		t.Fatalf("ByPath returned unexpected map: %v", m)
	}
}

func TestOutputSet_Write(t *testing.T) {
	tmpDir := t.TempDir()

	set := NewOutputSet()
	_ = set.Add("file1.txt", []byte("first"), 0644)
	_ = set.Add("dir/file2.sh", []byte("second"), 0755)

	if err := set.Write(tmpDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	b1, err := os.ReadFile(filepath.Join(tmpDir, "file1.txt"))
	if err != nil || string(b1) != "first" {
		t.Errorf("file1.txt issue: %v, %s", err, b1)
	}
	info, _ := os.Stat(filepath.Join(tmpDir, "file1.txt"))
	if info.Mode() != 0644 {
		t.Errorf("expected file1.txt mode 0644, got %v", info.Mode())
	}

	b2, err := os.ReadFile(filepath.Join(tmpDir, "dir", "file2.sh"))
	if err != nil || string(b2) != "second" {
		t.Errorf("file2.sh issue: %v, %s", err, b2)
	}
	info2, _ := os.Stat(filepath.Join(tmpDir, "dir", "file2.sh"))
	if info2.Mode() != 0755 {
		t.Errorf("expected file2.sh mode 0755, got %v", info2.Mode())
	}
}

func TestRenderChartOutput(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:         "app",
		ChartYAML:    "chart: true",
		ValuesYAML:   "values: true",
		ValuesSchema: "{}",
		Helpers:      "{{- define }}",
		Notes:        "hello",
		Templates: map[string]string{
			"templates/a.yaml": "a: b",
		},
		ExternalFiles: []types.ExternalFileInfo{
			{Path: "README.md", Content: "docs"},
			{Path: "hooks/script.sh", Content: "#!/bin/sh"},
		},
	}

	set, err := RenderChartOutput(chart)
	if err != nil {
		t.Fatalf("RenderChartOutput failed: %v", err)
	}

	m := set.ByPath()
	expectedPaths := []string{
		"app/.helmignore",
		"app/Chart.yaml",
		"app/values.yaml",
		"app/values.schema.json",
		"app/templates/_helpers.tpl",
		"app/templates/NOTES.txt",
		"app/templates/a.yaml",
		"app/README.md",
		"app/hooks/script.sh",
	}

	for _, p := range expectedPaths {
		if _, ok := m[p]; !ok {
			t.Errorf("missing path: %s", p)
		}
	}

	// Check mode for script
	for _, f := range set.Files {
		if f.RelPath == "app/hooks/script.sh" {
			if f.Mode != 0755 {
				t.Errorf("script should have mode 0755, got %v", f.Mode)
			}
		} else {
			if f.Mode != 0644 {
				t.Errorf("%s should have mode 0644, got %v", f.RelPath, f.Mode)
			}
		}
	}
}

func TestRenderOutputSet(t *testing.T) {
	charts := []*types.GeneratedChart{
		{
			Name: "app",
			Templates: map[string]string{
				"templates/x.yaml": "x",
			},
		},
	}
	set, err := RenderOutputSet(charts, types.OutputModeUniversal, "app")
	if err != nil {
		t.Fatalf("RenderOutputSet failed: %v", err)
	}
	if len(set.Files) == 0 {
		t.Errorf("expected files in set")
	}
}
