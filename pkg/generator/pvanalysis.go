package generator

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// PVIssue identifies a category of PV/PVC best-practice finding.
type PVIssue string

const (
	// PVIssueRWOMultiPod: a ReadWriteOnce claim is mounted by several pods
	// (replicas > 1, a DaemonSet, a parallel Job, or several workloads). Pods
	// scheduled on another node hang in ContainerCreating (Multi-Attach error).
	PVIssueRWOMultiPod PVIssue = "rwo-multi-pod"
	// PVIssueRWORollingUpdate: a Deployment mounts a ReadWriteOnce claim with
	// the RollingUpdate strategy; the new pod can land on another node and
	// wait forever for the volume held by the old one. Use strategy Recreate.
	PVIssueRWORollingUpdate PVIssue = "rwo-rolling-update"
	// PVIssueMissingStorageClass: no storageClassName, so the cluster default
	// StorageClass is used (or the claim stays Pending if there is none).
	PVIssueMissingStorageClass PVIssue = "missing-storage-class"
	// PVIssueOrphanedPVC: the claim is not mounted by any workload of the input.
	PVIssueOrphanedPVC PVIssue = "orphaned-pvc"
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
	PVIssueRWOMultiPod:         PVIssueSeverityCritical,
	PVIssueRWORollingUpdate:    PVIssueSeverityWarning,
	PVIssueOrphanedPVC:         PVIssueSeverityWarning,
	PVIssueMissingStorageClass: PVIssueSeverityInfo,
}

// PVCFinding represents a single PVC best-practice finding.
type PVCFinding struct {
	// PVCName is the claim name; for a StatefulSet volumeClaimTemplate it is
	// "<statefulset>/<template>".
	PVCName   string
	Namespace string
	Issue     PVIssue
	Severity  PVIssueSeverity
	Message   string
}

// PVAnalysisReport holds all PVC findings.
type PVAnalysisReport struct {
	// Analyzed is the number of claims and volumeClaimTemplates inspected.
	Analyzed           int
	Findings           []PVCFinding
	TotalFindings      int
	FindingsByIssue    map[PVIssue]int
	FindingsBySeverity map[PVIssueSeverity]int
}

// PVAnalysisOptions configures which checks are enabled.
type PVAnalysisOptions struct {
	// CheckAccessModes reports ReadWriteOnce claims shared by several pods
	// and Deployments that roll ReadWriteOnce claims.
	CheckAccessModes         bool
	CheckMissingStorageClass bool
	CheckOrphanedPVC         bool
}

// DefaultPVAnalysisOptions enables every check.
func DefaultPVAnalysisOptions() PVAnalysisOptions {
	return PVAnalysisOptions{CheckAccessModes: true, CheckMissingStorageClass: true, CheckOrphanedPVC: true}
}

func (r *PVAnalysisReport) addFinding(name, namespace string, issue PVIssue, msg string) {
	f := PVCFinding{PVCName: name, Namespace: namespace, Issue: issue, Severity: issueSeverity[issue], Message: msg}
	r.Findings = append(r.Findings, f)
	r.TotalFindings++
	r.FindingsByIssue[f.Issue]++
	r.FindingsBySeverity[f.Severity]++
}

// pvcOwners returns the workloads that mount the claim (via RelationPVC).
// Owners that are not part of the graph are returned with only their key.
func pvcOwners(graph *types.ResourceGraph, pvcKey types.ResourceKey) []types.ResourceKey {
	var owners []types.ResourceKey
	for _, rel := range graph.GetRelationshipsTo(pvcKey) {
		if rel.Type == types.RelationPVC {
			owners = append(owners, rel.From)
		}
	}
	return owners
}

// isReadWriteOnce reports whether a claim spec only allows single-node access.
func isReadWriteOnce(spec map[string]interface{}) bool {
	modes, _, _ := unstructured.NestedStringSlice(spec, "accessModes")
	rwo := false
	for _, m := range modes {
		switch m {
		case "ReadWriteMany", "ReadOnlyMany":
			return false
		case "ReadWriteOnce", "ReadWriteOncePod":
			rwo = true
		}
	}
	return rwo
}

// multiPodReason describes why a workload runs more than one pod at once;
// ok is false when it runs a single pod.
func multiPodReason(r *types.ProcessedResource) (string, bool) {
	obj := r.Original.Object
	switch r.Original.GVK.Kind {
	case "DaemonSet":
		return "DaemonSet " + obj.GetName() + " (one pod per node)", true
	case "Job", "CronJob", "Deployment", "StatefulSet":
		if n := extractReplicasFromResource(obj); n > 1 {
			return fmt.Sprintf("%s %s (%d pods)", r.Original.GVK.Kind, obj.GetName(), n), true
		}
	}
	return "", false
}

