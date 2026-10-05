package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

const (
	defaultVeleroSchedule  = "0 2 * * *"
	defaultVeleroTTL       = "720h0m0s"
	defaultVeleroNamespace = "velero"
	veleroValuesKey        = "veleroBackup"
	// VeleroScheduleTemplatePath is the template added by InjectVeleroBackup.
	VeleroScheduleTemplatePath = "templates/velero-schedule.yaml"
)

// VeleroBackupOptions configures the defaults written to values.yaml
// (veleroBackup.*). All of them can be changed at install time.
type VeleroBackupOptions struct {
	// Namespace is where Velero runs; Schedules must live there (default "velero").
	Namespace string
	// Schedule is the cron expression of the backup (default "0 2 * * *").
	Schedule string
	// TTL is how long backups are kept, as a Go duration (default 720h0m0s).
	TTL string
	// StorageLocation is the BackupStorageLocation; empty uses Velero's default.
	StorageLocation string
	// VolumeSnapshotLocations restricts the VolumeSnapshotLocations used.
	VolumeSnapshotLocations []string
	// SnapshotVolumes takes volume snapshots of the release's PVs.
	SnapshotVolumes bool
	// DefaultVolumesToFsBackup backs up all pod volumes with file-system backup
	// (Kopia/Restic) instead of snapshots.
	DefaultVolumesToFsBackup bool
}

// normalize applies defaults and validates the options.
func (opts *VeleroBackupOptions) normalize() error {
	if opts.Namespace == "" {
		opts.Namespace = defaultVeleroNamespace
	}
	if opts.Schedule == "" {
		opts.Schedule = defaultVeleroSchedule
	}
	if opts.TTL == "" {
		opts.TTL = defaultVeleroTTL
	}
	if err := opsValidateDuration(opts.TTL); err != nil {
		return fmt.Errorf("velero ttl: %w", err)
	}
	if fields := strings.Fields(opts.Schedule); len(fields) != 5 && !strings.HasPrefix(opts.Schedule, "@") {
		return fmt.Errorf("velero schedule %q is not a 5-field cron expression", opts.Schedule)
	}
	return nil
}

// GenerateVeleroScheduleTemplate returns a Helm template for a Velero
// Schedule (velero.io/v1) that backs up the release: every object carrying
// the chart's selector labels in the release namespace, together with the
// volumes of its pods. prefix is the prefix of the chart's
// "<prefix>.fullname" helpers.
func GenerateVeleroScheduleTemplate(prefix string) string {
	v := ".Values." + veleroValuesKey
	var b strings.Builder
	w := func(s string) { b.WriteString(s + "\n") }
	w(`{{- if ` + v + `.enabled }}`)
	w(`apiVersion: velero.io/v1`)
	w(`kind: Schedule`)
	w(`metadata:`)
	w(fmt.Sprintf(`  name: {{ include %q . | trunc 56 | trimSuffix "-" }}-backup`, prefix+".fullname"))
	w(`  namespace: {{ ` + v + `.namespace | default "` + defaultVeleroNamespace + `" }}`)
	w(`  labels:`)
	w(fmt.Sprintf(`    {{- include %q . | nindent 4 }}`, prefix+".labels"))
	w(`spec:`)
	w(`  schedule: {{ ` + v + `.schedule | default "` + defaultVeleroSchedule + `" | quote }}`)
	w(`  template:`)
	w(`    includedNamespaces:`)
	w(`      - {{ .Release.Namespace | quote }}`)
	w(`    labelSelector:`)
	w(`      matchLabels:`)
	w(fmt.Sprintf(`        {{- include %q . | nindent 8 }}`, prefix+".selectorLabels"))
	w(`    ttl: {{ ` + v + `.ttl | default "` + defaultVeleroTTL + `" | quote }}`)
	w(`    {{- with ` + v + `.storageLocation }}`)
	w(`    storageLocation: {{ . | quote }}`)
	w(`    {{- end }}`)
	w(`    {{- with ` + v + `.volumeSnapshotLocations }}`)
	w(`    volumeSnapshotLocations:`)
	w(`      {{- toYaml . | nindent 6 }}`)
	w(`    {{- end }}`)
	w(`    {{- if kindIs "bool" ` + v + `.snapshotVolumes }}`)
	w(`    snapshotVolumes: {{ ` + v + `.snapshotVolumes }}`)
	w(`    {{- end }}`)
	w(`    {{- if kindIs "bool" ` + v + `.defaultVolumesToFsBackup }}`)
	w(`    defaultVolumesToFsBackup: {{ ` + v + `.defaultVolumesToFsBackup }}`)
	w(`    {{- end }}`)
	w(`{{- end }}`)
	return b.String()
}

// InjectVeleroBackup adds the Velero Schedule template and its values
// (veleroBackup.*) to the chart. It returns the chart unchanged if the
// template is already present. The input chart is not modified.
func InjectVeleroBackup(chart *types.GeneratedChart, opts VeleroBackupOptions) (*types.GeneratedChart, bool, error) {
	if chart == nil {
		return nil, false, nil
	}
	if _, exists := chart.Templates[VeleroScheduleTemplatePath]; exists {
		return chart, false, nil
	}
	if err := opts.normalize(); err != nil {
		return nil, false, err
	}

	locations := make([]interface{}, 0, len(opts.VolumeSnapshotLocations))
	for _, l := range opts.VolumeSnapshotLocations {
		locations = append(locations, l)
	}
	values, err := appendTopLevelValues(chart.ValuesYAML, veleroValuesKey, map[string]interface{}{
		"enabled":                  true,
		"namespace":                opts.Namespace,
		"schedule":                 opts.Schedule,
		"ttl":                      opts.TTL,
		"storageLocation":          opts.StorageLocation,
		"volumeSnapshotLocations":  locations,
		"snapshotVolumes":          opts.SnapshotVolumes,
		"defaultVolumesToFsBackup": opts.DefaultVolumesToFsBackup,
	})
	if err != nil {
		return nil, false, err
	}

	out := cloneChart(chart)
	out.ValuesYAML = values
	out.Templates[VeleroScheduleTemplatePath] = GenerateVeleroScheduleTemplate(opsHelperPrefix(chart))
	return out, true, nil
}

// hasVolumeClaimTemplates returns true if the StatefulSet resource has volumeClaimTemplates.
func hasVolumeClaimTemplates(r *types.ProcessedResource) bool {
	return len(volumeClaimTemplates(r)) > 0
}

// volumeClaimTemplates returns the volumeClaimTemplates of a StatefulSet.
func volumeClaimTemplates(r *types.ProcessedResource) []map[string]interface{} {
	if r == nil || r.Original == nil || r.Original.Object == nil {
		return nil
	}
	spec, ok := r.Original.Object.Object["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	list, ok := spec["volumeClaimTemplates"].([]interface{})
	if !ok {
		return nil
	}
	var out []map[string]interface{}
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}
