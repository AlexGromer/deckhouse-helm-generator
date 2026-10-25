package connect

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"
)

// ParseConfigFile parses a configuration file based on its extension.
func ParseConfigFile(filename, content string) []KeyValue {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".properties", ".env":
		return parseProperties(content)
	case ".yaml", ".yml":
		return parseYAML(content)
	case ".json":
		return parseJSON(content)
	case ".toml", ".ini", ".cfg", ".conf":
		return parseINI(content)
	default:
		// Attempt to guess based on content or fallback to properties.
		if strings.HasPrefix(strings.TrimSpace(content), "{") {
			return parseJSON(content)
		}
		if strings.HasPrefix(strings.TrimSpace(content), "---") || strings.Contains(content, ": ") {
			return parseYAML(content)
		}
		return parseProperties(content)
	}
}

func parseProperties(content string) []KeyValue {
	var kvs []KeyValue
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	var currentKey, currentValue string
	inContinuation := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inContinuation && (trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!")) {
			continue
		}

		trimmed = strings.TrimPrefix(trimmed, "export ")

		if inContinuation {
			if strings.HasSuffix(trimmed, "\\") {
				currentValue += strings.TrimSuffix(trimmed, "\\")
			} else {
				currentValue += trimmed
				kvs = append(kvs, KeyValue{Key: currentKey, Value: resolveEnvVars(unquote(currentValue))})
				inContinuation = false
			}
			continue
		}

		idxEqual := strings.Index(trimmed, "=")
		idxColon := strings.Index(trimmed, ":")

		idx := -1
		if idxEqual != -1 && idxColon != -1 {
			if idxEqual < idxColon {
				idx = idxEqual
			} else {
				idx = idxColon
			}
		} else if idxEqual != -1 {
			idx = idxEqual
		} else if idxColon != -1 {
			idx = idxColon
		}

		if idx != -1 {
			currentKey = strings.TrimSpace(trimmed[:idx])
			val := strings.TrimSpace(trimmed[idx+1:])

			if strings.HasSuffix(val, "\\") {
				currentValue = strings.TrimSuffix(val, "\\")
				inContinuation = true
			} else {
				kvs = append(kvs, KeyValue{Key: currentKey, Value: resolveEnvVars(unquote(val))})
			}
		}
	}
	return kvs
}

func unquote(s string) string {
	if (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
		if len(s) >= 2 {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// resolveEnvVars resolves ${VAR:default} pattern
func resolveEnvVars(val string) string {
	re := regexp.MustCompile(`\$\{([^:}]+)(?::([^}]+))?\}`)
	return re.ReplaceAllStringFunc(val, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) == 3 {
			// In context of static analysis without live env, we can just return the default if available,
			// or the original placeholder. Since this is static parsing, we keep the placeholder if no default.
			if parts[2] != "" {
				return parts[2]
			}
		}
		return match
	})
}

func parseYAML(content string) []KeyValue {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	// Properly split multi-document yaml ignoring potential trailing spaces
	// Split by \n--- followed by newline or space
	re := regexp.MustCompile(`\n---[ \t]*\n`)
	docs := re.Split("\n"+normalized+"\n", -1)

	var kvs []KeyValue

	for _, doc := range docs {
		if len(bytes.TrimSpace([]byte(doc))) == 0 {
			continue
		}

		var raw interface{}
		if err := sigsyaml.Unmarshal([]byte(doc), &raw); err != nil {
			continue
		}

		flat := flattenJSON("", raw)
		for k, v := range flat {
			kvs = append(kvs, KeyValue{Key: k, Value: resolveEnvVars(v)})
		}
	}

	return kvs
}

func parseJSON(content string) []KeyValue {
	var raw interface{}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil
	}

	flat := flattenJSON("", raw)
	var kvs []KeyValue
	for k, v := range flat {
		kvs = append(kvs, KeyValue{Key: k, Value: resolveEnvVars(v)})
	}

	return kvs
}

func parseINI(content string) []KeyValue {
	var kvs []KeyValue
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	currentSection := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			currentSection = trimmed[1 : len(trimmed)-1]
			continue
		}

		idx := strings.Index(trimmed, "=")
		if idx != -1 {
			k := strings.TrimSpace(trimmed[:idx])
			v := strings.TrimSpace(trimmed[idx+1:])

			fullKey := k
			if currentSection != "" {
				fullKey = currentSection + "." + k
			}

			kvs = append(kvs, KeyValue{Key: fullKey, Value: resolveEnvVars(unquote(v))})
		}
	}

	return kvs
}

func flattenJSON(prefix string, v interface{}) map[string]string {
	result := make(map[string]string)

	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			newPrefix := k
			if prefix != "" {
				newPrefix = prefix + "." + k
			}
			childMap := flattenJSON(newPrefix, child)
			for ck, cv := range childMap {
				result[ck] = cv
			}
		}
	case []interface{}:
		// Arrays are generally skipped in standard properties mapping,
		// but Spring maps them as arr[0], arr[1] or arr[0].key.
		for i, child := range val {
			newPrefix := prefix + "[" + string(rune(i+'0')) + "]"
			childMap := flattenJSON(newPrefix, child)
			for ck, cv := range childMap {
				result[ck] = cv
			}
		}
	case string:
		result[prefix] = val
	case float64:
		// Go unmarshals numbers to float64
		// Since we just need strings, format it
		b, _ := json.Marshal(val)
		result[prefix] = string(b)
	case bool:
		if val {
			result[prefix] = "true"
		} else {
			result[prefix] = "false"
		}
	case nil:
		result[prefix] = ""
	}

	return result
}

// JavaOpts parses -Dkey=value from Java arguments.
func JavaOpts(raw string) []KeyValue {
	var kvs []KeyValue
	parts := strings.Fields(raw)

	for _, p := range parts {
		if strings.HasPrefix(p, "-D") {
			p = strings.TrimPrefix(p, "-D")
			idx := strings.Index(p, "=")
			if idx != -1 {
				k := p[:idx]
				v := p[idx+1:]
				kvs = append(kvs, KeyValue{Key: k, Value: resolveEnvVars(unquote(v))})
			} else {
				kvs = append(kvs, KeyValue{Key: p, Value: "true"})
			}
		}
	}

	return kvs
}

// CommandLineProperties parses --key=value or --spring.application.json=...
func CommandLineProperties(args []string) []KeyValue {
	var kvs []KeyValue

	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			arg = strings.TrimPrefix(arg, "--")
			idx := strings.Index(arg, "=")
			if idx != -1 {
				k := arg[:idx]
				v := arg[idx+1:]
				if k == "spring.application.json" {
					kvs = append(kvs, parseJSON(unquote(v))...)
				} else {
					kvs = append(kvs, KeyValue{Key: k, Value: resolveEnvVars(unquote(v))})
				}
			}
		}
	}

	return kvs
}

// ApplicationJSON parses raw string as Spring application JSON.
func ApplicationJSON(raw string) []KeyValue {
	return parseJSON(raw)
}
