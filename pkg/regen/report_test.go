package regen

import (
	"strings"
	"testing"
)

func TestPlanSummary(t *testing.T) {
	p := &Plan{
		Files: []FilePlan{
			{Path: "f1", Action: ActionUpdated},
			{Path: "f2", Action: ActionUpdated},
			{Path: "f3", Action: ActionAdded},
			{Path: "f4", Action: ActionConflict, Unresolved: true},
			{Path: "f5", Action: ActionUnchanged},
		},
	}

	sum := p.Summary()
	if !strings.Contains(sum, "Merged 4 files:") {
		t.Errorf("Expected 'Merged 4 files:', got %q", sum)
	}
	if !strings.Contains(sum, "2 updated") {
		t.Errorf("Expected '2 updated', got %q", sum)
	}
	if !strings.Contains(sum, "1 added") {
		t.Errorf("Expected '1 added', got %q", sum)
	}
	if !strings.Contains(sum, "1 conflict -> MERGE.md") {
		t.Errorf("Expected conflict notice, got %q", sum)
	}

	if p.Conflicts() != 1 {
		t.Errorf("Expected 1 conflict, got %d", p.Conflicts())
	}
}

func TestMergeReport(t *testing.T) {
	p := &Plan{
		Files: []FilePlan{
			{
				Path:      "values.yaml",
				Action:    ActionConflict,
				Conflicts: 1,
				Ours:      []byte("1"),
				Theirs:    []byte("2"),
				Base:      []byte("3"),
			},
			{
				Path:      "templates/deploy.yaml",
				Action:    ActionConflict,
				Conflicts: 3,
				Ours:      []byte("some"),
				Theirs:    []byte("data"),
				Base:      []byte("here"),
			},
			{
				Path:      "templates/deleted.yaml",
				Action:    ActionConflict,
				Conflicts: 1,
				Ours:      nil, // Deleted by us
				Theirs:    []byte("data"),
				Base:      []byte("data"),
			},
		},
	}

	report := p.MergeReport()

	if !strings.Contains(report, "<!-- dhg merge report -->") {
		t.Error("Missing report header")
	}
	if !strings.Contains(report, "## Значения YAML") {
		t.Error("Missing YAML section")
	}
	if !strings.Contains(report, "values.yaml") {
		t.Error("Missing values.yaml in report")
	}
	if !strings.Contains(report, "## Текстовые конфликты") {
		t.Error("Missing text conflicts section")
	}
	if !strings.Contains(report, "templates/deploy.yaml — 3 участок") {
		t.Error("Missing text conflict line")
	}
	if !strings.Contains(report, "## Изменено с одной стороны, удалено с другой") {
		t.Error("Missing deleted conflicts section")
	}
	if !strings.Contains(report, "templates/deleted.yaml") {
		t.Error("Missing deleted.yaml line")
	}
}
