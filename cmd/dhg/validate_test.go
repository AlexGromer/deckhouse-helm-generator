package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeChart(t *testing.T, templates map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"Chart.yaml":  "apiVersion: v2\nname: app\nversion: 0.1.0\n",
		"values.yaml": "replicas: 1\n",
	}
	for name, content := range templates {
		files[filepath.Join("templates", name)] = content
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestValidate_TemplateSyntax(t *testing.T) {
	good := writeChart(t, map[string]string{
		"_helpers.tpl":   `{{- define "app.name" -}}{{ .Chart.Name | trunc 63 }}{{- end }}`,
		"cm.yaml":        "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ include \"app.name\" . }}\n",
		"hooks/job.yaml": "{{- if .Values.hooks }}\napiVersion: batch/v1\nkind: Job\n{{- end }}\n",
	})
	if err := runValidate(context.Background(), validateOptions{paths: []string{good}, kubeVersions: "1.27-1.32"}); err != nil {
		t.Errorf("valid chart rejected: %v", err)
	}

	// Balanced braces but invalid syntax: the old {{/}} counting accepted this.
	bad := writeChart(t, map[string]string{"hooks/job.yaml": "{{ if }}{{ end }}\n"})
	err := runValidate(context.Background(), validateOptions{paths: []string{bad}})
	if err == nil {
		t.Error("expected a syntax error for a malformed template in a subdirectory")
	}
}

func TestValidate_KubeVersions(t *testing.T) {
	chart := writeChart(t, map[string]string{
		"pdb.yaml": "apiVersion: policy/v1beta1\nkind: PodDisruptionBudget\nmetadata:\n  name: x\n",
	})
	if err := runValidate(context.Background(), validateOptions{paths: []string{chart}, kubeVersions: "1.24"}); err != nil {
		t.Errorf("policy/v1beta1 exists in 1.24, got error: %v", err)
	}
	err := runValidate(context.Background(), validateOptions{paths: []string{chart}, kubeVersions: "1.24-1.25"})
	if err == nil || !strings.Contains(err.Error(), "1 error") {
		t.Errorf("policy/v1beta1 was removed in 1.25, want 1 error, got %v", err)
	}
	if err := runValidate(context.Background(), validateOptions{paths: []string{chart}, kubeVersions: ""}); err != nil {
		t.Errorf("empty --kube-versions must disable the check, got %v", err)
	}
}

func TestParseKubeVersions(t *testing.T) {
	if o, err := parseKubeVersions("1.27-1.32"); err != nil || o.MinVersion != "1.27" || o.MaxVersion != "1.32" {
		t.Errorf("range: %+v %v", o, err)
	}
	if o, err := parseKubeVersions("1.29, 1.31"); err != nil || len(o.TargetVersions) != 2 {
		t.Errorf("list: %+v %v", o, err)
	}
	if o, err := parseKubeVersions(" "); err != nil || o != nil {
		t.Errorf("empty: %+v %v", o, err)
	}
}
