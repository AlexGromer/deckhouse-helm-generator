package generator

import (
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ImageFindingType categorizes image security issues.
type ImageFindingType string

const (
	FindingLatestTag     ImageFindingType = "latest-tag"
	FindingUntagged      ImageFindingType = "untagged"
	FindingNoPullPolicy  ImageFindingType = "missing-pull-policy"
)

// ImageFinding represents a security issue found in a container image reference.
type ImageFinding struct {
	// Image is the full image reference.
	Image string

	// Container is the container name.
	Container string

	// Resource is the resource name.
	Resource string

	// Type categorizes the finding.
	Type ImageFindingType

	// Message provides a human-readable description.
	Message string
}

// AnalyzeImageSecurity inspects all workload templates in a chart for image security issues.
// It checks for: untagged images (implicit :latest), explicit :latest tag, missing imagePullPolicy.
func AnalyzeImageSecurity(chart *types.GeneratedChart) []ImageFinding {
	if chart == nil {
		return nil
	}

	var findings []ImageFinding

	for path, content := range chart.Templates {
		kind := extractKind(content)
		if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" && kind != "Job" && kind != "CronJob" {
			continue
		}

		containers := extractImageRefs(content)
		for _, c := range containers {
			resource := resourceNameFromPath(path)

			if c.image == "" {
				continue
			}

			// Check for untagged (no colon → implicit latest)
			if !strings.Contains(c.image, ":") && !strings.Contains(c.image, "@") {
				findings = append(findings, ImageFinding{
					Image:     c.image,
					Container: c.name,
					Resource:  resource,
					Type:      FindingUntagged,
					Message:   "image has no tag (implicit :latest)",
				})
			}

			// Check for explicit :latest
			if strings.HasSuffix(c.image, ":latest") {
				findings = append(findings, ImageFinding{
					Image:     c.image,
					Container: c.name,
					Resource:  resource,
					Type:      FindingLatestTag,
					Message:   "image uses :latest tag",
				})
			}

			// Check for missing imagePullPolicy
			if c.pullPolicy == "" {
				findings = append(findings, ImageFinding{
					Image:     c.image,
					Container: c.name,
					Resource:  resource,
					Type:      FindingNoPullPolicy,
					Message:   "imagePullPolicy not set",
				})
			}
		}
	}

	return findings
}

// containerImageRef holds parsed container image info from YAML content.
type containerImageRef struct {
	name       string
	image      string
	pullPolicy string
}

// extractImageRefs parses container image references from a YAML template string.
// It looks for image: and imagePullPolicy: fields within container blocks.
func extractImageRefs(content string) []containerImageRef {
	var refs []containerImageRef

	lines := strings.Split(content, "\n")
	var currentName, currentImage, currentPolicy string
	inContainer := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect container name
		if strings.HasPrefix(trimmed, "- name:") || strings.HasPrefix(trimmed, "name:") {
			if inContainer && currentImage != "" {
				refs = append(refs, containerImageRef{
					name:       currentName,
					image:      currentImage,
					pullPolicy: currentPolicy,
				})
			}
			currentName = strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:"))
			currentName = strings.TrimSpace(strings.TrimPrefix(currentName, "name:"))
			currentName = strings.Trim(currentName, "\"' ")
			currentImage = ""
			currentPolicy = ""
			inContainer = true
		}

		if strings.HasPrefix(trimmed, "image:") {
			currentImage = strings.TrimSpace(strings.TrimPrefix(trimmed, "image:"))
			currentImage = strings.Trim(currentImage, "\"' ")
		}

		if strings.HasPrefix(trimmed, "imagePullPolicy:") {
			currentPolicy = strings.TrimSpace(strings.TrimPrefix(trimmed, "imagePullPolicy:"))
			currentPolicy = strings.Trim(currentPolicy, "\"' ")
		}
	}

	// Flush last container
	if inContainer && currentImage != "" {
		refs = append(refs, containerImageRef{
			name:       currentName,
			image:      currentImage,
			pullPolicy: currentPolicy,
		})
	}

	return refs
}

