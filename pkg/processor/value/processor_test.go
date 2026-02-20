package value

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDetectType(t *testing.T) {
	p := DefaultProcessor()

	tests := []struct {
		name     string
		value    string
		expected DataType
	}{
		{
			name:     "Simple text",
			value:    "hello world",
			expected: DataTypeText,
		},
		{
			name:     "JSON object",
			value:    `{"key": "value", "number": 123}`,
			expected: DataTypeJSON,
		},
		{
			name:     "JSON array",
			value:    `["item1", "item2", "item3"]`,
			expected: DataTypeJSON,
		},
		{
			name:     "XML document",
			value:    `<?xml version="1.0"?><root><item>value</item></root>`,
			expected: DataTypeXML,
		},
		{
			name:     "Base64 encoded text",
			value:    base64.StdEncoding.EncodeToString([]byte("hello world")),
			expected: DataTypeBase64,
		},
		{
			name:     "Base64 encoded JSON",
			value:    base64.StdEncoding.EncodeToString([]byte(`{"key": "value"}`)),
			expected: DataTypeBase64JSON,
		},
		{
			name:     "Base64 encoded XML",
			value:    base64.StdEncoding.EncodeToString([]byte(`<root><item>value</item></root>`)),
			expected: DataTypeBase64XML,
		},
		{
			name:     "Empty string",
			value:    "",
			expected: DataTypeText,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := p.detectType(tt.value)
			if detected != tt.expected {
				t.Errorf("detectType() = %v, want %v", detected, tt.expected)
			}
		})
	}
}

func TestProcess(t *testing.T) {
	p := DefaultProcessor()
	p.SizeThreshold = 100 // Low threshold for testing

	tests := []struct {
		name                 string
		key                  string
		value                string
		expectedType         DataType
		expectExternalize    bool
		expectedExternalPath string
	}{
		{
			name:              "Small text",
			key:               "config.txt",
			value:             "small text",
			expectedType:      DataTypeText,
			expectExternalize: false,
		},
		{
			name:                 "Large text",
			key:                  "large.txt",
			value:                string(make([]byte, 200)), // 200 bytes
			expectedType:         DataTypeText,
			expectExternalize:    true,
			expectedExternalPath: "files/large_txt.txt",
		},
		{
			name: "Large JSON",
			key:  "config.json",
			value: func() string {
				// Create valid JSON with long value
				longValue := ""
				for i := 0; i < 150; i++ {
					longValue += "x"
				}
				return `{"key": "` + longValue + `"}`
			}(),
			expectedType:         DataTypeJSON,
			expectExternalize:    true,
			expectedExternalPath: "files/config_json.json",
		},
		{
			name:                 "Base64 JSON",
			key:                  "data.base64",
			value:                base64.StdEncoding.EncodeToString([]byte(`{"key": "value"}`)),
			expectedType:         DataTypeBase64JSON,
			expectExternalize:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.Process(tt.key, tt.value)

			if result.DetectedType != tt.expectedType {
				t.Errorf("Process() type = %v, want %v", result.DetectedType, tt.expectedType)
			}

			if result.ShouldExternalize != tt.expectExternalize {
				t.Errorf("Process() externalize = %v, want %v", result.ShouldExternalize, tt.expectExternalize)
			}

			if tt.expectExternalize && result.ExternalPath != tt.expectedExternalPath {
				t.Errorf("Process() externalPath = %v, want %v", result.ExternalPath, tt.expectedExternalPath)
			}

			if result.Checksum == "" {
				t.Error("Process() checksum is empty")
			}
		})
	}
}

func TestPrettyJSON(t *testing.T) {
	p := DefaultProcessor()

	input := `{"key":"value","nested":{"a":1,"b":2},"array":[1,2,3]}`
	expected := `{
  "array": [
    1,
    2,
    3
  ],
  "key": "value",
  "nested": {
    "a": 1,
    "b": 2
  }
}`

	formatted, err := p.prettyJSON(input)
	if err != nil {
		t.Fatalf("prettyJSON() error = %v", err)
	}

	if formatted != expected {
		t.Errorf("prettyJSON() = %v, want %v", formatted, expected)
	}
}

