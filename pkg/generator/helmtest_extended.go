package generator

import (
	"fmt"
	"path"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// GenerateSnapshotTests returns a helm-unittest snapshot suite per rendered
// template (tests/<template>_snapshot_test.yaml). The first `helm unittest`
// run records __snapshot__/ files; later runs fail when the rendered output
// changes, which turns chart edits into reviewable diffs
// (`helm unittest -u` accepts a change).
func GenerateSnapshotTests(chart *types.GeneratedChart) map[string]string {
	if chart == nil {
		return nil
	}
	result := make(map[string]string)
	for tmplPath := range chart.Templates {
		if !strings.HasPrefix(tmplPath, "templates/") || skipTestFiles[path.Base(tmplPath)] {
			continue
		}
		if ext := path.Ext(tmplPath); ext != ".yaml" && ext != ".yml" {
			continue
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(tmplPath, "templates/"), path.Ext(tmplPath))
		name := strings.ReplaceAll(rel, "/", "-")
		result[fmt.Sprintf("tests/%s_snapshot_test.yaml", name)] = fmt.Sprintf(`suite: %[1]s snapshot
templates:
  - %[2]s
tests:
  - it: renders the same manifest as the recorded snapshot
    asserts:
      - matchSnapshot: {}
`, name, strings.TrimPrefix(tmplPath, "templates/"))
	}
	return result
}