// resourceNameFromPath extracts a resource name from template path like "templates/myapp-deployment.yaml".
func resourceNameFromPath(path string) string {
	name := path
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.TrimSuffix(name, ".yaml")
	name = strings.TrimSuffix(name, ".yml")
	return name
}

// InjectImageDefaults sets imagePullPolicy based on tag conventions:
// - :latest or untagged → Always
// - tagged with specific version → IfNotPresent
// Also adds imagePullSecrets if a private registry is detected.
// Returns a new chart (copy-on-write).
func InjectImageDefaults(chart *types.GeneratedChart) *types.GeneratedChart {
	if chart == nil {
		return nil
	}

	newTemplates := make(map[string]string, len(chart.Templates))
	needsPrivateSecrets := false

	for path, content := range chart.Templates {
		kind := extractKind(content)
		if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" && kind != "Job" && kind != "CronJob" {
			newTemplates[path] = content
			continue
		}

		refs := extractImageRefs(content)
		modified := content

		for _, ref := range refs {
			if ref.image == "" {
				continue
			}

			// Detect private registry
			if isPrivateRegistry(ref.image) {
				needsPrivateSecrets = true
			}

			// Determine desired pull policy
			desiredPolicy := "IfNotPresent"
			if strings.HasSuffix(ref.image, ":latest") || !strings.Contains(ref.image, ":") {
				desiredPolicy = "Always"
			}

			// Inject imagePullPolicy if missing
			if ref.pullPolicy == "" {
				modified = injectPullPolicy(modified, ref.image, desiredPolicy)
			}
		}

		newTemplates[path] = modified
	}

	result := &types.GeneratedChart{
		Name:          chart.Name,
		Path:          chart.Path,
		ChartYAML:     chart.ChartYAML,
		ValuesYAML:    chart.ValuesYAML,
		Templates:     newTemplates,
		Helpers:       chart.Helpers,
		Notes:         chart.Notes,
		ValuesSchema:  chart.ValuesSchema,
		ExternalFiles: chart.ExternalFiles,
	}

	// Add imagePullSecrets to workload templates if private registry detected
	if needsPrivateSecrets {
		for path, content := range result.Templates {
			kind := extractKind(content)
			if kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" {
				if !strings.Contains(content, "imagePullSecrets") {
					result.Templates[path] = injectImagePullSecrets(content)
				}
			}
		}
	}

	return result
}

// isPrivateRegistry returns true if the image reference points to a private registry.
func isPrivateRegistry(image string) bool {
	// Images with a dot in the first segment before / are registry URLs.
	// e.g., registry.example.com/myapp:v1, gcr.io/project/image:v1
	// Public: nginx, library/nginx, docker.io/library/nginx
	parts := strings.SplitN(image, "/", 2)
	if len(parts) < 2 {
		return false // simple image like "nginx"
	}
	registry := parts[0]
	if registry == "docker.io" || registry == "index.docker.io" {
		return false
	}
	return strings.Contains(registry, ".") || strings.Contains(registry, ":")
}

// injectPullPolicy inserts imagePullPolicy after the image: line in YAML content.
func injectPullPolicy(content, image, policy string) string {
	lines := strings.Split(content, "\n")
	var result []string

	for i, line := range lines {
		result = append(result, line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "image:") && strings.Contains(trimmed, image) {
			// Check next line isn't already imagePullPolicy
			if i+1 < len(lines) && strings.Contains(strings.TrimSpace(lines[i+1]), "imagePullPolicy") {
				continue
			}
			indent := len(line) - len(strings.TrimLeft(line, " "))
			result = append(result, strings.Repeat(" ", indent)+"imagePullPolicy: "+policy)
		}
	}

	return strings.Join(result, "\n")
}

// injectImagePullSecrets adds imagePullSecrets section to a workload template.
func injectImagePullSecrets(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		result = append(result, line)
		trimmed := strings.TrimSpace(line)
		// Insert after "containers:" line at the same indent level
		if trimmed == "containers:" {
			indent := len(line) - len(strings.TrimLeft(line, " "))
			result = append(result,
				strings.Repeat(" ", indent)+"imagePullSecrets:",
				strings.Repeat(" ", indent)+"  - name: registry-secret",
			)
		}
	}

	return strings.Join(result, "\n")
}