func TestProcessBatch(t *testing.T) {
	p := DefaultProcessor()

	data := map[string]string{
		"text.txt":    "simple text",
		"config.json": `{"key": "value"}`,
		"data.xml":    `<root><item>value</item></root>`,
	}

	results := p.ProcessBatch(data)

	if len(results) != len(data) {
		t.Errorf("ProcessBatch() returned %d results, want %d", len(results), len(data))
	}

	if results["config.json"].DetectedType != DataTypeJSON {
		t.Error("ProcessBatch() failed to detect JSON")
	}

	if results["data.xml"].DetectedType != DataTypeXML {
		t.Error("ProcessBatch() failed to detect XML")
	}
}

func TestExternalFileManager(t *testing.T) {
	manager := NewExternalFileManager()

	file1 := &ExternalFile{
		Path:           "files/config.json",
		Content:        `{"key": "value"}`,
		SourceKey:      "config.json",
		SourceResource: "my-configmap",
		DataType:       DataTypeJSON,
		Checksum:       "abc123",
	}

	// Add file
	err := manager.Add(file1)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Try to add duplicate with same content
	err = manager.Add(file1)
	if err != nil {
		t.Errorf("Add() duplicate with same content should not error, got %v", err)
	}

	// Try to add duplicate with different content
	file2 := &ExternalFile{
		Path:           "files/config.json",
		Content:        `{"key": "different"}`,
		SourceKey:      "config.json",
		SourceResource: "my-configmap",
		DataType:       DataTypeJSON,
		Checksum:       "def456",
	}

	err = manager.Add(file2)
	if err == nil {
		t.Error("Add() duplicate with different content should error")
	}

	// Get files
	files := manager.GetFiles()
	if len(files) != 1 {
		t.Errorf("GetFiles() returned %d files, want 1", len(files))
	}

	// Test Helm reference
	ref := manager.GetHelmReference("files/config.json")
	expected := `{{ .Files.Get "files/config.json" }}`
	if ref != expected {
		t.Errorf("GetHelmReference() = %v, want %v", ref, expected)
	}
}

func TestSuggestExternalPath(t *testing.T) {
	p := DefaultProcessor()

	tests := []struct {
		name     string
		key      string
		dataType DataType
		expected string
	}{
		{
			name:     "JSON file",
			key:      "config.json",
			dataType: DataTypeJSON,
			expected: "files/config_json.json",
		},
		{
			name:     "XML file",
			key:      "data.xml",
			dataType: DataTypeXML,
			expected: "files/data_xml.xml",
		},
		{
			name:     "Nested key",
			key:      "app/config/settings.json",
			dataType: DataTypeJSON,
			expected: "files/app_config_settings_json.json",
		},
		{
			name:     "Binary data",
			key:      "cert.bin",
			dataType: DataTypeBinary,
			expected: "files/cert_bin.bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := p.suggestExternalPath(tt.key, tt.dataType)
			if path != tt.expected {
				t.Errorf("suggestExternalPath() = %v, want %v", path, tt.expected)
			}
		})
	}
}

// ── AddFromProcessed ─────────────────────────────────────────────────────────

