package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ChecksumAnnotation represents a checksum annotation for a referenced resource.
type ChecksumAnnotation struct {
	// Key is the annotation key (e.g., "checksum/config-myapp")
	Key string
	// TemplatePath is the path used in sha256sum (e.g., "configmap.yaml")
	TemplatePath string
	// Expression is the Helm template expression
	Expression string
}

// GenerateChecksumAnnotations analyzes a workload's dependencies and generates
// checksum annotations for referenced ConfigMaps and Secrets.
// This ensures pods restart when ConfigMap/Secret content changes.
func GenerateChecksumAnnotations(serviceName string, dependencies []types.ResourceKey) []ChecksumAnnotation {
	var annotations []ChecksumAnnotation

	seen := make(map[string]bool)
	for _, dep := range dependencies {
		switch dep.GVK.Kind {
		case "ConfigMap":
			key := fmt.Sprintf("checksum/config-%s", dep.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			templatePath := fmt.Sprintf("%s-configmap.yaml", serviceName)
			annotations = append(annotations, ChecksumAnnotation{
				Key:          key,
				TemplatePath: templatePath,
				Expression:   fmt.Sprintf(`{{ include (print $.Template.BasePath "/%s") . | sha256sum }}`, templatePath),
			})
		case "Secret":
			key := fmt.Sprintf("checksum/secret-%s", dep.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			templatePath := fmt.Sprintf("%s-secret.yaml", serviceName)
			annotations = append(annotations, ChecksumAnnotation{
				Key:          key,
				TemplatePath: templatePath,
				Expression:   fmt.Sprintf(`{{ include (print $.Template.BasePath "/%s") . | sha256sum }}`, templatePath),
			})
		}
	}

	return annotations
}

// FormatChecksumAnnotations formats checksum annotations as YAML for inclusion in pod template.
func FormatChecksumAnnotations(annotations []ChecksumAnnotation) string {
	if len(annotations) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, ann := range annotations {
		sb.WriteString(fmt.Sprintf("        %s: %s\n", ann.Key, ann.Expression))
	}
	return sb.String()
}
