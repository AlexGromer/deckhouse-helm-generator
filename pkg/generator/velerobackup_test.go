package generator

// ============================================================
// Test Plan — velerobackup_test.go
//
// Tests for GenerateVeleroBackup and InjectVeleroBackup.
// Detection: StatefulSet/PVC → Velero Schedule CRD + backup-volumes annotations.
//
//  1. TestVeleroBackup_StatefulSet_GeneratesScheduleCRD          — happy   StatefulSet in graph → Schedules map non-empty
//  2. TestVeleroBackup_StatefulSetWithPVC_BackupVolumesAnnotation — happy   StatefulSet with PVC → backup-volumes annotation present
//  3. TestVeleroBackup_CustomCron_UsedInSchedule                 — happy   custom ScheduleCron → schedule YAML contains custom cron
//  4. TestVeleroBackup_CustomTTL_UsedInSchedule                  — happy   custom TTL → schedule YAML contains custom TTL
//  5. TestVeleroBackup_StorageLocation_InSchedule                — happy   StorageLocation set → schedule YAML contains location name
//  6. TestVeleroBackup_EmptyGraph_NoSchedules                    — edge    empty ResourceGraph → len(Schedules)==0
//  7. TestVeleroBackup_NilChart_InjectReturnsZero                — error   nil chart → count==0, no panic
//  8. TestVeleroBackup_CopyOnWrite_OriginalUnchanged             — happy   InjectVeleroBackup returns new chart, original unchanged
//  9. TestVeleroBackup_Idempotent_DoubleInject                   — happy   injecting twice does not double-count annotations/schedules
// 10. TestVeleroBackup_NOTESTxt_MentionsVelero                  — happy   NOTESTxt field contains "velero" substring
// 11. TestVeleroBackup_MultipleStatefulSets_MultipleSchedules    — happy   3 StatefulSets → 3 schedule entries
// 12. TestVeleroBackup_VolumeSnapshotLocation_InSchedule        — happy   VolumeSnapshotLocations set → schedule YAML contains location
// ============================================================

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// makeStatefulSetResource creates a ProcessedResource for a StatefulSet.
func makeStatefulSetResource(name, namespace string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "app",
								"image": "postgres:16",
							},
						},
					},
				},
				"volumeClaimTemplates": []interface{}{
					map[string]interface{}{
						"metadata": map[string]interface{}{
							"name": "data",
						},
						"spec": map[string]interface{}{
							"accessModes": []interface{}{"ReadWriteOnce"},
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"storage": "10Gi",
								},
							},
						},
					},
				},
			},
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK: schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "StatefulSet",
			},
		},
		ServiceName: name,
		Values:      make(map[string]interface{}),
	}
}

// makeStatefulSetResourceNoPVC creates a StatefulSet without volumeClaimTemplates.
func makeStatefulSetResourceNoPVC(name, namespace string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "app",
								"image": "nginx:latest",
							},
						},
					},
				},
			},
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK: schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "StatefulSet",
			},
		},
		ServiceName: name,
		Values:      make(map[string]interface{}),
	}
}

// makeGraphWithStatefulSets builds a ResourceGraph from one or more StatefulSet resources.
func makeGraphWithStatefulSets(resources ...*types.ProcessedResource) *types.ResourceGraph {
	graph := types.NewResourceGraph()
	for _, r := range resources {
		graph.AddResource(r)
	}
	return graph
}

// defaultVeleroOpts returns a VeleroBackupOptions with all defaults set.
func defaultVeleroOpts() VeleroBackupOptions {
	return VeleroBackupOptions{
		Namespace:     "default",
		ScheduleCron:  "0 2 * * *",
		TTL:           "720h",
		StorageLocation: "default",
	}
}

// makeVeleroChart returns a minimal GeneratedChart suitable for InjectVeleroBackup tests.
func makeVeleroChart(name string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:      name,
		ChartYAML: "apiVersion: v2\nname: " + name + "\nversion: 0.1.0\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates: map[string]string{
			"templates/statefulset.yaml": "apiVersion: apps/v1\nkind: StatefulSet\n",
		},
		Notes: "",
	}
}

// ─── 1. StatefulSet → Schedule CRD generated ────────────────────────────────

func TestVeleroBackup_StatefulSet_GeneratesScheduleCRD(t *testing.T) {
	sts := makeStatefulSetResource("db", "default")
	graph := makeGraphWithStatefulSets(sts)
	opts := defaultVeleroOpts()

	result := GenerateVeleroBackup(graph, opts)
	if result == nil {
		t.Fatal("GenerateVeleroBackup returned nil result")
	}
	if len(result.Schedules) == 0 {
		t.Error("expected at least one Velero Schedule to be generated for a StatefulSet, got none")
	}
}