func TestExternalFileManager_AddFromProcessed(t *testing.T) {
	t.Run("ShouldExternalize true creates file", func(t *testing.T) {
		manager := NewExternalFileManager()
		pv := &ProcessedValue{
			ShouldExternalize: true,
			ExternalPath:      "files/config_json.json",
			FormattedValue:    `{"key": "value"}`,
			DetectedType:      DataTypeJSON,
			Checksum:          "deadbeef",
		}

		file, err := manager.AddFromProcessed("my-configmap", "config.json", pv)
		if err != nil {
			t.Fatalf("AddFromProcessed() unexpected error: %v", err)
		}
		if file == nil {
			t.Fatal("AddFromProcessed() returned nil file")
		}
		if file.Path != pv.ExternalPath {
			t.Errorf("file.Path = %q, want %q", file.Path, pv.ExternalPath)
		}
		if file.Content != pv.FormattedValue {
			t.Errorf("file.Content = %q, want %q", file.Content, pv.FormattedValue)
		}
		if file.SourceKey != "config.json" {
			t.Errorf("file.SourceKey = %q, want %q", file.SourceKey, "config.json")
		}
		if file.SourceResource != "my-configmap" {
			t.Errorf("file.SourceResource = %q, want %q", file.SourceResource, "my-configmap")
		}
		if file.DataType != DataTypeJSON {
			t.Errorf("file.DataType = %v, want %v", file.DataType, DataTypeJSON)
		}
		if file.Checksum != pv.Checksum {
			t.Errorf("file.Checksum = %q, want %q", file.Checksum, pv.Checksum)
		}

		// Verify the file was actually registered in the manager.
		stored := manager.GetFiles()
		if len(stored) != 1 {
			t.Errorf("GetFiles() count = %d, want 1", len(stored))
		}
	})

	t.Run("ShouldExternalize false returns error", func(t *testing.T) {
		manager := NewExternalFileManager()
		pv := &ProcessedValue{
			ShouldExternalize: false,
			ExternalPath:      "",
			FormattedValue:    "small text",
			DetectedType:      DataTypeText,
			Checksum:          "aabbcc",
		}

		file, err := manager.AddFromProcessed("my-configmap", "note.txt", pv)
		if err == nil {
			t.Fatal("AddFromProcessed() expected error when ShouldExternalize=false, got nil")
		}
		if file != nil {
			t.Errorf("AddFromProcessed() expected nil file on error, got %+v", file)
		}
	})

	t.Run("Path conflict propagated as error", func(t *testing.T) {
		manager := NewExternalFileManager()

		// Add the first file directly to pre-populate the manager.
		existing := &ExternalFile{
			Path:     "files/config_json.json",
			Content:  `{"key": "original"}`,
			Checksum: "checksum-original",
		}
		if err := manager.Add(existing); err != nil {
			t.Fatalf("Add() pre-population error: %v", err)
		}

		// Now try AddFromProcessed with the same path but different checksum.
		pv := &ProcessedValue{
			ShouldExternalize: true,
			ExternalPath:      "files/config_json.json",
			FormattedValue:    `{"key": "conflicting"}`,
			DetectedType:      DataTypeJSON,
			Checksum:          "checksum-conflict",
		}

		file, err := manager.AddFromProcessed("other-configmap", "config.json", pv)
		if err == nil {
			t.Fatal("AddFromProcessed() expected conflict error, got nil")
		}
		if file != nil {
			t.Errorf("AddFromProcessed() expected nil file on conflict, got %+v", file)
		}
	})
}

// ── GetHelmReferenceWithFallback ─────────────────────────────────────────────

