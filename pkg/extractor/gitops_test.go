package extractor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// makeRepo creates a local git repository with manifests on branch "main".
func makeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return "file://" + dir
}

func collect(t *testing.T, e Extractor, opts Options) []*types.ExtractedResource {
	t.Helper()
	if err := e.Validate(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	resCh, errCh := e.Extract(context.Background(), opts)
	var out []*types.ExtractedResource
	for resCh != nil || errCh != nil {
		select {
		case r, ok := <-resCh:
			if !ok {
				resCh = nil
				continue
			}
			out = append(out, r)
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			t.Fatalf("extract error: %v", err)
		}
	}
	return out
}

const cm = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: %s\n  labels:\n    app: %s\n"

func TestGitOpsExtractor_ClonesAndFilters(t *testing.T) {
	url := makeRepo(t, map[string]string{
		"apps/web/cm.yaml":   fmt.Sprintf(cm, "web", "web"),
		"apps/db/cm.yaml":    fmt.Sprintf(cm, "db", "db"),
		"infra/other.yaml":   fmt.Sprintf(cm, "other", "other"),
		"apps/web/README.md": "not yaml",
	})

	e := NewGitOpsExtractor()
	all := collect(t, e, Options{GitURL: url, GitBranch: "main", GitPath: "apps"})
	var names []string
	for _, r := range all {
		names = append(names, r.Object.GetName())
		if r.Source != types.SourceGitOps || !strings.HasPrefix(r.SourcePath, url+"@main:apps/") {
			t.Errorf("unexpected source %s %s", r.Source, r.SourcePath)
		}
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "db,web" {
		t.Errorf("got %v, want [db web] from apps/", names)
	}

	selected := collect(t, e, Options{GitURL: url, LabelSelector: "app=web"})
	if len(selected) != 1 || selected[0].Object.GetName() != "web" {
		t.Errorf("label selector not applied: %d resources", len(selected))
	}
}

func TestGitOpsExtractor_Errors(t *testing.T) {
	e := NewGitOpsExtractor()
	ctx := context.Background()
	if err := e.Validate(ctx, Options{}); err == nil {
		t.Error("expected error without --git-repo")
	}
	if err := e.Validate(ctx, Options{GitURL: "file:///x", GitPath: "../etc"}); err == nil {
		t.Error("expected error for a path escaping the repository")
	}
	if err := e.Validate(ctx, Options{GitURL: "file:///x", GitAuth: &GitAuthOptions{SSHKeyPath: "/nonexistent/key"}}); err == nil {
		t.Error("expected error for a missing ssh key")
	}

	url := makeRepo(t, map[string]string{"a.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n"})
	_, errCh := e.Extract(ctx, Options{GitURL: url, GitBranch: "no-such-branch"})
	var got error
	for err := range errCh {
		got = err
	}
	if got == nil || !strings.Contains(got.Error(), "git clone") {
		t.Errorf("expected a clone error, got %v", got)
	}
}