// ─── 2. StatefulSet with PVC → backup-volumes annotation ────────────────────

func TestVeleroBackup_StatefulSetWithPVC_BackupVolumesAnnotation(t *testing.T) {
	sts := makeStatefulSetResource("db", "default")
	graph := makeGraphWithStatefulSets(sts)
	opts := defaultVeleroOpts()

	result := GenerateVeleroBackup(graph, opts)
	if result == nil {
		t.Fatal("GenerateVeleroBackup returned nil result")
	}

	found := false
	for k := range result.Annotations {
		if strings.Contains(k, "backup.velero.io") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a 'backup.velero.io' annotation in Annotations map, got: %v",
			result.Annotations)
	}
}

// ─── 3. Custom cron used in schedule ────────────────────────────────────────

func TestVeleroBackup_CustomCron_UsedInSchedule(t *testing.T) {
	sts := makeStatefulSetResource("db", "default")
	graph := makeGraphWithStatefulSets(sts)
	customCron := "0 3 * * 0"
	opts := VeleroBackupOptions{
		Namespace:       "default",
		ScheduleCron:    customCron,
		TTL:             "720h",
		StorageLocation: "default",
	}

	result := GenerateVeleroBackup(graph, opts)
	if result == nil {
		t.Fatal("GenerateVeleroBackup returned nil result")
	}
	if len(result.Schedules) == 0 {
		t.Fatal("expected at least one schedule, got none")
	}

	found := false
	for _, yaml := range result.Schedules {
		if strings.Contains(yaml, customCron) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected custom cron %q to appear in at least one schedule YAML", customCron)
	}
}

// ─── 4. Custom TTL used in schedule ─────────────────────────────────────────

func TestVeleroBackup_CustomTTL_UsedInSchedule(t *testing.T) {
	sts := makeStatefulSetResource("db", "default")
	graph := makeGraphWithStatefulSets(sts)
	customTTL := "168h"
	opts := VeleroBackupOptions{
		Namespace:       "default",
		ScheduleCron:    "0 2 * * *",
		TTL:             customTTL,
		StorageLocation: "default",
	}

	result := GenerateVeleroBackup(graph, opts)
	if result == nil {
		t.Fatal("GenerateVeleroBackup returned nil result")
	}
	if len(result.Schedules) == 0 {
		t.Fatal("expected at least one schedule, got none")
	}

	found := false
	for _, yaml := range result.Schedules {
		if strings.Contains(yaml, customTTL) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected custom TTL %q to appear in at least one schedule YAML", customTTL)
	}
}

// ─── 5. Storage location in schedule ────────────────────────────────────────

func TestVeleroBackup_StorageLocation_InSchedule(t *testing.T) {
	sts := makeStatefulSetResource("db", "default")
	graph := makeGraphWithStatefulSets(sts)
	location := "aws-s3"
	opts := VeleroBackupOptions{
		Namespace:       "default",
		ScheduleCron:    "0 2 * * *",
		TTL:             "720h",
		StorageLocation: location,
	}

	result := GenerateVeleroBackup(graph, opts)
	if result == nil {
		t.Fatal("GenerateVeleroBackup returned nil result")
	}
	if len(result.Schedules) == 0 {
		t.Fatal("expected at least one schedule, got none")
	}

	found := false
	for _, yaml := range result.Schedules {
		if strings.Contains(yaml, location) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected storage location %q to appear in at least one schedule YAML", location)
	}
}

// ─── 6. Empty graph → no schedules ──────────────────────────────────────────

func TestVeleroBackup_EmptyGraph_NoSchedules(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := defaultVeleroOpts()

	result := GenerateVeleroBackup(graph, opts)
	if result == nil {
		t.Fatal("GenerateVeleroBackup returned nil result for empty graph")
	}
	if len(result.Schedules) != 0 {
		t.Errorf("expected 0 schedules for empty graph, got %d", len(result.Schedules))
	}
}

// ─── 7. Nil chart → InjectVeleroBackup returns zero ─────────────────────────

func TestVeleroBackup_NilChart_InjectReturnsZero(t *testing.T) {
	sts := makeStatefulSetResource("db", "default")
	graph := makeGraphWithStatefulSets(sts)
	result := GenerateVeleroBackup(graph, defaultVeleroOpts())

	// Must not panic on nil chart; must return count 0.
	updated, count := InjectVeleroBackup(nil, result)
	if count != 0 {
		t.Errorf("expected count==0 for nil chart, got %d", count)
	}
	if updated != nil {
		t.Errorf("expected nil chart returned for nil input, got non-nil")
	}
}

