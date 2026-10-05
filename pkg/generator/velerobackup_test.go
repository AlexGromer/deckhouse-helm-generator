package generator

import (
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestGenerateVeleroScheduleTemplate(t *testing.T) {
	tpl := GenerateVeleroScheduleTemplate("db")
	for _, want := range []string{
		"{{- if .Values.veleroBackup.enabled }}",
		"apiVersion: velero.io/v1\nkind: Schedule\n",
		`  name: {{ include "db.fullname" . | trunc 56 | trimSuffix "-" }}-backup`,
		`  namespace: {{ .Values.veleroBackup.namespace | default "velero" }}`,
		"    includedNamespaces:\n      - {{ .Release.Namespace | quote }}",
		"      matchLabels:\n        {{- include \"db.selectorLabels\" . | nindent 8 }}",
		`    ttl: {{ .Values.veleroBackup.ttl | default "720h0m0s" | quote }}`,
		"    snapshotVolumes: {{ .Values.veleroBackup.snapshotVolumes }}",
		"    defaultVolumesToFsBackup: {{ .Values.veleroBackup.defaultVolumesToFsBackup }}",
	} {
		if !strings.Contains(tpl, want) {
			t.Errorf("template misses %q:\n%s", want, tpl)
		}
	}
}

func TestInjectVeleroBackup(t *testing.T) {
	in := &types.GeneratedChart{
		Name:       "app/charts/db",
		ChartYAML:  "apiVersion: v2\nname: db\nversion: 0.1.0\n",
		ValuesYAML: "statefulSet:\n  replicas: 1\n",
		Helpers:    `{{- define "db.fullname" -}}x{{- end }}`,
		Templates:  map[string]string{"templates/db-statefulset.yaml": "kind: StatefulSet\n"},
	}
	out, added, err := InjectVeleroBackup(in, VeleroBackupOptions{
		StorageLocation:         "s3",
		VolumeSnapshotLocations: []string{"aws"},
		SnapshotVolumes:         true,
	})
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	if _, ok := in.Templates[VeleroScheduleTemplatePath]; ok {
		t.Error("input chart was modified")
	}
	if !strings.Contains(out.Templates[VeleroScheduleTemplatePath], `include "db.fullname"`) {
		t.Error("template must use the chart's own helpers")
	}

	var values struct {
		VeleroBackup struct {
			Enabled                  bool     `json:"enabled"`
			Namespace                string   `json:"namespace"`
			Schedule                 string   `json:"schedule"`
			TTL                      string   `json:"ttl"`
			StorageLocation          string   `json:"storageLocation"`
			VolumeSnapshotLocations  []string `json:"volumeSnapshotLocations"`
			SnapshotVolumes          bool     `json:"snapshotVolumes"`
			DefaultVolumesToFsBackup bool     `json:"defaultVolumesToFsBackup"`
		} `json:"veleroBackup"`
	}
	if err := yaml.Unmarshal([]byte(out.ValuesYAML), &values); err != nil {
		t.Fatal(err)
	}
	v := values.VeleroBackup
	if !v.Enabled || v.Namespace != "velero" || v.Schedule != "0 2 * * *" || v.TTL != "720h0m0s" ||
		v.StorageLocation != "s3" || len(v.VolumeSnapshotLocations) != 1 || !v.SnapshotVolumes || v.DefaultVolumesToFsBackup {
		t.Errorf("unexpected values: %+v", v)
	}

	again, added, err := InjectVeleroBackup(out, VeleroBackupOptions{})
	if err != nil || added || again != out {
		t.Errorf("second application changed the chart (added=%v err=%v)", added, err)
	}

	for _, bad := range []VeleroBackupOptions{{TTL: "30d"}, {Schedule: "daily"}} {
		if _, _, err := InjectVeleroBackup(in, bad); err == nil {
			t.Errorf("expected an error for %+v", bad)
		}
	}
}

func TestOpsHasPersistentStorage(t *testing.T) {
	sts := makeTestGraphWithWorkload("StatefulSet", "db", "default", 1, "", "", "", "")
	var stsRes *types.ProcessedResource
	for _, r := range sts.Resources {
		stsRes = r
	}
	if opsHasPersistentStorage([]*types.ProcessedResource{stsRes}) {
		t.Error("StatefulSet without volumeClaimTemplates has no persistent storage")
	}
	spec := stsRes.Original.Object.Object["spec"].(map[string]interface{})
	spec["volumeClaimTemplates"] = []interface{}{map[string]interface{}{"metadata": map[string]interface{}{"name": "data"}}}
	if !opsHasPersistentStorage([]*types.ProcessedResource{stsRes}) {
		t.Error("StatefulSet with volumeClaimTemplates has persistent storage")
	}

	pvc := makeTestGraphWithPVC("data", "default", "ReadWriteOnce", "OMIT")
	for _, r := range pvc.Resources {
		if !opsHasPersistentStorage([]*types.ProcessedResource{r}) {
			t.Error("PVC is persistent storage")
		}
	}
}
