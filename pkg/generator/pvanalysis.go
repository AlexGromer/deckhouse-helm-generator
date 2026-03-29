package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// PVIssue identifies a category of PV/PVC best-practice finding.
type PVIssue string

const (
	PVIssueRWXMisuse              PVIssue = "rwx-misuse"
	PVIssueMissingStorageClass    PVIssue = "missing-storage-class"
	PVIssueNoVolumeSnapshot       PVIssue = "no-volume-snapshot"
	PVIssueOrphanedPVC            PVIssue = "orphaned-pvc"
	PVIssueRetainPolicyOnEphemeral PVIssue = "retain-policy-on-ephemeral"
)

// PVIssueSeverity classifies the severity of a PV finding.
type PVIssueSeverity string

const (
	PVIssueSeverityCritical PVIssueSeverity = "critical"
	PVIssueSeverityWarning  PVIssueSeverity = "warning"
	PVIssueSeverityInfo     PVIssueSeverity = "info"
)

// issueSeverity returns the default severity for a given PVIssue type.
var issueSeverity = map[PVIssue]PVIssueSeverity{
	PVIssueRWXMisuse:               PVIssueSeverityCritical,
	PVIssueMissingStorageClass:     PVIssueSeverityWarning,
	PVIssueNoVolumeSnapshot:        PVIssueSeverityWarning,
	PVIssueOrphanedPVC:             PVIssueSeverityWarning,
	PVIssueRetainPolicyOnEphemeral: PVIssueSeverityWarning,
}

// PVCFinding represents a single PVC best-practice finding.
type PVCFinding struct {
	PVCName   string
	Namespace string
	Issue     PVIssue
	Severity  PVIssueSeverity
	Message   string
}

// PVAnalysisReport holds all PVC findings.
type PVAnalysisReport struct {
	Findings            []PVCFinding
	TotalFindings       int
	FindingsByIssue     map[PVIssue]int
	FindingsBySeverity  map[PVIssueSeverity]int
}

// PVAnalysisOptions configures which checks are enabled.
type PVAnalysisOptions struct {
	CheckRWXMisuse           bool
	CheckMissingStorageClass bool
	CheckVolumeSnapshot      bool
	CheckOrphanedPVC         bool
	CheckRetainPolicy        bool
	// SharedWorkloadKinds lists workload kinds for which RWX is considered acceptable.
	SharedWorkloadKinds []string
}

// addFinding is a helper that adds a finding to the report.
func (r *PVAnalysisReport) addFinding(f PVCFinding) {
	r.Findings = append(r.Findings, f)
	r.TotalFindings++
	r.FindingsByIssue[f.Issue]++
	r.FindingsBySeverity[f.Severity]++
}