// ─── 8. Copy-on-write: original chart unchanged ──────────────────────────────

func TestVeleroBackup_CopyOnWrite_OriginalUnchanged(t *testing.T) {
	chart := makeVeleroChart("myapp")
	originalNotes := chart.Notes
	originalTemplateCount := len(chart.Templates)

	sts := makeStatefulSetResource("db", "default")
	graph := makeGraphWithStatefulSets(sts)
	result := GenerateVeleroBackup(graph, defaultVeleroOpts())

	updated, count := InjectVeleroBackup(chart, result)
	if count == 0 {
		t.Skip("no injection performed — skipping copy-on-write check (schedule may be empty)")
	}

	// Original chart must be unchanged.
	if chart.Notes != originalNotes {
		t.Errorf("original chart.Notes was modified: got %q, want %q", chart.Notes, originalNotes)
	}
	if len(chart.Templates) != originalTemplateCount {
		t.Errorf("original chart.Templates was modified: len=%d, want %d",
			len(chart.Templates), originalTemplateCount)
	}

	// Updated chart must be a different pointer.
	if updated == chart {
		t.Error("InjectVeleroBackup returned the same chart pointer — expected a copy")
	}
}

// ─── 9. Idempotent: injecting twice does not double-count ───────────────────

func TestVeleroBackup_Idempotent_DoubleInject(t *testing.T) {
	chart := makeVeleroChart("myapp")
	sts := makeStatefulSetResource("db", "default")
	graph := makeGraphWithStatefulSets(sts)
	result := GenerateVeleroBackup(graph, defaultVeleroOpts())

	first, count1 := InjectVeleroBackup(chart, result)
	if first == nil || count1 == 0 {
		t.Skip("first inject produced no changes — skipping idempotency check")
	}

	second, count2 := InjectVeleroBackup(first, result)
	if second == nil {
		t.Fatal("second inject returned nil chart")
	}

	// The second inject should not produce strictly more injected items than the first.
	// An idempotent implementation would inject 0 on the second pass (already present)
	// or at most the same count (overwrite). It must never exceed the first count.
	if count2 > count1 {
		t.Errorf("second inject produced more items (%d) than first (%d) — not idempotent",
			count2, count1)
	}
}

// ─── 10. NOTESTxt mentions velero ───────────────────────────────────────────

func TestVeleroBackup_NOTESTxt_MentionsVelero(t *testing.T) {
	sts := makeStatefulSetResource("db", "default")
	graph := makeGraphWithStatefulSets(sts)
	result := GenerateVeleroBackup(graph, defaultVeleroOpts())

	if result == nil {
		t.Fatal("GenerateVeleroBackup returned nil result")
	}
	if !strings.Contains(strings.ToLower(result.NOTESTxt), "velero") {
		t.Errorf("expected NOTESTxt to mention 'velero', got: %q", result.NOTESTxt)
	}
}

// ─── 11. Multiple StatefulSets → multiple schedules ─────────────────────────

func TestVeleroBackup_MultipleStatefulSets_MultipleSchedules(t *testing.T) {
	sts1 := makeStatefulSetResource("db-primary", "default")
	sts2 := makeStatefulSetResource("db-replica", "default")
	sts3 := makeStatefulSetResource("cache", "default")
	graph := makeGraphWithStatefulSets(sts1, sts2, sts3)
	opts := defaultVeleroOpts()

	result := GenerateVeleroBackup(graph, opts)
	if result == nil {
		t.Fatal("GenerateVeleroBackup returned nil result")
	}
	if len(result.Schedules) != 3 {
		t.Errorf("expected 3 Velero Schedules for 3 StatefulSets, got %d", len(result.Schedules))
	}
}

// ─── 12. VolumeSnapshotLocation in schedule ─────────────────────────────────

func TestVeleroBackup_VolumeSnapshotLocation_InSchedule(t *testing.T) {
	sts := makeStatefulSetResource("db", "default")
	graph := makeGraphWithStatefulSets(sts)
	vsl := "gcp-vsl"
	opts := VeleroBackupOptions{
		Namespace:               "default",
		ScheduleCron:            "0 2 * * *",
		TTL:                     "720h",
		StorageLocation:         "default",
		VolumeSnapshotLocations: []string{vsl},
	}

	result := GenerateVeleroBackup(graph, opts)
	if result == nil {
		t.Fatal("GenerateVeleroBackup returned nil result")
	}
	if len(result.Schedules) == 0 {
		t.Fatal("expected at least one schedule, got none")
	}

	found := false
	for _, yaml := range result.Schedules {
		if strings.Contains(yaml, vsl) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected VolumeSnapshotLocation %q to appear in at least one schedule YAML", vsl)
	}
}

