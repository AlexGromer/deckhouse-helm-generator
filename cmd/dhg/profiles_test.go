package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValuesOverlay(t *testing.T) {
	base := map[string]interface{}{
		"same":    1.0,
		"changed": "a",
		"gone":    true,
		"nested":  map[string]interface{}{"keep": "x", "port": 8080.0},
		"list":    []interface{}{"a", "b"},
		"typeMix": map[string]interface{}{"a": 1.0},
	}
	variant := map[string]interface{}{
		"same":    1.0,
		"changed": "b",
		"added":   "new",
		"nested":  map[string]interface{}{"keep": "x", "port": 9000.0},
		"list":    []interface{}{"a", "c"},
		"typeMix": "scalar",
	}
	want := map[string]interface{}{
		"changed": "b",
		"added":   "new",
		"gone":    nil,
		"nested":  map[string]interface{}{"port": 9000.0},
		"list":    []interface{}{"a", "c"},
		"typeMix": "scalar",
	}
	if got := valuesOverlay(base, variant); !reflect.DeepEqual(got, want) {
		t.Errorf("overlay = %v\nwant %v", got, want)
	}
	if len(valuesOverlay(base, base)) != 0 {
		t.Error("identical values give an empty overlay")
	}
}

func TestGenerateWritesProfileValues(t *testing.T) {
	project := "../../tests/golden/testdata/synth/orders-service"
	for _, mode := range []string{"universal", "separate"} {
		out := t.TempDir()
		if _, err := executeCmd(t, "generate", "-s", "source", "-f", project, "--image", "orders:2", "-o", out, "--chart-name", "orders", "--mode", mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		files, _ := filepath.Glob(filepath.Join(out, "*", "values-profile-local.yaml"))
		if len(files) != 1 {
			t.Fatalf("%s: values-profile-local.yaml files = %v", mode, files)
		}
		data, _ := os.ReadFile(files[0])
		for _, want := range []string{"SPRING_PROFILES_ACTIVE: local", "9000"} {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s: overlay lacks %q:\n%s", mode, want, data)
			}
		}
		report, _ := os.ReadFile(filepath.Join(out, "SYNTHESIS.md"))
		if !strings.Contains(string(report), "configuration profiles local") {
			t.Errorf("%s: SYNTHESIS.md does not mention the profile", mode)
		}
	}
}
