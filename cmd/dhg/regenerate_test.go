package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The second generate into the same directory needs --update or --force
// (the first run saved the state): the flags the error message names must exist.
func TestGenerate_RegenerateIntoSameDirectory(t *testing.T) {
	src := t.TempDir()
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: web
          image: nginx:1.27
`
	if err := os.WriteFile(filepath.Join(src, "web.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "chart")
	generate := func(extra ...string) (string, error) {
		args := append([]string{"generate", "-f", src, "-o", out, "--chart-name", "app"}, extra...)
		return executeCmd(t, args...)
	}

	if _, err := generate(); err != nil {
		t.Fatalf("first generate: %v", err)
	}
	if _, err := generate(); err == nil || !strings.Contains(err.Error(), "--update") {
		t.Fatalf("second generate must refuse and name --update/--force, got %v", err)
	}
	if _, err := generate("--force"); err != nil {
		t.Errorf("--force: %v", err)
	}
	if _, err := generate("--update"); err != nil {
		t.Errorf("--update: %v", err)
	}
	if _, err := generate("--update", "--force"); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("--update with --force must be rejected, got %v", err)
	}
	if _, err := generate("--force", "--no-state"); err != nil {
		t.Errorf("--no-state: %v", err)
	}
}
