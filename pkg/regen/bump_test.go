package regen

import (
	"strings"
	"testing"
)

func TestBumpSemver(t *testing.T) {
	tests := []struct {
		version, policy, want string
	}{
		{"1.0.0", "patch", "1.0.1"},
		{"1.0.1", "minor", "1.1.0"},
		{"1.1.0", "major", "2.0.0"},
		{"1.2.3-rc.1", "patch", "1.2.4"},
	}

	for _, tt := range tests {
		got, err := bumpSemver(tt.version, tt.policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != tt.want {
			t.Errorf("bumpSemver(%q, %q) = %q, want %q", tt.version, tt.policy, got, tt.want)
		}
	}
}

func TestPlanBump(t *testing.T) {
	p := &Plan{
		Files: []FilePlan{
			{
				Path:    "charts/app/Chart.yaml",
				Action:  ActionUnchanged,
				Content: []byte("version: 1.0.0\n"),
			},
			{
				Path:    "charts/app/templates/deploy.yaml",
				Action:  ActionUpdated,
				Content: []byte("changed"),
			},
			{
				Path:    "Chart.yaml",
				Action:  ActionUnchanged,
				Content: []byte("version: 0.1.0\ndependencies:\n  - name: app\n    version: 1.0.0\n"),
			},
		},
	}

	if err := p.Bump("patch", nil); err != nil {
		t.Fatalf("Bump failed: %v", err)
	}

	var appChart, rootChart string
	for _, fp := range p.Files {
		if fp.Path == "charts/app/Chart.yaml" {
			appChart = string(fp.Content)
		} else if fp.Path == "Chart.yaml" {
			rootChart = string(fp.Content)
		}
	}

	if !strings.Contains(appChart, "version: 1.0.1") {
		t.Errorf("Expected app chart to be bumped to 1.0.1, got:\n%s", appChart)
	}

	if !strings.Contains(rootChart, "version: 1.0.1") {
		t.Errorf("Expected root chart dependency to be updated to 1.0.1, got:\n%s", rootChart)
	}
}
