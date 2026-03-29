package generator

// ============================================================
// Test Plan — pvanalysis_test.go
//
//  1. TestPVAnalysis_RWX_OnDeployment_Misuse            — happy   RWX accessMode on Deployment-owned PVC → "rwx-misuse" finding
//  2. TestPVAnalysis_RWX_OnStatefulSet_OK               — happy   RWX accessMode on StatefulSet-owned PVC → no "rwx-misuse"
//  3. TestPVAnalysis_NoStorageClassName_Finding         — happy   PVC without storageClassName → "missing-storage-class" finding
//  4. TestPVAnalysis_EmptyStorageClassName_Finding      — edge    PVC with storageClassName="" → "missing-storage-class" finding
//  5. TestPVAnalysis_StatefulSet_NoBackupAnnotation     — happy   StatefulSet PVC without backup annotation → "no-volume-snapshot" finding
//  6. TestPVAnalysis_StatefulSet_VeleroAnnotation_OK    — happy   StatefulSet PVC with velero backup annotation → no "no-volume-snapshot"
//  7. TestPVAnalysis_OrphanedPVC                        — happy   PVC not mounted by any workload → "orphaned-pvc" finding
//  8. TestPVAnalysis_MultipleFindings_OnOnePVC          — happy   PVC with both missing-storage-class and rwx-misuse → 2 findings
//  9. TestPVAnalysis_EmptyGraph                         — edge    empty graph → TotalFindings=0
// 10. TestPVAnalysis_CheckRWXMisuse_False_Respected     — edge    CheckRWXMisuse=false → no rwx-misuse findings even if present
// 11. TestPVAnalysis_NamespaceCollision_Distinct        — edge    same PVC name in two namespaces → treated as distinct findings
// 12. TestGeneratePVNotes_CriticalBeforeWarning         — integration GeneratePVNotes → CRITICAL section appears before WARNING section
// 13. TestInjectPVNotes_Idempotent                      — integration second inject does not duplicate NOTES content
// 14. TestPVAnalysis_RetainPolicy_OnEphemeral           — happy   PVC marked ephemeral with Retain policy → "retain-policy-on-ephemeral" finding
// ============================================================

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// makeTestGraphWithPVC creates a ResourceGraph containing a single PVC resource.
// accessMode: e.g., "ReadWriteMany", "ReadWriteOnce", "ReadOnlyMany"
// storageClass: "" means the field is absent (not set); use the sentinel "OMIT" to
// leave the key entirely absent from the spec, or "" to set an empty string value.
func makeTestGraphWithPVC(name, namespace, accessMode, storageClass string) *types.ResourceGraph {
	spec := map[string]interface{}{}

	if accessMode != "" {
		spec["accessModes"] = []interface{}{accessMode}
	}

	// Always set resources.
	spec["resources"] = map[string]interface{}{
		"requests": map[string]interface{}{
			"storage": "5Gi",
		},
	}

	// Only set storageClassName when caller wants it present (even if empty).
	if storageClass != "OMIT" {
		spec["storageClassName"] = storageClass
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}

	r := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvkForKind("PersistentVolumeClaim"),
		},
		ServiceName: name,
		Values:      make(map[string]interface{}),
	}

	graph := types.NewResourceGraph()
	graph.AddResource(r)
	return graph
}

// addWorkloadOwner attaches a workload resource to the given graph that mounts pvcName.
// kind: "Deployment", "StatefulSet", etc.
func addWorkloadOwner(graph *types.ResourceGraph, kind, workloadName, namespace, pvcName string) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": gvkForKind(kind).GroupVersion().String(),
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      workloadName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "data",
								"persistentVolumeClaim": map[string]interface{}{
									"claimName": pvcName,
								},
							},
						},
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "app",
								"image": workloadName + ":latest",
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      "data",
										"mountPath": "/data",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	r := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvkForKind(kind),
		},
		ServiceName: workloadName,
		Values:      make(map[string]interface{}),
	}
	graph.AddResource(r)

	// Add PVC ownership relationship.
	workloadKey := r.Original.ResourceKey()
	pvcObj := &unstructured.Unstructured{}
	pvcObj.SetName(pvcName)
	pvcObj.SetNamespace(namespace)
	pvcKey := types.ResourceKey{
		GVK:       gvkForKind("PersistentVolumeClaim"),
		Namespace: namespace,
		Name:      pvcName,
	}
	graph.AddRelationship(types.Relationship{
		From: workloadKey,
		To:   pvcKey,
		Type: types.RelationPVC,
	})
}

// defaultPVAnalysisOptions returns PVAnalysisOptions with all checks enabled.
func defaultPVAnalysisOptions() PVAnalysisOptions {
	return PVAnalysisOptions{
		CheckRWXMisuse:          true,
		CheckMissingStorageClass: true,
		CheckVolumeSnapshot:     true,
		CheckOrphanedPVC:        true,
		CheckRetainPolicy:       true,
		SharedWorkloadKinds:     []string{"StatefulSet"},
	}
}

