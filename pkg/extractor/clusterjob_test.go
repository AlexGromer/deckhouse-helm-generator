package extractor

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func liveJob(name string, extra map[string]interface{}) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"selector": map[string]interface{}{"matchLabels": map[string]interface{}{"batch.kubernetes.io/controller-uid": "uid-1"}},
		"template": map[string]interface{}{"metadata": map[string]interface{}{"labels": map[string]interface{}{
			"app":                                "web",
			"controller-uid":                     "uid-1",
			"job-name":                           name,
			"batch.kubernetes.io/controller-uid": "uid-1",
			"batch.kubernetes.io/job-name":       name,
			"app.kubernetes.io/name":             "web",
		}}},
	}
	for k, v := range extra {
		spec[k] = v
	}
	job := liveObject("Job", name, map[string]interface{}{"spec": spec})
	job.SetAPIVersion("batch/v1")
	job.SetLabels(map[string]string{
		"app":                                "web",
		"controller-uid":                     "uid-1",
		"job-name":                           name,
		"batch.kubernetes.io/controller-uid": "uid-1",
		"batch.kubernetes.io/job-name":       name,
	})
	return job
}

func TestPrepareClusterObject_JobDropsControllerLabelsAndSelector(t *testing.T) {
	job := liveJob("migrate", nil)
	if !prepareClusterObject(job) {
		t.Fatal("Job must be kept")
	}
	if got := job.GetLabels(); len(got) != 1 || got["app"] != "web" {
		t.Errorf("metadata.labels = %v; want only app=web", got)
	}
	tl, _, _ := unstructured.NestedStringMap(job.Object, "spec", "template", "metadata", "labels")
	if len(tl) != 2 || tl["app"] != "web" || tl["app.kubernetes.io/name"] != "web" {
		t.Errorf("template labels = %v; want user labels only", tl)
	}
	if _, found, _ := unstructured.NestedMap(job.Object, "spec", "selector"); found {
		t.Error("spec.selector must be removed")
	}
}

func TestPrepareClusterObject_JobManualSelectorKeepsSelector(t *testing.T) {
	job := liveJob("migrate", map[string]interface{}{"manualSelector": true})
	prepareClusterObject(job)
	if _, found, _ := unstructured.NestedMap(job.Object, "spec", "selector"); !found {
		t.Error("spec.selector must stay when manualSelector is true")
	}
	if _, set := job.GetLabels()["controller-uid"]; set {
		t.Error("controller-uid label must still be removed")
	}
}

func TestPrepareClusterObject_JobOnlyGeneratedLabelsRemovesMap(t *testing.T) {
	job := liveJob("migrate", nil)
	job.SetLabels(map[string]string{"controller-uid": "uid-1", "job-name": "migrate"})
	prepareClusterObject(job)
	if _, found, _ := unstructured.NestedFieldNoCopy(job.Object, "metadata", "labels"); found {
		t.Errorf("empty metadata.labels must be removed, got %v", job.GetLabels())
	}
}

func TestPrepareClusterObject_OtherKindsKeepJobNameLabel(t *testing.T) {
	obj := liveObject("ConfigMap", "x", nil)
	obj.SetLabels(map[string]string{"job-name": "keep"})
	prepareClusterObject(obj)
	if obj.GetLabels()["job-name"] != "keep" {
		t.Error("labels of non-Job kinds must stay")
	}
}

func TestFinishedJobState(t *testing.T) {
	cond := func(typ, status string) map[string]interface{} {
		return map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"type": typ, "status": status}}}
	}
	tests := []struct {
		name   string
		kind   string
		status map[string]interface{}
		want   string
	}{
		{"succeeded counter", "Job", map[string]interface{}{"succeeded": int64(1)}, "succeeded"},
		{"succeeded counter as float", "Job", map[string]interface{}{"succeeded": float64(2)}, "succeeded"},
		{"complete condition", "Job", cond("Complete", "True"), "succeeded"},
		{"failed condition", "Job", cond("Failed", "True"), "failed"},
		{"false condition", "Job", cond("Failed", "False"), ""},
		{"running", "Job", map[string]interface{}{"active": int64(1)}, ""},
		{"no status", "Job", nil, ""},
		{"not a job", "Deployment", map[string]interface{}{"succeeded": int64(1)}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := liveObject(tt.kind, "x", nil)
			if tt.status != nil {
				obj.Object["status"] = tt.status
			}
			if got := finishedJobState(obj); got != tt.want {
				t.Errorf("finishedJobState = %q; want %q", got, tt.want)
			}
		})
	}
}
