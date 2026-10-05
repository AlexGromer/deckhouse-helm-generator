package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestInjectPostRenderer_Layout(t *testing.T) {
	chart := &types.GeneratedChart{Name: "app", Templates: map[string]string{}}
	out := InjectPostRenderer(chart, []string{"dev", "prod"})

	got := map[string]string{}
	for _, f := range out.ExternalFiles {
		got[f.Path] = f.Content
	}
	for _, p := range []string{
		"post-renderer/kustomize.sh",
		"post-renderer/base/kustomization.yaml",
		"post-renderer/overlays/dev/kustomization.yaml",
		"post-renderer/overlays/prod/kustomization.yaml",
	} {
		content, ok := got[p]
		if !ok {
			t.Errorf("missing %s", p)
			continue
		}
		if strings.HasSuffix(p, ".yaml") {
			var m map[string]interface{}
			if err := yaml.Unmarshal([]byte(content), &m); err != nil {
				t.Errorf("%s is not valid YAML: %v", p, err)
			}
		}
	}
	if !strings.Contains(got["post-renderer/overlays/prod/kustomization.yaml"], "../../base") {
		t.Error("overlay must build on the base")
	}
	if len(chart.ExternalFiles) != 0 {
		t.Error("input chart mutated")
	}
}

// The script must be valid POSIX shell.
func TestPostRendererScript_Syntax(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	script := filepath.Join(t.TempDir(), "kustomize.sh")
	if err := os.WriteFile(script, []byte(postRendererScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(sh, "-n", script).CombinedOutput(); err != nil {
		t.Errorf("sh -n failed: %v\n%s", err, out)
	}
}