// ─── Section 1: RWX on Deployment → misuse ───────────────────────────────────

func TestPVAnalysis_RWX_OnDeployment_Misuse(t *testing.T) {
	graph := makeTestGraphWithPVC("data-pvc", "default", "ReadWriteMany", "standard")
	addWorkloadOwner(graph, "Deployment", "web", "default", "data-pvc")

	opts := defaultPVAnalysisOptions()
	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}

	count := report.FindingsByIssue[PVIssueRWXMisuse]
	if count == 0 {
		t.Errorf("expected 'rwx-misuse' finding for RWX PVC on Deployment, got FindingsByIssue=%v", report.FindingsByIssue)
	}
}

// ─── Section 2: RWX on StatefulSet → OK ──────────────────────────────────────

func TestPVAnalysis_RWX_OnStatefulSet_OK(t *testing.T) {
	graph := makeTestGraphWithPVC("data-pvc", "default", "ReadWriteMany", "standard")
	addWorkloadOwner(graph, "StatefulSet", "db", "default", "data-pvc")

	// StatefulSet is in SharedWorkloadKinds, so RWX is allowed.
	opts := defaultPVAnalysisOptions()
	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}

	count := report.FindingsByIssue[PVIssueRWXMisuse]
	if count > 0 {
		t.Errorf("expected no 'rwx-misuse' finding for RWX PVC on StatefulSet, got count=%d", count)
	}
}

// ─── Section 3: No storageClassName → finding ────────────────────────────────

func TestPVAnalysis_NoStorageClassName_Finding(t *testing.T) {
	// "OMIT" sentinel: the storageClassName key is not present in the spec at all.
	graph := makeTestGraphWithPVC("data-pvc", "default", "ReadWriteOnce", "OMIT")

	opts := defaultPVAnalysisOptions()
	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}

	count := report.FindingsByIssue[PVIssueMissingStorageClass]
	if count == 0 {
		t.Errorf("expected 'missing-storage-class' finding when storageClassName key is absent, got FindingsByIssue=%v", report.FindingsByIssue)
	}
}

// ─── Section 4: Empty storageClassName → finding ─────────────────────────────

func TestPVAnalysis_EmptyStorageClassName_Finding(t *testing.T) {
	// storageClassName is present but empty.
	graph := makeTestGraphWithPVC("data-pvc", "default", "ReadWriteOnce", "")

	opts := defaultPVAnalysisOptions()
	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}

	count := report.FindingsByIssue[PVIssueMissingStorageClass]
	if count == 0 {
		t.Errorf("expected 'missing-storage-class' finding when storageClassName is empty string, got FindingsByIssue=%v", report.FindingsByIssue)
	}
}

// ─── Section 5: StatefulSet without backup annotation → no-volume-snapshot ───

func TestPVAnalysis_StatefulSet_NoBackupAnnotation(t *testing.T) {
	graph := makeTestGraphWithPVC("data-pvc", "default", "ReadWriteOnce", "standard")
	addWorkloadOwner(graph, "StatefulSet", "db", "default", "data-pvc")
	// No backup annotations on PVC or StatefulSet.

	opts := defaultPVAnalysisOptions()
	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}

	count := report.FindingsByIssue[PVIssueNoVolumeSnapshot]
	if count == 0 {
		t.Errorf("expected 'no-volume-snapshot' finding for StatefulSet PVC without backup annotation, got FindingsByIssue=%v", report.FindingsByIssue)
	}
}

// ─── Section 6: StatefulSet with velero annotation → no no-volume-snapshot ───

func TestPVAnalysis_StatefulSet_VeleroAnnotation_OK(t *testing.T) {
	// PVC carries velero backup annotation.
	pvcObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"metadata": map[string]interface{}{
				"name":      "data-pvc",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"backup.velero.io/backup-volumes": "data-pvc",
				},
			},
			"spec": map[string]interface{}{
				"accessModes": []interface{}{"ReadWriteOnce"},
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{"storage": "10Gi"},
				},
				"storageClassName": "standard",
			},
		},
	}
	pvcResource := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: pvcObj,
			GVK:    gvkForKind("PersistentVolumeClaim"),
		},
		ServiceName: "data-pvc",
		Values:      make(map[string]interface{}),
	}
	graph := types.NewResourceGraph()
	graph.AddResource(pvcResource)
	addWorkloadOwner(graph, "StatefulSet", "db", "default", "data-pvc")

	opts := defaultPVAnalysisOptions()
	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}

	count := report.FindingsByIssue[PVIssueNoVolumeSnapshot]
	if count > 0 {
		t.Errorf("expected no 'no-volume-snapshot' finding when velero annotation is present, got count=%d", count)
	}
}

