// Package value provides intelligent processing of complex data values.
package value

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"unicode/utf8"
)

// DataType represents detected data type.
type DataType string

const (
	DataTypeText       DataType = "text"
	DataTypeJSON       DataType = "json"
	DataTypeXML        DataType = "xml"
	DataTypeBase64     DataType = "base64"
	DataTypeBase64JSON DataType = "base64-json"
	DataTypeBase64XML  DataType = "base64-xml"
	DataTypeBinary     DataType = "binary"
	DataTypeUnknown    DataType = "unknown"
)

// ProcessedValue represents a processed data value.
type ProcessedValue struct {
	// Original is the original value
	Original string

	// DetectedType is the detected data type
	DetectedType DataType

	// Size is the size in bytes
	Size int

	// ShouldExternalize indicates if this should be extracted to external file
	ShouldExternalize bool

	// ExternalPath is suggested path for external file (if ShouldExternalize=true)
	ExternalPath string

	// FormattedValue is the formatted/pretty-printed value
	FormattedValue string

	// Checksum is SHA256 checksum
	Checksum string

	// Metadata contains additional information
	Metadata map[string]string
}

// Processor processes complex data values.
type Processor struct {
	// SizeThreshold is the size threshold for externalization (bytes)
	SizeThreshold int

	// PrettyPrint enables pretty-printing for structured data
	PrettyPrint bool

	// DecodeBase64 enables automatic base64 decoding
	DecodeBase64 bool
}

// DefaultProcessor returns processor with default settings.
func DefaultProcessor() *Processor {
	return &Processor{
		SizeThreshold: 1024, // 1KB
		PrettyPrint:   true,
		DecodeBase64:  true,
	}
}

// Process analyzes and processes a data value.
func (p *Processor) Process(key, value string) *ProcessedValue {
	result := &ProcessedValue{
		Original: value,
		Size:     len(value),
		Metadata: make(map[string]string),
	}

	// Calculate checksum
	hash := sha256.Sum256([]byte(value))
	result.Checksum = fmt.Sprintf("%x", hash[:8]) // First 8 bytes

	// Detect type
	result.DetectedType = p.detectType(value)
	result.Metadata["key"] = key

	// Format based on type
	formatted, meta := p.formatValue(value, result.DetectedType)
	result.FormattedValue = formatted
	for k, v := range meta {
		result.Metadata[k] = v
	}

	// Determine if should externalize
	result.ShouldExternalize = p.shouldExternalize(result)
	if result.ShouldExternalize {
		result.ExternalPath = p.suggestExternalPath(key, result.DetectedType)
	}

	return result
}

// detectType detects the data type of a value.
func (p *Processor) detectType(value string) DataType {
	if value == "" {
		return DataTypeText
	}

	// Check if valid UTF-8
	if !utf8.ValidString(value) {
		return DataTypeBinary
	}

	// Try base64 detection
	if p.DecodeBase64 && p.looksLikeBase64(value) {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err == nil {
			// Check what's inside base64
			decodedStr := string(decoded)
			if p.looksLikeJSON(decodedStr) {
				return DataTypeBase64JSON
			}
			if p.looksLikeXML(decodedStr) {
				return DataTypeBase64XML
			}
			if utf8.ValidString(decodedStr) {
				return DataTypeBase64
			}
		}
	}

	// Try JSON detection
	if p.looksLikeJSON(value) {
		return DataTypeJSON
	}

	// Try XML detection
	if p.looksLikeXML(value) {
		return DataTypeXML
	}

	// Default to text
	return DataTypeText
}

// looksLikeBase64 checks if string looks like base64.
func (p *Processor) looksLikeBase64(s string) bool {
	// Base64 should be at least somewhat long
	if len(s) < 16 {
		return false
	}

	// Check character set (base64 uses A-Za-z0-9+/=)
	base64Chars := 0
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
			base64Chars++
		}
	}

	// If >95% of characters are base64, likely base64
	return float64(base64Chars)/float64(len(s)) > 0.95
}

// looksLikeJSON checks if string looks like JSON.
func (p *Processor) looksLikeJSON(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return false
	}

	// Quick check: JSON objects start with { or [
	if (s[0] == '{' || s[0] == '[') && json.Valid([]byte(s)) {
		return true
	}

	return false
}

