package value

import (
	"encoding/base64"
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
		name                  string
		key                   string
		value                 string
		expectedType          DataType
		expectExternalize     bool
		expectedExternalPath  string
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
		"text.txt":   "simple text",
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
