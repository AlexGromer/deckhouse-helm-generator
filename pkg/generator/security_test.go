package generator

import (
	"os"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// TestWriteChart_PathTraversal ensures that external file paths cannot escape the chart directory.
func TestWriteChart_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	chart := &types.GeneratedChart{
		Name:      "test-chart",
		ChartYAML: "apiVersion: v2\nname: test-chart\nversion: 0.1.0\n",
		ValuesYAML: "{}",
		Templates:  map[string]string{},
		ExternalFiles: []types.ExternalFileInfo{
			{
				Path:    "../../etc/passwd",
				Content: "pwned",
			},
		},
	}

	err := WriteChart(chart, tmpDir)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}

	// Verify the sensitive file was NOT created
	_, statErr := os.Stat("/etc/passwd.extra")
	if statErr == nil {
		t.Fatal("path traversal succeeded: sensitive file was written")
	}
}

// TestWriteChart_ValidExternalFiles verifies that legitimate external files write correctly.
func TestWriteChart_ValidExternalFiles(t *testing.T) {
	tmpDir := t.TempDir()

	chart := &types.GeneratedChart{
		Name:      "test-chart",
		ChartYAML: "apiVersion: v2\nname: test-chart\nversion: 0.1.0\n",
		ValuesYAML: "{}",
		Templates:  map[string]string{},
		ExternalFiles: []types.ExternalFileInfo{
			{
				Path:    "openapi/values.yaml",
				Content: "type: object",
			},
			{
				Path:    "hooks/startup.sh",
				Content: "#!/bin/sh",
			},
		},
	}

	if err := WriteChart(chart, tmpDir); err != nil {
		t.Fatalf("unexpected error for valid paths: %v", err)
	}
}
