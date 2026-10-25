package regen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/generator"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/merge"
)

type FileAction string

const (
	ActionAdded       FileAction = "added"
	ActionUpdated     FileAction = "updated"
	ActionMerged      FileAction = "merged"
	ActionConflict    FileAction = "conflict"
	ActionRemoved     FileAction = "removed"
	ActionKept        FileAction = "kept"
	ActionKeptDeleted FileAction = "kept-deleted"
	ActionUnchanged   FileAction = "unchanged"
)

type FilePlan struct {
	Path        string
	Action      FileAction
	Content     []byte
	Mode        os.FileMode
	Base        []byte
	Ours        []byte
	Theirs      []byte
	Conflicts   int
	Reformatted bool
	Unresolved  bool
}

type PlanOptions struct {
	DHGVersion string
	ChartName  string
}

type Plan struct {
	Files []FilePlan
}

func isStructuralYAML(path string) bool {
	base := filepath.Base(path)
	if base == "values.yaml" || strings.HasPrefix(base, "values-") && strings.HasSuffix(base, ".yaml") || base == "Chart.yaml" {
		return true
	}
	if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
		return false
	}
	parts := strings.Split(path, "/")
	for _, p := range parts {
		if p == "templates" || p == "crds" {
			return false
		}
	}
	return true
}

func isBinary(data []byte) bool {
	return bytes.Contains(data, []byte{0x00})
}

func hasUnresolvedMarkers(data []byte) bool {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "<<<<<<< ours:") || strings.HasPrefix(line, ">>>>>>> theirs:") || strings.HasPrefix(strings.TrimSpace(line), "# dhg-conflict:") {
			return true
		}
	}
	return false
}

func BuildPlan(outputDir string, base, theirs *generator.OutputSet, o PlanOptions) (*Plan, error) {
	paths := make(map[string]bool)
	if base != nil {
		for _, f := range base.Files {
			paths[f.RelPath] = true
		}
	}
	for _, f := range theirs.Files {
		paths[f.RelPath] = true
	}

	var sortedPaths []string
	for p := range paths {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)

	plan := &Plan{}

	for _, path := range sortedPaths {
		var t *generator.OutputFile
		var bContent, tContent, oContent []byte
		var bExists, tExists, oExists bool
		var oMode os.FileMode

		if base != nil {
			if f, ok := base.Get(path); ok {
				bContent = f.Content
				bExists = true
			}
		}

		if f, ok := theirs.Get(path); ok {
			t = f
			tContent = f.Content
			tExists = true
		}

		fullPath := filepath.Join(outputDir, path)
		if data, err := os.ReadFile(fullPath); err == nil {
			info, _ := os.Stat(fullPath)
			if info.IsDir() {
				return nil, fmt.Errorf("path %s is a directory, but file expected", fullPath)
			}
			oContent = data
			oExists = true
			oMode = info.Mode()
		} else if !os.IsNotExist(err) {
			return nil, err
		}

		fp := FilePlan{
			Path:   path,
			Base:   bContent,
			Ours:   oContent,
			Theirs: tContent,
		}

		if tExists {
			fp.Mode = t.Mode
		} else if oExists {
			fp.Mode = oMode
		}

		if oExists && hasUnresolvedMarkers(oContent) {
			fp.Unresolved = true
		}

		bEqO := bExists && oExists && bytes.Equal(bContent, oContent)
		bEqT := bExists && tExists && bytes.Equal(bContent, tContent)
		oEqT := oExists && tExists && bytes.Equal(oContent, tContent)

		if !bExists && !oExists && tExists {
			fp.Action = ActionAdded
			fp.Content = tContent
		} else if !bExists && oExists && tExists {
			if oEqT {
				fp.Action = ActionUnchanged
				fp.Content = oContent
			} else {
				fp.Action, fp.Content, fp.Conflicts = doMerge(path, nil, oContent, tContent, o)
			}
		} else if bExists && !oExists && tExists {
			if bEqT {
				fp.Action = ActionKeptDeleted
			} else {
				fp.Action = ActionConflict
				fp.Conflicts = 1 // Modify/delete
			}
		} else if bExists && oExists && tExists {
			if bEqO {
				if oEqT {
					fp.Action = ActionUnchanged
					fp.Content = oContent
				} else {
					fp.Action = ActionUpdated
					fp.Content = tContent
				}
			} else if bEqT {
				fp.Action = ActionKept
				fp.Content = oContent
			} else if oEqT {
				fp.Action = ActionUnchanged
				fp.Content = oContent
			} else {
				fp.Action, fp.Content, fp.Conflicts = doMerge(path, bContent, oContent, tContent, o)
			}
		} else if bExists && oExists && !tExists {
			if bEqO {
				fp.Action = ActionRemoved
			} else {
				fp.Action = ActionKept
				fp.Content = oContent
			}
		} else if bExists && !oExists && !tExists {
			fp.Action = ActionKeptDeleted
		}

		// Ensure unresolved markers correctly flag conflict action in plan.
		if fp.Unresolved && fp.Action != ActionConflict {
			if fp.Action == ActionMerged {
				fp.Action = ActionConflict
			}
		}

		plan.Files = append(plan.Files, fp)
	}

	return plan, nil
}

func doMerge(path string, b, o, t []byte, opts PlanOptions) (FileAction, []byte, int) {
	if isBinary(b) || isBinary(o) || isBinary(t) {
		return ActionConflict, o, 1
	}

	labels := merge.Labels{
		Ours:   fmt.Sprintf("ours: %s (current)", path),
		Base:   fmt.Sprintf("base: dhg %s", opts.DHGVersion),
		Theirs: "theirs: dhg new",
	}

	if isStructuralYAML(path) {
		res, err := merge.MergeYAML(b, o, t, merge.YAMLOptions{Labels: labels})
		if err != nil {
			return ActionConflict, o, 1
		}
		if res.Conflict {
			return ActionConflict, []byte(res.Content), res.ConflictCount
		}
		return ActionMerged, []byte(res.Content), 0
	}

	res := merge.Merge3(string(b), string(o), string(t), merge.Merge3Options{
		ShowBase: false,
	})
	if res.Conflict {
		return ActionConflict, []byte(res.Content), res.ConflictCount
	}
	return ActionMerged, []byte(res.Content), 0
}

func (p *Plan) Apply(outputDir string) error {
	var toRemove []string
	for _, f := range p.Files {
		fullPath := filepath.Join(outputDir, f.Path)
		if f.Action == ActionRemoved || f.Action == ActionKeptDeleted {
			if f.Action == ActionRemoved {
				if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
					return err
				}
				toRemove = append(toRemove, filepath.Dir(fullPath))
			}
			continue
		}

		if f.Content != nil && !bytes.Equal(f.Ours, f.Content) {
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				return err
			}
			tmpFile := fullPath + ".dhg-tmp"
			if err := os.WriteFile(tmpFile, f.Content, f.Mode); err != nil {
				return err
			}
			if err := os.Rename(tmpFile, fullPath); err != nil {
				return err
			}
		}
	}

	// Clean up empty directories bottom-up
	sort.Slice(toRemove, func(i, j int) bool {
		return len(toRemove[i]) > len(toRemove[j])
	})
	for _, dir := range toRemove {
		_ = os.Remove(dir) // Ignore error if not empty
	}

	return nil
}