func TestExternalFileManager_GetHelmReferenceWithFallback(t *testing.T) {
	manager := NewExternalFileManager()

	tests := []struct {
		name     string
		path     string
		fallback string
		expected string
	}{
		{
			name:     "Plain fallback",
			path:     "files/config.json",
			fallback: "default-value",
			expected: `{{ .Files.Get "files/config.json" | default "default-value" }}`,
		},
		{
			name:     "Fallback with double quotes escaped",
			path:     "files/data.json",
			fallback: `{"key": "val"}`,
			expected: `{{ .Files.Get "files/data.json" | default "{\"key\": \"val\"}" }}`,
		},
		{
			name:     "Empty fallback",
			path:     "files/empty.txt",
			fallback: "",
			expected: `{{ .Files.Get "files/empty.txt" | default "" }}`,
		},
		{
			name:     "Fallback with multiple special chars",
			path:     "files/cert.pem",
			fallback: `"BEGIN" "END"`,
			expected: `{{ .Files.Get "files/cert.pem" | default "\"BEGIN\" \"END\"" }}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.GetHelmReferenceWithFallback(tt.path, tt.fallback)
			if result != tt.expected {
				t.Errorf("GetHelmReferenceWithFallback() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// ── GenerateHelmHelper ───────────────────────────────────────────────────────

func TestExternalFileManager_GenerateHelmHelper(t *testing.T) {
	manager := NewExternalFileManager()

	tests := []struct {
		name      string
		chartName string
	}{
		{name: "Simple chart name", chartName: "my-chart"},
		{name: "Nested chart name", chartName: "deckhouse"},
		{name: "Chart with dots", chartName: "app.v2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := manager.GenerateHelmHelper(tt.chartName)

			if output == "" {
				t.Fatal("GenerateHelmHelper() returned empty string")
			}

			// The chart name must appear as the define prefix for both helpers.
			getFileDefine := `define "` + tt.chartName + `.getFile"`
			getFileBase64Define := `define "` + tt.chartName + `.getFileBase64"`

			if !strings.Contains(output, getFileDefine) {
				t.Errorf("GenerateHelmHelper() missing define %q in output:\n%s", getFileDefine, output)
			}
			if !strings.Contains(output, getFileBase64Define) {
				t.Errorf("GenerateHelmHelper() missing define %q in output:\n%s", getFileBase64Define, output)
			}

			// Usage comments must also mention the chart name.
			usageInclude := `include "` + tt.chartName + `.getFile"`
			if !strings.Contains(output, usageInclude) {
				t.Errorf("GenerateHelmHelper() missing usage comment %q in output:\n%s", usageInclude, output)
			}

			// Both helpers must have the {{- end -}} terminator.
			if strings.Count(output, "{{- end -}}") < 2 {
				t.Errorf("GenerateHelmHelper() expected at least 2 '{{- end -}}' blocks, got %d",
					strings.Count(output, "{{- end -}}"))
			}
		})
	}
}

// ── ToValuesReference ────────────────────────────────────────────────────────

func TestExternalFileManager_ToValuesReference(t *testing.T) {
	manager := NewExternalFileManager()

	file := &ExternalFile{
		Path:     "files/config.json",
		DataType: DataTypeJSON,
		Checksum: "sha256abc",
	}

	ref := manager.ToValuesReference(file)

	if ref == nil {
		t.Fatal("ToValuesReference() returned nil")
	}

	outer, ok := ref["externalFile"]
	if !ok {
		t.Fatal("ToValuesReference() missing top-level key 'externalFile'")
	}

	inner, ok := outer.(map[string]interface{})
	if !ok {
		t.Fatalf("ToValuesReference()['externalFile'] is not a map, got %T", outer)
	}

	if enabled, ok := inner["enabled"]; !ok || enabled != true {
		t.Errorf("ToValuesReference() inner['enabled'] = %v, want true", inner["enabled"])
	}
	if path, ok := inner["path"]; !ok || path != file.Path {
		t.Errorf("ToValuesReference() inner['path'] = %v, want %q", inner["path"], file.Path)
	}
	if checksum, ok := inner["checksum"]; !ok || checksum != file.Checksum {
		t.Errorf("ToValuesReference() inner['checksum'] = %v, want %q", inner["checksum"], file.Checksum)
	}
	if dtype, ok := inner["type"]; !ok || dtype != string(file.DataType) {
		t.Errorf("ToValuesReference() inner['type'] = %v, want %q", inner["type"], string(file.DataType))
	}
}

// ── SuggestValuesStructure ───────────────────────────────────────────────────

func TestExternalFileManager_SuggestValuesStructure(t *testing.T) {
	t.Run("Empty manager returns nil", func(t *testing.T) {
		manager := NewExternalFileManager()
		result := manager.SuggestValuesStructure()
		if result != nil {
			t.Errorf("SuggestValuesStructure() on empty manager = %v, want nil", result)
		}
	})

	t.Run("Single file structure", func(t *testing.T) {
		manager := NewExternalFileManager()
		if err := manager.Add(&ExternalFile{
			Path:           "files/config.json",
			Content:        `{"x":1}`,
			SourceKey:      "config.json",
			SourceResource: "my-configmap",
			DataType:       DataTypeJSON,
			Checksum:       "csum1",
		}); err != nil {
			t.Fatalf("Add() error: %v", err)
		}

		result := manager.SuggestValuesStructure()
		if result == nil {
			t.Fatal("SuggestValuesStructure() returned nil for non-empty manager")
		}

		outerRaw, ok := result["externalFiles"]
		if !ok {
			t.Fatal("SuggestValuesStructure() missing top-level key 'externalFiles'")
		}
		outer, ok := outerRaw.(map[string]interface{})
		if !ok {
			t.Fatalf("SuggestValuesStructure()['externalFiles'] is not a map, got %T", outerRaw)
		}

		if enabled, ok := outer["enabled"]; !ok || enabled != true {
			t.Errorf("SuggestValuesStructure() ['enabled'] = %v, want true", outer["enabled"])
		}

		filesRaw, ok := outer["files"]
		if !ok {
			t.Fatal("SuggestValuesStructure() missing 'files' key inside 'externalFiles'")
		}
		files, ok := filesRaw.(map[string]interface{})
		if !ok {
			t.Fatalf("SuggestValuesStructure()['files'] is not a map, got %T", filesRaw)
		}
		if len(files) != 1 {
			t.Errorf("SuggestValuesStructure() files count = %d, want 1", len(files))
		}

		// The key in the nested map is the base filename.
		entry, ok := files["config.json"]
		if !ok {
			t.Fatal("SuggestValuesStructure() missing entry for 'config.json'")
		}
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			t.Fatalf("SuggestValuesStructure() entry is not a map, got %T", entry)
		}
		if entryMap["path"] != "files/config.json" {
			t.Errorf("entry['path'] = %v, want %q", entryMap["path"], "files/config.json")
		}
		if entryMap["source"] != "my-configmap" {
			t.Errorf("entry['source'] = %v, want %q", entryMap["source"], "my-configmap")
		}
		if entryMap["key"] != "config.json" {
			t.Errorf("entry['key'] = %v, want %q", entryMap["key"], "config.json")
		}
		if entryMap["type"] != string(DataTypeJSON) {
			t.Errorf("entry['type'] = %v, want %q", entryMap["type"], string(DataTypeJSON))
		}
		if entryMap["checksum"] != "csum1" {
			t.Errorf("entry['checksum'] = %v, want %q", entryMap["checksum"], "csum1")
		}
	})

	t.Run("Multiple files all present in structure", func(t *testing.T) {
		manager := NewExternalFileManager()
		for _, f := range []*ExternalFile{
			{Path: "files/alpha.json", SourceKey: "alpha.json", SourceResource: "cm1", DataType: DataTypeJSON, Checksum: "c1"},
			{Path: "files/beta.xml", SourceKey: "beta.xml", SourceResource: "cm2", DataType: DataTypeXML, Checksum: "c2"},
			{Path: "files/gamma.txt", SourceKey: "gamma.txt", SourceResource: "cm3", DataType: DataTypeText, Checksum: "c3"},
		} {
			if err := manager.Add(f); err != nil {
				t.Fatalf("Add() error: %v", err)
			}
		}

		result := manager.SuggestValuesStructure()
		if result == nil {
			t.Fatal("SuggestValuesStructure() returned nil")
		}

		outer := result["externalFiles"].(map[string]interface{})
		files := outer["files"].(map[string]interface{})
		if len(files) != 3 {
			t.Errorf("SuggestValuesStructure() files count = %d, want 3", len(files))
		}

		for _, name := range []string{"alpha.json", "beta.xml", "gamma.txt"} {
			if _, ok := files[name]; !ok {
				t.Errorf("SuggestValuesStructure() missing entry for %q", name)
			}
		}
	})
}

// ── GenerateTemplateAnnotation ───────────────────────────────────────────────

func TestExternalFileManager_GenerateTemplateAnnotation(t *testing.T) {
	manager := NewExternalFileManager()

	t.Run("Empty slice returns empty string", func(t *testing.T) {
		result := manager.GenerateTemplateAnnotation([]*ExternalFile{})
		if result != "" {
			t.Errorf("GenerateTemplateAnnotation([]) = %q, want empty string", result)
		}
	})

	t.Run("Nil slice returns empty string", func(t *testing.T) {
		result := manager.GenerateTemplateAnnotation(nil)
		if result != "" {
			t.Errorf("GenerateTemplateAnnotation(nil) = %q, want empty string", result)
		}
	})

	t.Run("Single file produces path:checksum", func(t *testing.T) {
		files := []*ExternalFile{
			{Path: "files/config.json", Checksum: "abc123"},
		}
		result := manager.GenerateTemplateAnnotation(files)
		expected := "files/config.json:abc123"
		if result != expected {
			t.Errorf("GenerateTemplateAnnotation() = %q, want %q", result, expected)
		}
	})

	t.Run("Multiple files joined with comma", func(t *testing.T) {
		files := []*ExternalFile{
			{Path: "files/a.json", Checksum: "hash1"},
			{Path: "files/b.xml", Checksum: "hash2"},
			{Path: "files/c.txt", Checksum: "hash3"},
		}
		result := manager.GenerateTemplateAnnotation(files)

		// The function joins with "," so verify the count of separators and that
		// every entry appears in the result.
		parts := strings.Split(result, ",")
		if len(parts) != 3 {
			t.Errorf("GenerateTemplateAnnotation() produced %d parts, want 3; got %q", len(parts), result)
		}
		for _, f := range files {
			want := f.Path + ":" + f.Checksum
			if !strings.Contains(result, want) {
				t.Errorf("GenerateTemplateAnnotation() missing %q in result %q", want, result)
			}
		}
	})
}

// ── formatValue / Process branch coverage ────────────────────────────────────

func TestProcess_XMLFormatting(t *testing.T) {
	p := DefaultProcessor()
	// XML value: keep it small — we only care about type detection and
	// FormattedValue fallback behaviour (the XML branch guards formatted != "").
	input := `<root><item>value</item></root>`
	result := p.Process("data.xml", input)

	if result.DetectedType != DataTypeXML {
		t.Errorf("Process() type = %v, want %v", result.DetectedType, DataTypeXML)
	}
	// The XML branch falls back to the original value when prettyXML returns "".
	// Either way FormattedValue must be non-empty (it is at least the original).
	if result.FormattedValue == "" {
		t.Error("Process() FormattedValue is empty for XML input")
	}
}

func TestProcess_Base64TextDecoded(t *testing.T) {
	p := DefaultProcessor()
	// Base64-encoded plain text: FormattedValue should be the decoded string,
	// and Metadata must carry encoding info.
	original := "this is plain text padded to exceed sixteen chars"
	encoded := base64.StdEncoding.EncodeToString([]byte(original))

	result := p.Process("note.txt", encoded)

	if result.DetectedType != DataTypeBase64 {
		t.Errorf("Process() type = %v, want %v", result.DetectedType, DataTypeBase64)
	}
	if result.FormattedValue != original {
		t.Errorf("Process() FormattedValue = %q, want %q", result.FormattedValue, original)
	}
	if result.Metadata["encoding"] != "base64" {
		t.Errorf("Process() Metadata['encoding'] = %q, want %q", result.Metadata["encoding"], "base64")
	}
	if result.Metadata["decoded_size"] == "" {
		t.Error("Process() Metadata['decoded_size'] is empty")
	}
}

func TestProcess_Base64XMLDecoded(t *testing.T) {
	p := DefaultProcessor()
	xmlContent := `<root><item>value</item></root>`
	encoded := base64.StdEncoding.EncodeToString([]byte(xmlContent))

	result := p.Process("data.xml.b64", encoded)

	// The detected type must be DataTypeBase64XML regardless of formatting.
	if result.DetectedType != DataTypeBase64XML {
		t.Errorf("Process() type = %v, want %v", result.DetectedType, DataTypeBase64XML)
	}
	// Metadata is always populated for Base64XML even when prettyXML returns "".
	if result.Metadata["encoding"] != "base64" {
		t.Errorf("Process() Metadata['encoding'] = %q, want %q", result.Metadata["encoding"], "base64")
	}
	if result.Metadata["content_type"] != "xml" {
		t.Errorf("Process() Metadata['content_type'] = %q, want %q", result.Metadata["content_type"], "xml")
	}
	// Checksum must always be computed.
	if result.Checksum == "" {
		t.Error("Process() Checksum is empty for Base64XML input")
	}
}

func TestProcess_BinaryMetadata(t *testing.T) {
	p := DefaultProcessor()
	// Construct a non-UTF-8 byte sequence that is NOT valid base64.
	// \x80-\x8f are continuation bytes that are invalid as leading UTF-8 bytes.
	binaryData := "\x80\x81\x82\x83\x84\x85\x86\x87\x88\x89\x8a\x8b\x8c\x8d\x8e\x8f"

	result := p.Process("blob.bin", binaryData)

	if result.DetectedType != DataTypeBinary {
		t.Errorf("Process() type = %v, want %v", result.DetectedType, DataTypeBinary)
	}
	if result.Metadata["type"] != "binary" {
		t.Errorf("Process() Metadata['type'] = %q, want %q", result.Metadata["type"], "binary")
	}
	if result.Metadata["size"] == "" {
		t.Error("Process() Metadata['size'] is empty for binary input")
	}
	// Binary data is always externalized regardless of size threshold.
	if !result.ShouldExternalize {
		t.Error("Process() binary data should always be externalized")
	}
}

// ── prettyJSON error path ─────────────────────────────────────────────────────

func TestPrettyJSON_InvalidInput(t *testing.T) {
	p := DefaultProcessor()

	_, err := p.prettyJSON("this is not json {{{")
	if err == nil {
		t.Error("prettyJSON() expected error for invalid JSON, got nil")
	}
}

// ── shouldExternalize: large structured data (>512 bytes) under SizeThreshold ──

func TestShouldExternalize_LargeStructuredUnderSizeThreshold(t *testing.T) {
	// SizeThreshold is set very high so the size-only check does NOT trigger.
	// The structured-data branch (>512 bytes) must still force externalization.
	p := DefaultProcessor()
	p.SizeThreshold = 100_000 // 100 KB — well above the test payload

	// Build a JSON value that is > 512 bytes but < 100 KB.
	var sb strings.Builder
	sb.WriteString(`{"data":"`)
	for i := 0; i < 520; i++ {
		sb.WriteByte('x')
	}
	sb.WriteString(`"}`)
	largeJSON := sb.String()

	result := p.Process("config.json", largeJSON)

	if result.DetectedType != DataTypeJSON {
		t.Fatalf("expected DataTypeJSON, got %v", result.DetectedType)
	}
	if len(result.Original) <= 512 {
		t.Fatalf("test setup error: payload is only %d bytes, need >512", len(result.Original))
	}
	if !result.ShouldExternalize {
		t.Error("shouldExternalize() should return true for JSON > 512 bytes even under SizeThreshold")
	}
}

// ── getExtension: DataTypeBase64 → "txt" ─────────────────────────────────────

func TestSuggestExternalPath_Base64Extension(t *testing.T) {
	p := DefaultProcessor()
	// DataTypeBase64 must produce a .txt extension.
	path := p.suggestExternalPath("secret.b64", DataTypeBase64)
	expected := "files/secret_b64.txt"
	if path != expected {
		t.Errorf("suggestExternalPath() = %q, want %q", path, expected)
	}
}

// ── detectType: binary (non-UTF-8 input) ─────────────────────────────────────

func TestDetectType_Binary(t *testing.T) {
	p := DefaultProcessor()
	// Raw bytes that are not valid UTF-8 and not valid base64.
	binaryValue := "\x80\x81\x82\x83\x84\x85\x86\x87"
	detected := p.detectType(binaryValue)
	if detected != DataTypeBinary {
		t.Errorf("detectType() = %v, want %v", detected, DataTypeBinary)
	}
}

// ── looksLikeBase64: short string boundary (< 16 chars) ─────────────────────

func TestDetectType_ShortBase64LikeStringIsText(t *testing.T) {
	p := DefaultProcessor()
	// "SGVsbG8=" is base64 for "Hello" — only 8 chars, below the 16-char
	// minimum that looksLikeBase64 requires.  detectType must NOT classify it
	// as any base64 variant.
	short := "SGVsbG8="
	detected := p.detectType(short)
	if detected == DataTypeBase64 || detected == DataTypeBase64JSON || detected == DataTypeBase64XML {
		t.Errorf("detectType() = %v for short string %q, should not detect as base64 (< 16 chars)", detected, short)
	}
}