// AnalyzePVBestPractices examines all PVCs and StatefulSet volumeClaimTemplates
// in the graph and returns a PVAnalysisReport.
func AnalyzePVBestPractices(graph *types.ResourceGraph, opts PVAnalysisOptions) *PVAnalysisReport {
	report := &PVAnalysisReport{
		Findings:           []PVCFinding{},
		FindingsByIssue:    make(map[PVIssue]int),
		FindingsBySeverity: make(map[PVIssueSeverity]int),
	}
	if graph == nil {
		return report
	}

	for _, r := range opsSortedResources(graph) {
		obj := r.Original.Object
		switch r.Original.GVK.Kind {
		case "StatefulSet":
			for _, vct := range volumeClaimTemplates(r) {
				report.Analyzed++
				name, _, _ := unstructured.NestedString(vct, "metadata", "name")
				spec, _ := vct["spec"].(map[string]interface{})
				if opts.CheckMissingStorageClass && storageClassMissing(spec) {
					report.addFinding(obj.GetName()+"/"+name, obj.GetNamespace(), PVIssueMissingStorageClass,
						fmt.Sprintf("volumeClaimTemplate %s of StatefulSet %s has no storageClassName; the cluster default StorageClass is used",
							name, obj.GetName()))
				}
			}

		case "PersistentVolumeClaim":
			report.Analyzed++
			name, ns := obj.GetName(), obj.GetNamespace()
			spec, _ := obj.Object["spec"].(map[string]interface{})
			if opts.CheckMissingStorageClass && storageClassMissing(spec) {
				report.addFinding(name, ns, PVIssueMissingStorageClass,
					fmt.Sprintf("PVC %s has no storageClassName; the cluster default StorageClass is used", name))
			}

			owners := pvcOwners(graph, r.Original.ResourceKey())
			if opts.CheckOrphanedPVC && len(owners) == 0 {
				report.addFinding(name, ns, PVIssueOrphanedPVC,
					fmt.Sprintf("PVC %s is not mounted by any workload", name))
			}
			if !opts.CheckAccessModes || !isReadWriteOnce(spec) {
				continue
			}

			var multi []string
			for _, key := range owners {
				owner, ok := graph.GetResourceByKey(key)
				if !ok {
					continue
				}
				if reason, ok := multiPodReason(owner); ok {
					multi = append(multi, reason)
				}
				if key.GVK.Kind == "Deployment" {
					strategy, _, _ := unstructured.NestedString(owner.Original.Object.Object, "spec", "strategy", "type")
					if strategy == "" || strategy == "RollingUpdate" {
						report.addFinding(name, ns, PVIssueRWORollingUpdate,
							fmt.Sprintf("Deployment %s mounts ReadWriteOnce PVC %s with the RollingUpdate strategy; "+
								"the new pod can be scheduled on another node and wait for the volume. Use strategy Recreate",
								key.Name, name))
					}
				}
			}
			if len(owners) > 1 {
				multi = append(multi, fmt.Sprintf("%d workloads", len(owners)))
			}
			if len(multi) > 0 {
				report.addFinding(name, ns, PVIssueRWOMultiPod,
					fmt.Sprintf("ReadWriteOnce PVC %s is mounted by %s; pods on other nodes cannot attach it. "+
						"Use ReadWriteMany storage or a StatefulSet volumeClaimTemplate", name, strings.Join(multi, ", ")))
			}
		}
	}
	return report
}

// storageClassMissing reports whether a claim spec has no (or an empty)
// storageClassName.
func storageClassMissing(spec map[string]interface{}) bool {
	sc, ok := spec["storageClassName"]
	if !ok || sc == nil {
		return true
	}
	s, isString := sc.(string)
	return isString && s == ""
}

// Markdown renders the persistent volume findings as a Markdown section,
// most severe first.
func (r *PVAnalysisReport) Markdown() string {
	var b strings.Builder
	b.WriteString("## Persistent volumes\n\n")
	if r == nil || r.Analyzed == 0 {
		b.WriteString("No PersistentVolumeClaims or volumeClaimTemplates.\n")
		return b.String()
	}
	if len(r.Findings) == 0 {
		fmt.Fprintf(&b, "No findings for %d claim(s).\n", r.Analyzed)
		return b.String()
	}
	b.WriteString("| Severity | Claim | Finding |\n")
	b.WriteString("|---|---|---|\n")
	for _, sev := range []PVIssueSeverity{PVIssueSeverityCritical, PVIssueSeverityWarning, PVIssueSeverityInfo} {
		for _, f := range r.Findings {
			if f.Severity == sev {
				fmt.Fprintf(&b, "| %s | %s | %s: %s |\n", strings.ToUpper(string(sev)), f.PVCName, f.Issue, f.Message)
			}
		}
	}
	return b.String()
}
