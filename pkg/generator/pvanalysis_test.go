package generator

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// makeTestGraphWithPVC creates a ResourceGraph containing a single PVC.
// storageClass "OMIT" leaves storageClassName out of the spec; "" sets it empty.
func makeTestGraphWithPVC(name, namespace, accessMode, storageClass string) *types.ResourceGraph {
	spec := map[string]interface{}{
		"resources": map[string]interface{}{"requests": map[string]interface{}{"storage": "5Gi"}},
	}
	if accessMode != "" {
		spec["accessModes"] = []interface{}{accessMode}
	}
	if storageClass != "OMIT" {
		spec["storageClassName"] = storageClass
	}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PersistentVolumeClaim",
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
		"spec":       spec,
	}}
	graph := types.NewResourceGraph()
	graph.AddResource(&types.ProcessedResource{
		Original: &types.ExtractedResource{Object: obj, GVK: gvkForKind("PersistentVolumeClaim")},
	})
	return graph
}

// addWorkloadOwner adds a workload mounting pvcName, with the RelationPVC
// relationship the analyzer detects for persistentVolumeClaim volumes.
func addWorkloadOwner(graph *types.ResourceGraph, kind, workloadName, namespace, pvcName string, replicas int, strategy string) {
	spec := map[string]interface{}{
		"replicas": float64(replicas), // as decoded from YAML
		"template": map[string]interface{}{"spec": map[string]interface{}{
			"volumes": []interface{}{map[string]interface{}{
				"name":                  "data",
				"persistentVolumeClaim": map[string]interface{}{"claimName": pvcName},
			}},
			"containers": []interface{}{map[string]interface{}{"name": "app", "image": "app"}},
		}},
	}
	if strategy != "" {
		spec["strategy"] = map[string]interface{}{"type": strategy}
	}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": gvkForKind(kind).GroupVersion().String(),
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": workloadName, "namespace": namespace},
		"spec":       spec,
	}}
	r := &types.ProcessedResource{Original: &types.ExtractedResource{Object: obj, GVK: gvkForKind(kind)}}
	graph.AddResource(r)
	graph.AddRelationship(types.Relationship{
		From: r.Original.ResourceKey(),
		To:   types.ResourceKey{GVK: gvkForKind("PersistentVolumeClaim"), Namespace: namespace, Name: pvcName},
		Type: types.RelationPVC,
	})
}

func findingsOf(report *PVAnalysisReport) string {
	var out []string
	for _, f := range report.Findings {
		out = append(out, string(f.Issue))
	}
	return strings.Join(out, ",")
}

func TestPVAnalysis_AccessModes(t *testing.T) {
	cases := []struct {
		name       string
		accessMode string
		owners     func(g *types.ResourceGraph)
		want       string
	}{
		{"RWO on a single-replica Recreate Deployment is fine", "ReadWriteOnce", func(g *types.ResourceGraph) {
			addWorkloadOwner(g, "Deployment", "app", "prod", "data", 1, "Recreate")
		}, ""},
		{"RWO rolled by RollingUpdate", "ReadWriteOnce", func(g *types.ResourceGraph) {
			addWorkloadOwner(g, "Deployment", "app", "prod", "data", 1, "")
		}, "rwo-rolling-update"},
		{"RWO shared by 3 replicas", "ReadWriteOnce", func(g *types.ResourceGraph) {
			addWorkloadOwner(g, "Deployment", "app", "prod", "data", 3, "Recreate")
		}, "rwo-multi-pod"},
		{"RWO mounted by a DaemonSet", "ReadWriteOnce", func(g *types.ResourceGraph) {
			addWorkloadOwner(g, "DaemonSet", "agent", "prod", "data", 0, "")
		}, "rwo-multi-pod"},
		{"RWO mounted by two workloads", "ReadWriteOnce", func(g *types.ResourceGraph) {
			addWorkloadOwner(g, "StatefulSet", "a", "prod", "data", 1, "")
			addWorkloadOwner(g, "StatefulSet", "b", "prod", "data", 1, "")
		}, "rwo-multi-pod"},
		{"RWX shared by 3 replicas is fine", "ReadWriteMany", func(g *types.ResourceGraph) {
			addWorkloadOwner(g, "Deployment", "app", "prod", "data", 3, "")
		}, ""},
		{"orphaned claim", "ReadWriteOnce", func(g *types.ResourceGraph) {}, "orphaned-pvc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			graph := makeTestGraphWithPVC("data", "prod", c.accessMode, "standard")
			c.owners(graph)
			report := AnalyzePVBestPractices(graph, DefaultPVAnalysisOptions())
			if got := findingsOf(report); got != c.want {
				t.Errorf("findings = %q, want %q (%+v)", got, c.want, report.Findings)
			}
		})
	}
}

