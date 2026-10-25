package regen

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/generator"
)

const Dir = ".dhg"

type FileState struct {
	Path   string      `json:"path"`
	SHA256 string      `json:"sha256"`
	Mode   os.FileMode `json:"mode"`
}

type State struct {
	FormatVersion int               `json:"formatVersion"`
	DHGVersion    string            `json:"dhgVersion"`
	ChartName     string            `json:"chartName"`
	Mode          string            `json:"mode"`
	Options       map[string]string `json:"options"`
	Files         []FileState       `json:"files"`
}

func statePath(outputDir, chartName string) string {
	return filepath.Join(outputDir, Dir, chartName, "state.json")
}

func basePath(outputDir, chartName string) string {
	return filepath.Join(outputDir, Dir, chartName, "base")
}

func fileSHA256(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func Load(outputDir, chartName string) (*State, *generator.OutputSet, error) {
	stateFile := statePath(outputDir, chartName)
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, nil, fmt.Errorf("failed to parse state.json: %w", err)
	}

	if st.FormatVersion != 1 {
		return nil, nil, fmt.Errorf("unsupported state format version: %d", st.FormatVersion)
	}

	baseDir := basePath(outputDir, chartName)
	outSet := generator.NewOutputSet()

	for _, fs := range st.Files {
		path := filepath.Join(baseDir, fs.Path+".base")
		fileData, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read base file %s: %w", path, err)
		}

		if hash := fileSHA256(fileData); hash != fs.SHA256 {
			return nil, nil, fmt.Errorf("integrity error: %s has hash %s, expected %s", path, hash, fs.SHA256)
		}

		if err := outSet.Add(fs.Path, fileData, fs.Mode); err != nil {
			return nil, nil, err
		}
	}

	return &st, outSet, nil
}

func Discover(outputDir string) ([]string, error) {
	dhgDir := filepath.Join(outputDir, Dir)
	entries, err := os.ReadDir(dhgDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var charts []string
	for _, entry := range entries {
		if entry.IsDir() {
			charts = append(charts, entry.Name())
		}
	}
	sort.Strings(charts)
	return charts, nil
}

func Save(outputDir string, st *State, theirs *generator.OutputSet) error {
	baseDir := basePath(outputDir, st.ChartName)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return err
	}

	st.Files = make([]FileState, 0, theirs.Len())
	for _, f := range theirs.Files {
		dest := filepath.Join(baseDir, f.RelPath+".base")
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}

		tmpDest := dest + ".tmp"
		if err := os.WriteFile(tmpDest, f.Content, f.Mode); err != nil {
			return err
		}
		if err := os.Rename(tmpDest, dest); err != nil {
			return err
		}

		st.Files = append(st.Files, FileState{
			Path:   f.RelPath,
			SHA256: fileSHA256(f.Content),
			Mode:   f.Mode,
		})
	}

	sort.Slice(st.Files, func(i, j int) bool {
		return st.Files[i].Path < st.Files[j].Path
	})

	stateData, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	stateFile := statePath(outputDir, st.ChartName)
	tmpStateFile := stateFile + ".tmp"
	if err := os.WriteFile(tmpStateFile, stateData, 0644); err != nil {
		return err
	}

	return os.Rename(tmpStateFile, stateFile)
}

func Edited(outputDir string, base *generator.OutputSet) ([]string, error) {
	var edited []string

	for _, f := range base.Files {
		fullPath := filepath.Join(outputDir, f.RelPath)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				edited = append(edited, f.RelPath)
			} else {
				return nil, err
			}
			continue
		}

		if fileSHA256(data) != fileSHA256(f.Content) {
			edited = append(edited, f.RelPath)
		}
	}

	sort.Strings(edited)
	return edited, nil
}