// looksLikeXML checks if string looks like XML.
func (p *Processor) looksLikeXML(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return false
	}

	// Quick check: XML starts with <
	if s[0] != '<' {
		return false
	}

	// Try to parse as XML
	var tmp interface{}
	return xml.Unmarshal([]byte(s), &tmp) == nil
}

// formatValue formats value based on detected type.
func (p *Processor) formatValue(value string, dataType DataType) (string, map[string]string) {
	meta := make(map[string]string)

	switch dataType {
	case DataTypeJSON:
		if p.PrettyPrint {
			formatted, err := p.prettyJSON(value)
			if err == nil {
				return formatted, meta
			}
		}
		return value, meta

	case DataTypeXML:
		if p.PrettyPrint {
			formatted, err := p.prettyXML(value)
			if err == nil && formatted != "" {
				return formatted, meta
			}
		}
		return value, meta

	case DataTypeBase64:
		if p.DecodeBase64 {
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err == nil {
				meta["encoding"] = "base64"
				meta["decoded_size"] = fmt.Sprintf("%d", len(decoded))
				// Return decoded if it's printable text
				if utf8.ValidString(string(decoded)) {
					return string(decoded), meta
				}
			}
		}
		return value, meta

	case DataTypeBase64JSON:
		if p.DecodeBase64 {
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err == nil {
				meta["encoding"] = "base64"
				meta["content_type"] = "json"
				if p.PrettyPrint {
					formatted, err := p.prettyJSON(string(decoded))
					if err == nil {
						return formatted, meta
					}
				}
				return string(decoded), meta
			}
		}
		return value, meta

	case DataTypeBase64XML:
		if p.DecodeBase64 {
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err == nil {
				meta["encoding"] = "base64"
				meta["content_type"] = "xml"
				if p.PrettyPrint {
					formatted, err := p.prettyXML(string(decoded))
					if err == nil {
						return formatted, meta
					}
				}
				return string(decoded), meta
			}
		}
		return value, meta

	case DataTypeBinary:
		meta["type"] = "binary"
		meta["size"] = fmt.Sprintf("%d", len(value))
		return value, meta

	default:
		return value, meta
	}
}

// prettyJSON formats JSON with indentation.
func (p *Processor) prettyJSON(s string) (string, error) {
	var obj interface{}
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return "", err
	}

	formatted, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return "", err
	}

	return string(formatted), nil
}

// prettyXML formats XML with indentation.
func (p *Processor) prettyXML(s string) (string, error) {
	var obj interface{}
	if err := xml.Unmarshal([]byte(s), &obj); err != nil {
		return "", err
	}

	formatted, err := xml.MarshalIndent(obj, "", "  ")
	if err != nil {
		return "", err
	}

	return string(formatted), nil
}

// shouldExternalize determines if value should be extracted to external file.
func (p *Processor) shouldExternalize(pv *ProcessedValue) bool {
	// Externalize if exceeds threshold
	if pv.Size > p.SizeThreshold {
		return true
	}

	// Externalize large structured data even if under threshold
	if (pv.DetectedType == DataTypeJSON || pv.DetectedType == DataTypeXML ||
		pv.DetectedType == DataTypeBase64JSON || pv.DetectedType == DataTypeBase64XML) &&
		pv.Size > 512 {
		return true
	}

	// Externalize binary data
	if pv.DetectedType == DataTypeBinary {
		return true
	}

	return false
}

// suggestExternalPath suggests a path for external file.
func (p *Processor) suggestExternalPath(key string, dataType DataType) string {
	// Sanitize key for filename
	filename := strings.ReplaceAll(key, "/", "_")
	filename = strings.ReplaceAll(filename, ".", "_")
	filename = strings.ToLower(filename)

	// Add extension based on type
	ext := p.getExtension(dataType)
	if ext != "" {
		filename = filename + "." + ext
	}

	return "files/" + filename
}

// getExtension returns file extension for data type.
func (p *Processor) getExtension(dataType DataType) string {
	switch dataType {
	case DataTypeJSON, DataTypeBase64JSON:
		return "json"
	case DataTypeXML, DataTypeBase64XML:
		return "xml"
	case DataTypeBase64:
		return "txt"
	case DataTypeBinary:
		return "bin"
	default:
		return "txt"
	}
}

// ProcessBatch processes multiple key-value pairs.
func (p *Processor) ProcessBatch(data map[string]string) map[string]*ProcessedValue {
	results := make(map[string]*ProcessedValue)
	for key, value := range data {
		results[key] = p.Process(key, value)
	}
	return results
}
