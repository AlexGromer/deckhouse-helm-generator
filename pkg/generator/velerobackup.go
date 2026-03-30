package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// VeleroBackupOptions configures Velero backup schedule generation.
type VeleroBackupOptions struct {
	Namespace               string
	ScheduleCron            string
	TTL                     string
	IncludeNamespaces       []string
	StorageLocation         string
	VolumeSnapshotLocations []string
}

// VeleroBackupResult holds the generated Velero backup resources.
type VeleroBackupResult struct {
	// Schedules maps StatefulSet name → Velero Schedule CRD YAML.
	Schedules map[string]string
	// Annotations maps annotation key → value for pods/StatefulSets with PVCs.
	Annotations map[string]string
	// NOTESTxt is a human-readable notes string.
	NOTESTxt string
}

// GenerateVeleroBackup inspects the resource graph for StatefulSets with PVCs
// and produces a VeleroBackupResult with Schedule CRDs and backup annotations.
func GenerateVeleroBackup(graph *types.ResourceGraph, opts VeleroBackupOptions) *VeleroBackupResult {
	result := &VeleroBackupResult{
		Schedules:   make(map[string]string),
		Annotations: make(map[string]string),
	}

	if graph == nil {
		result.NOTESTxt = buildVeleroNotes(0)
		return result
	}

	// Apply defaults.
	cron := opts.ScheduleCron
	if cron == "" {
		cron = "0 2 * * *"
	}
	ttl := opts.TTL
	if ttl == "" {
		ttl = "720h"
	}
	storageLoc := opts.StorageLocation
	if storageLoc == "" {
		storageLoc = "default"
	}

	statefulSets := graph.GetResourcesByKind("StatefulSet")
	for _, r := range statefulSets {
		if r == nil || r.Original == nil || r.Original.Object == nil {
			continue
		}

		name := r.ServiceName
		if name == "" {
			name = r.Original.Object.GetName()
		}
		ns := opts.Namespace
		if ns == "" {
			ns = r.Original.Object.GetNamespace()
		}
		if ns == "" {
			ns = "default"
		}

		// Check for volumeClaimTemplates.
		hasPVC := hasVolumeClaimTemplates(r)

		// Build Schedule YAML.
		scheduleYAML := buildVeleroScheduleYAML(name, ns, cron, ttl, storageLoc, opts.IncludeNamespaces, opts.VolumeSnapshotLocations)
		result.Schedules[name] = scheduleYAML

		// Emit backup-volumes annotation if StatefulSet has PVCs.
		if hasPVC {
			annotKey := fmt.Sprintf("backup.velero.io/backup-volumes-%s", name)
			result.Annotations[annotKey] = "true"
		}
	}

	result.NOTESTxt = buildVeleroNotes(len(result.Schedules))
	return result
}

// hasVolumeClaimTemplates returns true if the StatefulSet resource has volumeClaimTemplates.
func hasVolumeClaimTemplates(r *types.ProcessedResource) bool {
	obj := r.Original.Object.Object
	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return false
	}
	vcts, ok := spec["volumeClaimTemplates"]
	if !ok {
		return false
	}
	list, ok := vcts.([]interface{})
	return ok && len(list) > 0
}

// sanitizeYAMLScalar strips newline and carriage-return characters from a string
// to prevent YAML structure injection. This is a conservative guard until a
// full yamlScalarQuote helper is introduced in the project.
func sanitizeYAMLScalar(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

// buildVeleroScheduleYAML renders a Velero Schedule CRD as YAML string.
func buildVeleroScheduleYAML(name, namespace, cron, ttl, storageLocation string, includeNamespaces, volumeSnapshotLocations []string) string {
	var sb strings.Builder

	// Sanitize all user-supplied scalar values before YAML interpolation.
	name = sanitizeYAMLScalar(name)
	namespace = sanitizeYAMLScalar(namespace)
	cron = sanitizeYAMLScalar(cron)
	ttl = sanitizeYAMLScalar(ttl)
	storageLocation = sanitizeYAMLScalar(storageLocation)

	sb.WriteString("apiVersion: velero.io/v1\n")
	sb.WriteString("kind: Schedule\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s-backup\n", name))
	sb.WriteString(fmt.Sprintf("  namespace: %s\n", namespace))
	sb.WriteString("spec:\n")
	sb.WriteString(fmt.Sprintf("  schedule: \"%s\"\n", cron))
	sb.WriteString("  template:\n")
	sb.WriteString(fmt.Sprintf("    ttl: %s\n", ttl))
	sb.WriteString(fmt.Sprintf("    storageLocation: %s\n", storageLocation))

	if len(includeNamespaces) > 0 {
		sb.WriteString("    includedNamespaces:\n")
		for _, ns := range includeNamespaces {
			sb.WriteString(fmt.Sprintf("      - %s\n", sanitizeYAMLScalar(ns)))
		}
	}

	if len(volumeSnapshotLocations) > 0 {
		sb.WriteString("    volumeSnapshotLocations:\n")
		for _, vsl := range volumeSnapshotLocations {
			sb.WriteString(fmt.Sprintf("      - %s\n", sanitizeYAMLScalar(vsl)))
		}
	}

	return sb.String()
}

// buildVeleroNotes returns a human-readable notes string for Velero backup.
func buildVeleroNotes(scheduleCount int) string {
	return fmt.Sprintf(
		"Velero backup configured.\n"+
			"Generated %d Velero Schedule CRD(s).\n"+
			"Apply the generated schedules to enable automated backups.\n"+
			"Ensure the velero CLI and server are installed in your cluster.",
		scheduleCount,
	)
}

// InjectVeleroBackup injects Velero Schedule CRDs into the chart's ExternalFiles.
// It is copy-on-write (original chart is not modified) and idempotent.
// Returns the updated chart and the number of files injected.
func InjectVeleroBackup(chart *types.GeneratedChart, result *VeleroBackupResult) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}
	if result == nil || len(result.Schedules) == 0 {
		return chart, 0
	}

	// Build set of already-present external file paths for idempotency.
	existing := make(map[string]struct{}, len(chart.ExternalFiles))
	for _, ef := range chart.ExternalFiles {
		existing[ef.Path] = struct{}{}
	}

	updated := copyChartTemplates(chart)
	// Copy ExternalFiles slice (copyChartTemplates does not copy it).
	updated.ExternalFiles = make([]types.ExternalFileInfo, len(chart.ExternalFiles))
	copy(updated.ExternalFiles, chart.ExternalFiles)

	count := 0
	for stsName, scheduleYAML := range result.Schedules {
		path := fmt.Sprintf("velero/schedules/%s-schedule.yaml", stsName)
		// Idempotent: overwrite if already present, add if not.
		if _, found := existing[path]; found {
			// Overwrite in-place.
			for i, ef := range updated.ExternalFiles {
				if ef.Path == path {
					updated.ExternalFiles[i].Content = scheduleYAML
					break
				}
			}
		} else {
			updated.ExternalFiles = append(updated.ExternalFiles, types.ExternalFileInfo{
				Path:    path,
				Content: scheduleYAML,
			})
		}
		count++
	}

	// Update NOTES.txt if provided.
	if result.NOTESTxt != "" {
		updated.Notes = result.NOTESTxt
	}

	return updated, count
}
