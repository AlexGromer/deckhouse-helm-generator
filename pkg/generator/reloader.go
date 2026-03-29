package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ReloaderOptions configures Stakater Reloader annotation injection.
type ReloaderOptions struct {
	AutoReload      bool
	WatchConfigMaps bool
	WatchSecrets    bool
}

// ReloaderCandidate represents a workload that should receive reloader annotations.
type ReloaderCandidate struct {
	WorkloadName      string
	MountedConfigMaps []string
	MountedSecrets    []string
}

// DetectReloaderCandidates scans the resource graph for workloads that mount
// ConfigMaps or Secrets and returns them as reloader candidates.
// It detects mounts both from graph relationships and from spec.template.spec.volumes.
func DetectReloaderCandidates(graph *types.ResourceGraph) []ReloaderCandidate {
	if graph == nil {
		return nil
	}

	var candidates []ReloaderCandidate

	for _, r := range graph.Resources {
		kind := r.Original.GVK.Kind
		if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" {
			continue
		}
		name := r.Original.Object.GetName()

		var configMaps, secrets []string

		// Strategy 1: parse volumes from object spec.
		configMaps, secrets = extractVolumeMounts(r.Original.Object.Object)

		// Strategy 2: check graph relationships for ConfigMap/Secret resources.
		if len(configMaps) == 0 && len(secrets) == 0 {
			key := r.Original.ResourceKey()
			for _, rel := range graph.GetRelationshipsFrom(key) {
				for _, target := range graph.Resources {
					if target.Original.ResourceKey() == rel.To {
						targetKind := target.Original.GVK.Kind
						targetName := target.Original.Object.GetName()
						switch targetKind {
						case "ConfigMap":
							configMaps = append(configMaps, targetName)
						case "Secret":
							secrets = append(secrets, targetName)
						}
					}
				}
			}
		}

		if len(configMaps) > 0 || len(secrets) > 0 {
			candidates = append(candidates, ReloaderCandidate{
				WorkloadName:      name,
				MountedConfigMaps: configMaps,
				MountedSecrets:    secrets,
			})
		}
	}

	return candidates
}

// extractVolumeMounts parses configmap and secret volume names from a workload object's
// spec.template.spec.volumes list.
func extractVolumeMounts(obj map[string]interface{}) (configMaps, secrets []string) {
	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return
	}
	tmpl, ok := spec["template"].(map[string]interface{})
	if !ok {
		return
	}
	tmplSpec, ok := tmpl["spec"].(map[string]interface{})
	if !ok {
		return
	}
	volumes, ok := tmplSpec["volumes"].([]interface{})
	if !ok {
		return
	}
	for _, v := range volumes {
		vol, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if cm, ok := vol["configMap"].(map[string]interface{}); ok {
			if cmName, ok := cm["name"].(string); ok {
				configMaps = append(configMaps, cmName)
			}
		}
		if sec, ok := vol["secret"].(map[string]interface{}); ok {
			if secName, ok := sec["secretName"].(string); ok {
				secrets = append(secrets, secName)
			}
		}
	}
	return
}

// InjectReloaderAnnotations injects Stakater Reloader annotations into workload
// templates in the chart. Returns the updated chart (copy-on-write) and count of
// templates modified. Returns (nil, 0) for nil chart.
func InjectReloaderAnnotations(chart *types.GeneratedChart, opts ReloaderOptions) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}

	newChart := copyChartTemplates(chart)
	count := 0

	for path, content := range newChart.Templates {
		if !isWorkloadTemplate(content) {
			continue
		}

		updated := injectReloaderAnnotationsIntoTemplate(content, opts)
		if updated != content {
			newChart.Templates[path] = updated
			count++
		}
	}

	return newChart, count
}

// injectReloaderAnnotationsIntoTemplate adds reloader annotations to a template.
func injectReloaderAnnotationsIntoTemplate(content string, opts ReloaderOptions) string {
	if !opts.AutoReload && !opts.WatchConfigMaps && !opts.WatchSecrets {
		return content
	}

	// Check if already injected.
	if strings.Contains(content, "reloader.stakater.com") {
		return content
	}

	var annotations strings.Builder
	if opts.AutoReload {
		annotations.WriteString(fmt.Sprintf("        reloader.stakater.com/auto: \"true\"\n"))
	}
	if opts.WatchConfigMaps {
		annotations.WriteString(fmt.Sprintf("        reloader.stakater.com/search: \"true\"\n"))
	}
	if opts.WatchSecrets {
		annotations.WriteString(fmt.Sprintf("        reloader.stakater.com/secret.reload: \"true\"\n"))
	}

	annotationBlock := annotations.String()
	if annotationBlock == "" {
		return content
	}

	// Inject into "annotations: {}" pattern.
	if strings.Contains(content, "      annotations: {}") {
		return strings.Replace(content,
			"      annotations: {}",
			"      annotations:\n"+annotationBlock,
			1)
	}
	// Inject after existing "annotations:" line.
	if strings.Contains(content, "      annotations:") {
		return strings.Replace(content,
			"      annotations:",
			"      annotations:\n"+annotationBlock,
			1)
	}
	// No annotations section — inject after template marker.
	if strings.Contains(content, "  template:") {
		return strings.Replace(content,
			"  template:",
			"  template:\n    metadata:\n      annotations:\n"+annotationBlock,
			1)
	}

	return content
}
