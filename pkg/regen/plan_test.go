package regen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/generator"
)

func TestBuildPlanClassification(t *testing.T) {
	dir := t.TempDir()

	base := generator.NewOutputSet()
	theirs := generator.NewOutputSet()

	// 1. Added: not in base, not on disk, is in theirs
	theirs.Add("added.yaml", []byte("new"), 0644)

	// 2. Unchanged (added in base): not in base, is on disk (O=T), is in theirs
	theirs.Add("unchanged.yaml", []byte("same"), 0644)
	os.WriteFile(filepath.Join(dir, "unchanged.yaml"), []byte("same"), 0644)

	// 3. Merged/Conflict (O!=T without base): not in base, is on disk, is in theirs
	theirs.Add("merged.yaml", []byte("new"), 0644)
	os.WriteFile(filepath.Join(dir, "merged.yaml"), []byte("old"), 0644)

	// 4. Kept-Deleted (T=B): in base, not on disk, in theirs
	base.Add("kept-deleted.yaml", []byte("same"), 0644)
	theirs.Add("kept-deleted.yaml", []byte("same"), 0644)

	// 5. Conflict (modify/delete): in base, not on disk, in theirs but T!=B
	base.Add("modify-delete.yaml", []byte("old"), 0644)
	theirs.Add("modify-delete.yaml", []byte("new"), 0644)

	// 6. Updated (O=B, T!=O): in base, on disk, in theirs
	base.Add("updated.yaml", []byte("old"), 0644)
	os.WriteFile(filepath.Join(dir, "updated.yaml"), []byte("old"), 0644)
	theirs.Add("updated.yaml", []byte("new"), 0644)

	// 7. Kept (T=B, O!=B): in base, on disk, in theirs
	base.Add("kept.yaml", []byte("old"), 0644)
	os.WriteFile(filepath.Join(dir, "kept.yaml"), []byte("new-o"), 0644)
	theirs.Add("kept.yaml", []byte("old"), 0644)

	// 8. Removed (O=B): in base, on disk, not in theirs
	base.Add("removed.yaml", []byte("same"), 0644)
	os.WriteFile(filepath.Join(dir, "removed.yaml"), []byte("same"), 0644)

	plan, err := BuildPlan(dir, base, theirs, PlanOptions{})
	if err != nil {
		t.Fatalf("BuildPlan failed: %v", err)
	}

	actions := make(map[string]FileAction)
	for _, fp := range plan.Files {
		actions[fp.Path] = fp.Action
	}

	if actions["added.yaml"] != ActionAdded {
		t.Errorf("Expected added.yaml to be ActionAdded, got %s", actions["added.yaml"])
	}
	if actions["unchanged.yaml"] != ActionUnchanged {
		t.Errorf("Expected unchanged.yaml to be ActionUnchanged, got %s", actions["unchanged.yaml"])
	}
	if actions["merged.yaml"] != ActionConflict {
		t.Errorf("Expected merged.yaml to be ActionConflict, got %s", actions["merged.yaml"])
	}
	if actions["kept-deleted.yaml"] != ActionKeptDeleted {
		t.Errorf("Expected kept-deleted.yaml to be ActionKeptDeleted, got %s", actions["kept-deleted.yaml"])
	}
	if actions["modify-delete.yaml"] != ActionConflict {
		t.Errorf("Expected modify-delete.yaml to be ActionConflict, got %s", actions["modify-delete.yaml"])
	}
	if actions["updated.yaml"] != ActionUpdated {
		t.Errorf("Expected updated.yaml to be ActionUpdated, got %s", actions["updated.yaml"])
	}
	if actions["kept.yaml"] != ActionKept {
		t.Errorf("Expected kept.yaml to be ActionKept, got %s", actions["kept.yaml"])
	}
	if actions["removed.yaml"] != ActionRemoved {
		t.Errorf("Expected removed.yaml to be ActionRemoved, got %s", actions["removed.yaml"])
	}
}

func TestApplyPlan(t *testing.T) {
	dir := t.TempDir()
	p := &Plan{
		Files: []FilePlan{
			{Path: "test.yaml", Action: ActionUpdated, Content: []byte("content"), Mode: 0644},
			{Path: "removed.yaml", Action: ActionRemoved},
		},
	}

	os.WriteFile(filepath.Join(dir, "removed.yaml"), []byte("bye"), 0644)

	if err := p.Apply(dir); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "removed.yaml")); !os.IsNotExist(err) {
		t.Errorf("Expected removed.yaml to be deleted")
	}

	if data, err := os.ReadFile(filepath.Join(dir, "test.yaml")); err != nil || string(data) != "content" {
		t.Errorf("Expected test.yaml with 'content', got %v", string(data))
	}
}
