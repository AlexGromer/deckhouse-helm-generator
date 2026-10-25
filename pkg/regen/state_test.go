package regen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/generator"
)

func TestStateSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	outSet := generator.NewOutputSet()
	_ = outSet.Add("templates/test.yaml", []byte("test content"), 0644)

	st := &State{
		FormatVersion: 1,
		DHGVersion:    "1.0.0",
		ChartName:     "mychart",
		Mode:          "umbrella",
	}

	if err := Save(dir, st, outSet); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loadedSt, loadedOutSet, err := Load(dir, "mychart")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loadedSt == nil {
		t.Fatalf("Loaded state is nil")
	}

	if loadedSt.DHGVersion != "1.0.0" {
		t.Errorf("expected dhgVersion 1.0.0, got %s", loadedSt.DHGVersion)
	}

	f, ok := loadedOutSet.Get("templates/test.yaml")
	if !ok {
		t.Fatalf("Loaded file not found")
	}
	if string(f.Content) != "test content" {
		t.Errorf("expected content 'test content', got '%s'", string(f.Content))
	}
}

func TestStateLoadInvalidHash(t *testing.T) {
	dir := t.TempDir()

	outSet := generator.NewOutputSet()
	_ = outSet.Add("templates/test.yaml", []byte("test content"), 0644)

	st := &State{
		FormatVersion: 1,
		DHGVersion:    "1.0.0",
		ChartName:     "mychart",
		Mode:          "umbrella",
	}

	if err := Save(dir, st, outSet); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Corrupt file
	basePath := filepath.Join(dir, Dir, "mychart", "base", "templates", "test.yaml.base")
	_ = os.WriteFile(basePath, []byte("corrupted"), 0644)

	_, _, err := Load(dir, "mychart")
	if err == nil {
		t.Fatalf("Expected load to fail due to integrity error")
	}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()

	dhgDir := filepath.Join(dir, Dir)
	_ = os.MkdirAll(filepath.Join(dhgDir, "chart-a"), 0755)
	_ = os.MkdirAll(filepath.Join(dhgDir, "chart-b"), 0755)

	charts, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(charts) != 2 || charts[0] != "chart-a" || charts[1] != "chart-b" {
		t.Errorf("Unexpected discovered charts: %v", charts)
	}
}

func TestEdited(t *testing.T) {
	dir := t.TempDir()

	outSet := generator.NewOutputSet()
	_ = outSet.Add("templates/test.yaml", []byte("test content"), 0644)
    _ = outSet.Add("templates/removed.yaml", []byte("bye"), 0644)

    os.MkdirAll(filepath.Join(dir, "templates"), 0755)
    os.WriteFile(filepath.Join(dir, "templates", "test.yaml"), []byte("changed"), 0644)

	edited, err := Edited(dir, outSet)
	if err != nil {
		t.Fatalf("Edited failed: %v", err)
	}

	if len(edited) != 2 || edited[0] != "templates/removed.yaml" || edited[1] != "templates/test.yaml" {
	    t.Errorf("unexpected edited paths: %v", edited)
	}
}
