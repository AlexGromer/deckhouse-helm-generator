package extractor

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func helmLabels(extra map[string]string) map[string]string {
	l := map[string]string{
		"helm.sh/chart":                "x-0.1.0",
		"app.kubernetes.io/managed-by": "Helm",
		"app.kubernetes.io/version":    "1.2.3",
		"app.kubernetes.io/instance":   "rel",
		"app":                          "web",
	}
	for k, v := range extra {
		l[k] = v
	}
	return l
}

func helmDeployment(labels map[string]string, annotations map[string]string) *unstructured.Unstructured {
	tl := map[string]interface{}{}
	for k, v := range labels {
		tl[k] = v
	}
	obj := liveObject("Deployment", "web", map[string]interface{}{"spec": map[string]interface{}{
		"template": map[string]interface{}{"metadata": map[string]interface{}{"labels": tl}},
	}})
	obj.SetAPIVersion("apps/v1")
	obj.SetLabels(labels)
	obj.SetAnnotations(annotations)
	return obj
}

func TestPrepareClusterObject_HelmLabelsDropped(t *testing.T) {
	obj := helmDeployment(helmLabels(nil), map[string]string{"meta.helm.sh/release-name": "rel", "meta.helm.sh/release-namespace": "app"})
	prepareClusterObject(obj)
	tl, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels")
	for name, got := range map[string]map[string]string{"metadata.labels": obj.GetLabels(), "template labels": tl} {
		if len(got) != 1 || got["app"] != "web" {
			t.Errorf("%s = %v; want only app=web", name, got)
		}
	}
}

func TestPrepareClusterObject_HelmInstanceOfAnotherReleaseKept(t *testing.T) {
	obj := helmDeployment(helmLabels(map[string]string{"app.kubernetes.io/instance": "other"}), map[string]string{"meta.helm.sh/release-name": "rel"})
	prepareClusterObject(obj)
	got := obj.GetLabels()
	if got["app.kubernetes.io/instance"] != "other" || got["app.kubernetes.io/version"] != "1.2.3" || got["helm.sh/chart"] != "" {
		t.Errorf("labels = %v; instance and version stay (not set by this release), chart goes", got)
	}
}

func TestPrepareClusterObject_HelmManagedByLabelOnlyKeepsInstance(t *testing.T) {
	obj := helmDeployment(helmLabels(nil), nil)
	prepareClusterObject(obj)
	got := obj.GetLabels()
	if got["app.kubernetes.io/managed-by"] != "" || got["helm.sh/chart"] != "" || got["app.kubernetes.io/instance"] != "rel" || got["app.kubernetes.io/version"] != "1.2.3" {
		t.Errorf("labels = %v; without release-name only chart and managed-by go", got)
	}
}

func TestPrepareClusterObject_NoHelmMarkersUntouched(t *testing.T) {
	obj := helmDeployment(map[string]string{"helm.sh/chart": "x-0.1.0", "app.kubernetes.io/managed-by": "kustomize", "app": "web"}, nil)
	prepareClusterObject(obj)
	if got := obj.GetLabels(); len(got) != 3 {
		t.Errorf("labels = %v; objects without Helm markers must be untouched", got)
	}
}

func TestPrepareClusterObject_HelmOnlyLabelsRemovesMaps(t *testing.T) {
	obj := helmDeployment(map[string]string{"helm.sh/chart": "x-0.1.0", "app.kubernetes.io/managed-by": "Helm"}, nil)
	prepareClusterObject(obj)
	if _, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "metadata", "labels"); found {
		t.Errorf("empty metadata.labels must be removed, got %v", obj.GetLabels())
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec", "template", "metadata", "labels"); found {
		t.Error("empty template labels must be removed")
	}
}

func TestPrepareClusterObject_HelmCronJobTemplateLabels(t *testing.T) {
	obj := liveObject("CronJob", "nightly", map[string]interface{}{"spec": map[string]interface{}{
		"jobTemplate": map[string]interface{}{"spec": map[string]interface{}{
			"template": map[string]interface{}{"metadata": map[string]interface{}{"labels": map[string]interface{}{
				"helm.sh/chart": "x-0.1.0", "app": "web",
			}}},
		}},
	}})
	obj.SetAPIVersion("batch/v1")
	obj.SetLabels(map[string]string{"app.kubernetes.io/managed-by": "Helm"})
	prepareClusterObject(obj)
	tl, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "jobTemplate", "spec", "template", "metadata", "labels")
	if len(tl) != 1 || tl["app"] != "web" {
		t.Errorf("jobTemplate pod labels = %v; want only app=web", tl)
	}
}

func TestHelmRelease(t *testing.T) {
	if _, ok := helmRelease(helmDeployment(map[string]string{"app": "x"}, nil)); ok {
		t.Error("plain object is not Helm-managed")
	}
	if name, ok := helmRelease(helmDeployment(nil, map[string]string{"meta.helm.sh/release-name": "rel"})); !ok || name != "rel" {
		t.Errorf("got %q %v; want rel true", name, ok)
	}
}
