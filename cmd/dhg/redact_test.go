package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/extractor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func configMapResource(source types.Source, data map[string]interface{}) *types.ExtractedResource {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]interface{}{"name": "app"},
		"data":     data,
	}}
	return &types.ExtractedResource{Object: u, GVK: u.GroupVersionKind(), Source: source}
}

func TestRedactExtracted(t *testing.T) {
	for _, tc := range []struct {
		source   types.Source
		redact   bool
		redacted bool
	}{
		{types.SourceCluster, true, true},
		{types.SourceFile, true, true},
		{types.SourceGitOps, true, true},
		{types.SourceCluster, false, false},
		{types.SourceFile, false, false},
		// Synthesized manifests already put credentials into Secrets.
		{types.SourceImage, true, false},
		{types.SourceCompose, true, false},
		{types.SourceCode, true, false},
	} {
		r := configMapResource(tc.source, map[string]interface{}{"DB_PASSWORD": "hunter2", "DB_URL": "jdbc:x"})
		note := redactExtracted(pipelineOptions{source: tc.source, redactCredentials: tc.redact}, []*types.ExtractedResource{r})
		got := r.Object.Object["data"].(map[string]interface{})
		if tc.redacted {
			if got["DB_PASSWORD"] != "<SET-ME>" || got["DB_URL"] != "jdbc:x" || !strings.Contains(note, "ConfigMap/app data.DB_PASSWORD") {
				t.Errorf("%s: data = %v note = %q", tc.source, got, note)
			}
		} else if got["DB_PASSWORD"] != "hunter2" || note != "" {
			t.Errorf("%s redact=%v: data = %v note = %q", tc.source, tc.redact, got, note)
		}
	}
}

func TestRedactFlagDefaultsToTrueAndComesFromConfig(t *testing.T) {
	cmd := newGenerateCmd()
	f := cmd.Flags().Lookup("redact-credentials")
	if f == nil || f.DefValue != "true" {
		t.Fatalf("--redact-credentials must exist and default to true: %v", f)
	}

	cfg := filepath.Join(t.TempDir(), "dhg.yaml")
	if err := os.WriteFile(cfg, []byte("redact-credentials: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyConfigFile(cmd, cfg); err != nil {
		t.Fatal(err)
	}
	if v := cmd.Flags().Lookup("redact-credentials").Value.String(); v != "false" {
		t.Errorf("redact-credentials from the config file = %s", v)
	}
}

func TestProcessResourcesReportsRedaction(t *testing.T) {
	r := configMapResource(types.SourceFile, map[string]interface{}{"KC_DB_PASSWORD": "hunter2"})
	res, err := processResources(context.Background(), pipelineOptions{
		source: types.SourceFile, extract: extractor.Options{}, chartName: "app", outputMode: types.OutputModeUniversal,
		redactCredentials: true, quiet: true,
	}, []*types.ExtractedResource{r})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range res.notes {
		if strings.Contains(n, "ConfigMap/app data.KC_DB_PASSWORD") && strings.Contains(n, "--set") {
			found = true
		}
	}
	if !found {
		t.Errorf("notes = %v", res.notes)
	}
	for _, p := range res.processed {
		if strings.Contains(p.TemplateContent, "hunter2") {
			t.Errorf("the template keeps the value:\n%s", p.TemplateContent)
		}
		for _, v := range p.Values {
			if s, ok := v.(map[string]interface{}); ok && strings.Contains(strings.Join(flatten(s), " "), "hunter2") {
				t.Errorf("values keep the value: %v", s)
			}
		}
	}
}

func flatten(m map[string]interface{}) []string {
	var out []string
	for _, v := range m {
		switch t := v.(type) {
		case string:
			out = append(out, t)
		case map[string]interface{}:
			out = append(out, flatten(t)...)
		}
	}
	return out
}