// ─── Section 7: Orphaned PVC ──────────────────────────────────────────────────

func TestPVAnalysis_OrphanedPVC(t *testing.T) {
	// PVC exists in graph but no workload mounts it (no relationship added).
	graph := makeTestGraphWithPVC("lonely-pvc", "default", "ReadWriteOnce", "standard")

	opts := defaultPVAnalysisOptions()
	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}

	count := report.FindingsByIssue[PVIssueOrphanedPVC]
	if count == 0 {
		t.Errorf("expected 'orphaned-pvc' finding for PVC with no owner workload, got FindingsByIssue=%v", report.FindingsByIssue)
	}
}

// ─── Section 8: Multiple findings on one PVC ─────────────────────────────────

func TestPVAnalysis_MultipleFindings_OnOnePVC(t *testing.T) {
	// PVC: RWX (misuse on Deployment) + empty storageClass (missing-storage-class).
	graph := makeTestGraphWithPVC("multi-pvc", "default", "ReadWriteMany", "")
	addWorkloadOwner(graph, "Deployment", "web", "default", "multi-pvc")

	opts := defaultPVAnalysisOptions()
	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}
	if report.TotalFindings < 2 {
		t.Errorf("expected at least 2 findings (rwx-misuse + missing-storage-class) for multi-issue PVC, got TotalFindings=%d, FindingsByIssue=%v",
			report.TotalFindings, report.FindingsByIssue)
	}
}

// ─── Section 9: Empty graph ───────────────────────────────────────────────────

func TestPVAnalysis_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := defaultPVAnalysisOptions()

	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil for empty graph")
	}
	if report.TotalFindings != 0 {
		t.Errorf("expected TotalFindings=0 for empty graph, got %d", report.TotalFindings)
	}
	if len(report.Findings) != 0 {
		t.Errorf("expected 0 findings for empty graph, got %d", len(report.Findings))
	}
}

// ─── Section 10: CheckRWXMisuse=false respected ───────────────────────────────

func TestPVAnalysis_CheckRWXMisuse_False_Respected(t *testing.T) {
	graph := makeTestGraphWithPVC("data-pvc", "default", "ReadWriteMany", "standard")
	addWorkloadOwner(graph, "Deployment", "web", "default", "data-pvc")

	opts := defaultPVAnalysisOptions()
	opts.CheckRWXMisuse = false // disable this check

	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}

	count := report.FindingsByIssue[PVIssueRWXMisuse]
	if count > 0 {
		t.Errorf("expected no 'rwx-misuse' findings when CheckRWXMisuse=false, got count=%d", count)
	}
}

// ─── Section 11: Same PVC name in two namespaces → distinct ──────────────────

func TestPVAnalysis_NamespaceCollision_Distinct(t *testing.T) {
	// Two PVCs with the same name but in different namespaces: both orphaned.
	pvcA := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"metadata":   map[string]interface{}{"name": "data-pvc", "namespace": "ns-a"},
			"spec": map[string]interface{}{
				"accessModes": []interface{}{"ReadWriteOnce"},
				"resources":   map[string]interface{}{"requests": map[string]interface{}{"storage": "5Gi"}},
				"storageClassName": "standard",
			},
		},
	}
	pvcB := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"metadata":   map[string]interface{}{"name": "data-pvc", "namespace": "ns-b"},
			"spec": map[string]interface{}{
				"accessModes": []interface{}{"ReadWriteOnce"},
				"resources":   map[string]interface{}{"requests": map[string]interface{}{"storage": "5Gi"}},
				"storageClassName": "standard",
			},
		},
	}

	graph := types.NewResourceGraph()
	for _, obj := range []*unstructured.Unstructured{pvcA, pvcB} {
		graph.AddResource(&types.ProcessedResource{
			Original: &types.ExtractedResource{
				Object: obj,
				GVK:    gvkForKind("PersistentVolumeClaim"),
			},
			Values: make(map[string]interface{}),
		})
	}

	opts := defaultPVAnalysisOptions()
	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}

	orphanCount := report.FindingsByIssue[PVIssueOrphanedPVC]
	if orphanCount < 2 {
		t.Errorf("expected 2 orphaned-pvc findings (one per namespace), got %d", orphanCount)
	}
}

// ─── Section 12: GeneratePVNotes — CRITICAL before WARNING ───────────────────

