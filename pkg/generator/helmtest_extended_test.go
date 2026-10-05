package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestGenerateSnapshotTests(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "app",
		Templates: map[string]string{
			"templates/web-deployment.yaml":        "kind: Deployment",
			"templates/hooks/pre-upgrade-job.yaml": "kind: Job",
			"templates/NOTES.txt":                  "notes",
			"tests/web-deployment_test.yaml":       "suite: x",
		},
	}
	got := GenerateSnapshotTests(chart)
	if len(got) != 2 {
		t.Fatalf("expected 2 snapshot suites, got %v", got)
	}
	web := got["tests/web-deployment_snapshot_test.yaml"]
	if !strings.Contains(web, "  - web-deployment.yaml\n") || !strings.Contains(web, "matchSnapshot: {}") {
		t.Errorf("unexpected suite:\n%s", web)
	}
	if hook, ok := got["tests/hooks-pre-upgrade-job_snapshot_test.yaml"]; !ok || !strings.Contains(hook, "  - hooks/pre-upgrade-job.yaml\n") {
		t.Errorf("nested templates must get uniquely named suites: %v", got)
	}
	if GenerateSnapshotTests(nil) != nil {
		t.Error("nil chart must give nil")
	}
}