func TestPVAnalysis_StorageClass(t *testing.T) {
	for sc, want := range map[string]string{"OMIT": "missing-storage-class", "": "missing-storage-class", "fast": ""} {
		graph := makeTestGraphWithPVC("data", "prod", "ReadWriteOnce", sc)
		addWorkloadOwner(graph, "StatefulSet", "db", "prod", "data", 1, "")
		report := AnalyzePVBestPractices(graph, DefaultPVAnalysisOptions())
		if got := findingsOf(report); got != want {
			t.Errorf("storageClassName %q: findings = %q, want %q", sc, got, want)
		}
	}

	// volumeClaimTemplates are checked as well.
	graph := makeTestGraphWithWorkload("StatefulSet", "db", "prod", 1, "", "", "", "")
	for _, r := range graph.Resources {
		r.Original.Object.Object["spec"].(map[string]interface{})["volumeClaimTemplates"] = []interface{}{
			map[string]interface{}{"metadata": map[string]interface{}{"name": "data"}, "spec": map[string]interface{}{}},
		}
	}
	report := AnalyzePVBestPractices(graph, DefaultPVAnalysisOptions())
	if report.Analyzed != 1 || findingsOf(report) != "missing-storage-class" || report.Findings[0].PVCName != "db/data" {
		t.Errorf("volumeClaimTemplate: %+v", report)
	}
}

func TestPVAnalysis_OptionsAndEmpty(t *testing.T) {
	graph := makeTestGraphWithPVC("data", "prod", "ReadWriteOnce", "OMIT")
	if report := AnalyzePVBestPractices(graph, PVAnalysisOptions{}); report.TotalFindings != 0 || report.Analyzed != 1 {
		t.Errorf("disabled checks must not report: %+v", report)
	}
	for _, g := range []*types.ResourceGraph{nil, types.NewResourceGraph()} {
		report := AnalyzePVBestPractices(g, DefaultPVAnalysisOptions())
		if report.TotalFindings != 0 || !strings.Contains(report.Markdown(), "No PersistentVolumeClaims") {
			t.Errorf("empty graph: %+v", report)
		}
	}
}

func TestPVAnalysis_NamespacesAreDistinct(t *testing.T) {
	graph := makeTestGraphWithPVC("data", "a", "ReadWriteOnce", "standard")
	other := makeTestGraphWithPVC("data", "b", "ReadWriteOnce", "standard")
	for _, r := range other.Resources {
		graph.AddResource(r)
	}
	addWorkloadOwner(graph, "Deployment", "app", "a", "data", 1, "Recreate")
	report := AnalyzePVBestPractices(graph, DefaultPVAnalysisOptions())
	if findingsOf(report) != "orphaned-pvc" || report.Findings[0].Namespace != "b" {
		t.Errorf("findings = %+v", report.Findings)
	}
}

func TestPVAnalysisReport_Markdown(t *testing.T) {
	graph := makeTestGraphWithPVC("data", "prod", "ReadWriteOnce", "OMIT")
	addWorkloadOwner(graph, "Deployment", "app", "prod", "data", 2, "")
	md := AnalyzePVBestPractices(graph, DefaultPVAnalysisOptions()).Markdown()
	critical := strings.Index(md, "| CRITICAL | data | rwo-multi-pod")
	warning := strings.Index(md, "| WARNING | data | rwo-rolling-update")
	info := strings.Index(md, "| INFO | data | missing-storage-class")
	if critical < 0 || warning < critical || info < warning {
		t.Errorf("findings must be listed most severe first:\n%s", md)
	}
}