func TestGeneratePVNotes_CriticalBeforeWarning(t *testing.T) {
	// Create a report with both critical and warning severity findings.
	criticalFinding := PVCFinding{
		PVCName:   "critical-pvc",
		Namespace: "default",
		Issue:     PVIssueRWXMisuse,
		Severity:  PVIssueSeverityCritical,
		Message:   "RWX misuse on Deployment",
	}
	warningFinding := PVCFinding{
		PVCName:   "warn-pvc",
		Namespace: "default",
		Issue:     PVIssueMissingStorageClass,
		Severity:  PVIssueSeverityWarning,
		Message:   "Missing storage class",
	}

	report := &PVAnalysisReport{
		Findings:          []PVCFinding{warningFinding, criticalFinding}, // intentionally reversed order
		TotalFindings:     2,
		FindingsByIssue:   map[PVIssue]int{PVIssueRWXMisuse: 1, PVIssueMissingStorageClass: 1},
		FindingsBySeverity: map[PVIssueSeverity]int{PVIssueSeverityCritical: 1, PVIssueSeverityWarning: 1},
	}

	notes := GeneratePVNotes(report)

	if notes == "" {
		t.Fatal("GeneratePVNotes must return non-empty string when findings exist")
	}

	critIdx := strings.Index(strings.ToUpper(notes), "CRITICAL")
	warnIdx := strings.Index(strings.ToUpper(notes), "WARNING")

	if critIdx == -1 {
		t.Errorf("expected 'CRITICAL' section in PV notes, got:\n%s", notes)
	}
	if warnIdx == -1 {
		t.Errorf("expected 'WARNING' section in PV notes, got:\n%s", notes)
	}
	if critIdx > warnIdx {
		t.Errorf("expected CRITICAL section (pos %d) to appear BEFORE WARNING section (pos %d) in PV notes:\n%s",
			critIdx, warnIdx, notes)
	}
}

// ─── Section 13: InjectPVNotes — idempotent ───────────────────────────────────

func TestInjectPVNotes_Idempotent(t *testing.T) {
	graph := makeTestGraphWithPVC("data-pvc", "default", "ReadWriteMany", "standard")
	addWorkloadOwner(graph, "Deployment", "web", "default", "data-pvc")

	opts := defaultPVAnalysisOptions()
	report := AnalyzePVBestPractices(graph, opts)
	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}

	chart := makeTestChartCost()

	result1, injected1 := InjectPVNotes(chart, report)
	if result1 == nil {
		t.Fatal("InjectPVNotes returned nil on first call")
	}
	if !injected1 {
		t.Error("expected injected=true on first InjectPVNotes call")
	}
	if result1.Notes == "" {
		t.Fatal("expected Notes to be non-empty after first inject")
	}

	result2, _ := InjectPVNotes(result1, report)
	if result2 == nil {
		t.Fatal("InjectPVNotes returned nil on second call")
	}

	count1 := strings.Count(result1.Notes, "PV Analysis")
	count2 := strings.Count(result2.Notes, "PV Analysis")
	if count2 > count1 {
		t.Errorf("second InjectPVNotes duplicated content: count before=%d, after=%d", count1, count2)
	}
}

// ─── Section 14: Retain policy on ephemeral PVC ──────────────────────────────

func TestPVAnalysis_RetainPolicy_OnEphemeral(t *testing.T) {
	// PVC is annotated as ephemeral (or carries annotation indicating transient purpose)
	// but the associated StorageClass has Retain reclaim policy.
	pvcObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"metadata": map[string]interface{}{
				"name":      "tmp-pvc",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"dhg.deckhouse.io/ephemeral": "true",
				},
			},
			"spec": map[string]interface{}{
				"accessModes": []interface{}{"ReadWriteOnce"},
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{"storage": "1Gi"},
				},
				"storageClassName": "retain-class",
			},
		},
	}
	// StorageClass resource with Retain policy.
	scObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion":        "storage.k8s.io/v1",
			"kind":              "StorageClass",
			"metadata":          map[string]interface{}{"name": "retain-class"},
			"reclaimPolicy":     "Retain",
			"provisioner":       "kubernetes.io/no-provisioner",
		},
	}

	graph := types.NewResourceGraph()
	for _, pair := range []struct {
		obj *unstructured.Unstructured
		gvk string
	}{
		{pvcObj, "PersistentVolumeClaim"},
		{scObj, "StorageClass"},
	} {
		kind := pair.gvk
		obj := pair.obj
		r := &types.ProcessedResource{
			Original: &types.ExtractedResource{
				Object: obj,
				GVK:    gvkForKind(kind),
			},
			Values: make(map[string]interface{}),
		}
		graph.AddResource(r)
	}

	opts := defaultPVAnalysisOptions()
	report := AnalyzePVBestPractices(graph, opts)

	if report == nil {
		t.Fatal("AnalyzePVBestPractices must not return nil")
	}

	count := report.FindingsByIssue[PVIssueRetainPolicyOnEphemeral]
	if count == 0 {
		t.Errorf("expected 'retain-policy-on-ephemeral' finding for ephemeral PVC with Retain StorageClass, got FindingsByIssue=%v", report.FindingsByIssue)
	}
}
