package value

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ExternalFile represents a file to be extracted from chart.
type ExternalFile struct {
	// Path is relative path in chart (e.g., "files/config.json")
	Path string

	// Content is the file content
	Content string

	// SourceKey is the original key in ConfigMap/Secret
	SourceKey string

	// SourceResource identifies the source resource
	SourceResource string

	// DataType is the detected data type
	DataType DataType

	// Checksum is content checksum
	Checksum string
}

// ExternalFileManager manages external files for chart generation.
type ExternalFileManager struct {
	files map[string]*ExternalFile
}

// NewExternalFileManager creates a new external file manager.
func NewExternalFileManager() *ExternalFileManager {
	return &ExternalFileManager{
		files: make(map[string]*ExternalFile),
	}
}

// Add adds an external file.
func (m *ExternalFileManager) Add(file *ExternalFile) error {
	// Check for path conflicts
	if existing, exists := m.files[file.Path]; exists {
		if existing.Checksum != file.Checksum {
			return fmt.Errorf("path conflict: %s already exists with different content", file.Path)
		}
		// Same content, ignore duplicate
		return nil
	}

	m.files[file.Path] = file
	return nil
}

// AddFromProcessed creates external file from processed value.
func (m *ExternalFileManager) AddFromProcessed(
	sourceResource, sourceKey string,
	pv *ProcessedValue,
) (*ExternalFile, error) {
	if !pv.ShouldExternalize {
		return nil, fmt.Errorf("value should not be externalized")
	}

	file := &ExternalFile{
		Path:           pv.ExternalPath,
		Content:        pv.FormattedValue,
		SourceKey:      sourceKey,
		SourceResource: sourceResource,
		DataType:       pv.DetectedType,
		Checksum:       pv.Checksum,
	}

	if err := m.Add(file); err != nil {
		return nil, err
	}

	return file, nil
}

// GetFiles returns all registered external files.
func (m *ExternalFileManager) GetFiles() []*ExternalFile {
	files := make([]*ExternalFile, 0, len(m.files))
	for _, f := range m.files {
		files = append(files, f)
	}
	return files
}

// GetHelmReference returns Helm template reference for external file.
func (m *ExternalFileManager) GetHelmReference(path string) string {
	// Convert path to Helm .Files reference
	// files/config.json -> {{ .Files.Get "files/config.json" }}
	return fmt.Sprintf(`{{ .Files.Get "%s" }}`, path)
}

// GetHelmReferenceWithFallback returns reference with fallback to inline value.
func (m *ExternalFileManager) GetHelmReferenceWithFallback(path, fallback string) string {
	// {{ .Files.Get "files/config.json" | default "fallback" }}
	// Escape quotes in fallback
	escaped := strings.ReplaceAll(fallback, `"`, `\"`)
	return fmt.Sprintf(`{{ .Files.Get "%s" | default "%s" }}`, path, escaped)
}

// GenerateHelmHelper generates _helpers.tpl function for file access.
func (m *ExternalFileManager) GenerateHelmHelper(chartName string) string {
	return fmt.Sprintf(`{{/*
Get file content with fallback
Usage: {{ include "%s.getFile" (dict "Files" .Files "path" "files/config.json" "default" "fallback") }}
*/}}
{{- define "%s.getFile" -}}
{{- $path := .path -}}
{{- $default := .default | default "" -}}
{{- .Files.Get $path | default $default -}}
{{- end -}}

{{/*
Get file content as base64
Usage: {{ include "%s.getFileBase64" (dict "Files" .Files "path" "files/data.bin") }}
*/}}
{{- define "%s.getFileBase64" -}}
{{- $path := .path -}}
{{- .Files.Get $path | b64enc -}}
{{- end -}}
`, chartName, chartName, chartName, chartName)
}

// ToValuesReference converts external file to values.yaml reference structure.
func (m *ExternalFileManager) ToValuesReference(file *ExternalFile) map[string]interface{} {
	return map[string]interface{}{
		"externalFile": map[string]interface{}{
			"enabled":  true,
			"path":     file.Path,
			"checksum": file.Checksum,
			"type":     string(file.DataType),
		},
	}
}

// SuggestValuesStructure suggests values.yaml structure for external files.
func (m *ExternalFileManager) SuggestValuesStructure() map[string]interface{} {
	if len(m.files) == 0 {
		return nil
	}

	externalFiles := make(map[string]interface{})
	for path, file := range m.files {
		// Create nested structure: files -> config.json -> metadata
		filename := filepath.Base(path)
		externalFiles[filename] = map[string]interface{}{
			"path":     path,
			"source":   file.SourceResource,
			"key":      file.SourceKey,
			"type":     string(file.DataType),
			"checksum": file.Checksum,
		}
	}

	return map[string]interface{}{
		"externalFiles": map[string]interface{}{
			"enabled": true,
			"files":   externalFiles,
		},
	}
}

// GenerateTemplateAnnotation generates annotation for ConfigMap/Secret template
// to reference external files.
func (m *ExternalFileManager) GenerateTemplateAnnotation(files []*ExternalFile) string {
	if len(files) == 0 {
		return ""
	}

	var checksums []string
	for _, file := range files {
		checksums = append(checksums, fmt.Sprintf("%s:%s", file.Path, file.Checksum))
	}

	return strings.Join(checksums, ",")
}