// isSharedKind returns true if kind is in SharedWorkloadKinds.
func isSharedKind(kind string, sharedKinds []string) bool {
	for _, k := range sharedKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// pvcOwnerKinds inspects the relationship graph to find workload kinds that own a PVC.
// Returns (ownerKinds []string, isOwned bool).
func pvcOwnerKinds(graph *types.ResourceGraph, pvcKey types.ResourceKey) ([]string, bool) {
	rels := graph.GetRelationshipsTo(pvcKey)
	var kinds []string
	for _, rel := range rels {
		if rel.Type == types.RelationPVC {
			// Look up the From resource kind.
			if r, ok := graph.GetResourceByKey(rel.From); ok {
				kinds = append(kinds, r.Original.GVK.Kind)
			} else {
				// ResourceKey encodes kind in GVK.
				kinds = append(kinds, rel.From.GVK.Kind)
			}
		}
	}
	return kinds, len(kinds) > 0
}

// hasBackupAnnotation checks whether the PVC carries any known backup annotation.
func hasBackupAnnotation(annotations map[string]string) bool {
	backupAnnotations := []string{
		"backup.velero.io/backup-volumes",
		"backup.velero.io/backup-volumes-excludes",
		"velero.io/backup",
		"snapshot.storage.kubernetes.io/volume-snapshot-class",
		"dhg.deckhouse.io/backup",
	}
	for _, key := range backupAnnotations {
		if _, ok := annotations[key]; ok {
			return true
		}
	}
	return false
}

// AnalyzePVBestPractices examines all PVCs in the graph and returns a PVAnalysisReport.
func AnalyzePVBestPractices(graph *types.ResourceGraph, opts PVAnalysisOptions) *PVAnalysisReport {
	report := &PVAnalysisReport{
		Findings:           []PVCFinding{},
		FindingsByIssue:    make(map[PVIssue]int),
		FindingsBySeverity: make(map[PVIssueSeverity]int),
	}

	if graph == nil {
		return report
	}

	// Build a map of StorageClass → reclaimPolicy from the graph.
	scReclaimPolicy := make(map[string]string)
	for _, r := range graph.Resources {
		if r.Original.GVK.Kind == "StorageClass" {
			obj := r.Original.Object
			name := obj.GetName()
			policy, _, _ := nestedString(obj.Object, "reclaimPolicy")
			if policy != "" {
				scReclaimPolicy[name] = policy
			}
		}
	}

	// Iterate PVCs.
	for _, r := range graph.Resources {
		if r.Original.GVK.Kind != "PersistentVolumeClaim" {
			continue
		}

		obj := r.Original.Object
		pvcName := obj.GetName()
		namespace := obj.GetNamespace()
		annotations := obj.GetAnnotations()
		pvcKey := r.Original.ResourceKey()

		// ── Check: missing storage class ──────────────────────────────────────
		if opts.CheckMissingStorageClass {
			spec, _ := obj.Object["spec"].(map[string]interface{})
			if spec != nil {
				scVal, scPresent := spec["storageClassName"]
				if !scPresent {
					// Key absent entirely.
					report.addFinding(PVCFinding{
						PVCName:   pvcName,
						Namespace: namespace,
						Issue:     PVIssueMissingStorageClass,
						Severity:  issueSeverity[PVIssueMissingStorageClass],
						Message:   fmt.Sprintf("PVC %s/%s has no storageClassName field", namespace, pvcName),
					})
				} else if scStr, ok := scVal.(string); ok && scStr == "" {
					// Key present but empty.
					report.addFinding(PVCFinding{
						PVCName:   pvcName,
						Namespace: namespace,
						Issue:     PVIssueMissingStorageClass,
						Severity:  issueSeverity[PVIssueMissingStorageClass],
						Message:   fmt.Sprintf("PVC %s/%s has empty storageClassName", namespace, pvcName),
					})
				}
			}
		}

		// ── Determine owner workloads ──────────────────────────────────────────
		ownerKinds, isOwned := pvcOwnerKinds(graph, pvcKey)

		// ── Check: orphaned PVC ───────────────────────────────────────────────
		if opts.CheckOrphanedPVC && !isOwned {
			report.addFinding(PVCFinding{
				PVCName:   pvcName,
				Namespace: namespace,
				Issue:     PVIssueOrphanedPVC,
				Severity:  issueSeverity[PVIssueOrphanedPVC],
				Message:   fmt.Sprintf("PVC %s/%s is not mounted by any workload", namespace, pvcName),
			})
		}

		// ── Check: RWX misuse ─────────────────────────────────────────────────
		if opts.CheckRWXMisuse && isOwned {
			// Get access modes.
			accessModes, _, _ := nestedStringSlice(obj.Object, "spec", "accessModes")
			isRWX := false
			for _, mode := range accessModes {
				if mode == "ReadWriteMany" {
					isRWX = true
					break
				}
			}

			if isRWX {
				// RWX is a misuse unless ALL owners are in SharedWorkloadKinds.
				for _, kind := range ownerKinds {
					if !isSharedKind(kind, opts.SharedWorkloadKinds) {
						report.addFinding(PVCFinding{
							PVCName:   pvcName,
							Namespace: namespace,
							Issue:     PVIssueRWXMisuse,
							Severity:  issueSeverity[PVIssueRWXMisuse],
							Message:   fmt.Sprintf("PVC %s/%s uses ReadWriteMany but is mounted by %s (not in SharedWorkloadKinds)", namespace, pvcName, kind),
						})
						break
					}
				}
			}
		}

		// ── Check: no volume snapshot / backup ───────────────────────────────
		if opts.CheckVolumeSnapshot && isOwned {
			// Only flag for SharedWorkloadKinds (stateful workloads like StatefulSet).
			hasSharedOwner := false
			for _, kind := range ownerKinds {
				if isSharedKind(kind, opts.SharedWorkloadKinds) {
					hasSharedOwner = true
					break
				}
			}
			if hasSharedOwner && !hasBackupAnnotation(annotations) {
				report.addFinding(PVCFinding{
					PVCName:   pvcName,
					Namespace: namespace,
					Issue:     PVIssueNoVolumeSnapshot,
					Severity:  issueSeverity[PVIssueNoVolumeSnapshot],
					Message:   fmt.Sprintf("PVC %s/%s on stateful workload has no backup/snapshot annotation", namespace, pvcName),
				})
			}
		}

		// ── Check: retain policy on ephemeral PVC ────────────────────────────
		if opts.CheckRetainPolicy {
			isEphemeral := annotations["dhg.deckhouse.io/ephemeral"] == "true"
			if isEphemeral {
				scName, _, _ := nestedString(obj.Object, "spec", "storageClassName")
				if scName != "" {
					policy, hasSC := scReclaimPolicy[scName]
					if hasSC && strings.EqualFold(policy, "Retain") {
						report.addFinding(PVCFinding{
							PVCName:   pvcName,
							Namespace: namespace,
							Issue:     PVIssueRetainPolicyOnEphemeral,
							Severity:  issueSeverity[PVIssueRetainPolicyOnEphemeral],
							Message:   fmt.Sprintf("PVC %s/%s is ephemeral but StorageClass %q has Retain reclaim policy", namespace, pvcName, scName),
						})
					}
				}
			}
		}
	}

	return report
}

// nestedString extracts a string from an unstructured map by field path.
func nestedString(obj map[string]interface{}, fields ...string) (string, bool, error) {
	cur := obj
	for i, f := range fields {
		v, ok := cur[f]
		if !ok {
			return "", false, nil
		}
		if i == len(fields)-1 {
			s, ok := v.(string)
			return s, ok, nil
		}
		next, ok := v.(map[string]interface{})
		if !ok {
			return "", false, nil
		}
		cur = next
	}
	return "", false, nil
}

// nestedStringSlice extracts a []string from an unstructured field path.
func nestedStringSlice(obj map[string]interface{}, fields ...string) ([]string, bool, error) {
	cur := obj
	for i, f := range fields {
		v, ok := cur[f]
		if !ok {
			return nil, false, nil
		}
		if i == len(fields)-1 {
			switch sl := v.(type) {
			case []string:
				return sl, true, nil
			case []interface{}:
				result := make([]string, 0, len(sl))
				for _, item := range sl {
					if s, ok := item.(string); ok {
						result = append(result, s)
					}
				}
				return result, true, nil
			}
			return nil, false, nil
		}
		next, ok := v.(map[string]interface{})
		if !ok {
			return nil, false, nil
		}
		cur = next
	}
	return nil, false, nil
}

// GeneratePVNotes returns a human-readable PV analysis report string.
// Critical findings are listed before Warning findings.
func GeneratePVNotes(report *PVAnalysisReport) string {
	if report == nil || len(report.Findings) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## PV Analysis\n\n")
	sb.WriteString(fmt.Sprintf("Total findings: %d\n\n", report.TotalFindings))

	// Separate by severity — CRITICAL first, then WARNING, then INFO.
	sections := []struct {
		label    string
		severity PVIssueSeverity
	}{
		{"CRITICAL", PVIssueSeverityCritical},
		{"WARNING", PVIssueSeverityWarning},
		{"INFO", PVIssueSeverityInfo},
	}

	for _, sec := range sections {
		var sectionFindings []PVCFinding
		for _, f := range report.Findings {
			if f.Severity == sec.severity {
				sectionFindings = append(sectionFindings, f)
			}
		}
		if len(sectionFindings) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s\n", sec.label))
		for _, f := range sectionFindings {
			sb.WriteString(fmt.Sprintf("- [%s] %s/%s: %s\n", f.Issue, f.Namespace, f.PVCName, f.Message))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// InjectPVNotes injects a PV analysis section into the chart's NOTES.txt.
// Idempotent: if the section already exists the chart is returned unchanged.
// Returns a copy of the chart and a boolean indicating whether injection occurred.
func InjectPVNotes(chart *types.GeneratedChart, report *PVAnalysisReport) (*types.GeneratedChart, bool) {
	if chart == nil {
		return nil, false
	}
	if report == nil {
		result := copyChartTemplates(chart)
		return result, false
	}

	const marker = "PV Analysis"
	if strings.Contains(chart.Notes, marker) {
		result := copyChartTemplates(chart)
		return result, false
	}

	notes := GeneratePVNotes(report)
	if notes == "" {
		result := copyChartTemplates(chart)
		return result, false
	}

	result := copyChartTemplates(chart)

	var sb strings.Builder
	if result.Notes != "" {
		sb.WriteString(result.Notes)
		if !strings.HasSuffix(result.Notes, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(notes)

	result.Notes = sb.String()
	return result, true
}
