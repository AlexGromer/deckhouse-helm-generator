package extractor

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func liveObject(kind, name string, fields map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	for k, v := range fields {
		obj.Object[k] = v
	}
	obj.SetAPIVersion("v1")
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace("app")
	return obj
}

func TestMatchesKinds(t *testing.T) {
	tests := []struct {
		kind    string
		opts    Options
		matches bool
	}{
		{"Deployment", Options{}, true},
		{"Deployment", Options{IncludeKinds: []string{"deployment"}}, true},
		{"Service", Options{IncludeKinds: []string{"Deployment"}}, false},
		{"Secret", Options{ExcludeKinds: []string{"secret"}}, false},
		{"Deployment", Options{IncludeKinds: []string{"Deployment"}, ExcludeKinds: []string{"Deployment"}}, false},
	}
	for _, tt := range tests {
		if got := matchesKinds(tt.kind, tt.opts); got != tt.matches {
			t.Errorf("matchesKinds(%s, %+v) = %v", tt.kind, tt.opts, got)
		}
	}
}

func TestPrepareClusterObject_SkipsGeneratedObjects(t *testing.T) {
	owned := liveObject("ReplicaSet", "web-5d8f", nil)
	owned.SetOwnerReferences([]metav1.OwnerReference{{Kind: "Deployment", Name: "web"}})
	for _, obj := range []*unstructured.Unstructured{
		owned,
		liveObject("ConfigMap", "kube-root-ca.crt", nil),
		liveObject("ServiceAccount", "default", nil),
		liveObject("Secret", "builder-token", map[string]interface{}{"type": "kubernetes.io/service-account-token"}),
	} {
		if prepareClusterObject(obj) {
			t.Errorf("%s/%s must be skipped", obj.GetKind(), obj.GetName())
		}
	}
	if !prepareClusterObject(liveObject("Secret", "db", map[string]interface{}{"type": "Opaque"})) {
		t.Error("an Opaque Secret must be kept")
	}
}

func TestPrepareClusterObject_RemovesServerFields(t *testing.T) {
	svc := liveObject("Service", "web", map[string]interface{}{
		"spec":   map[string]interface{}{"clusterIP": "10.0.0.5", "clusterIPs": []interface{}{"10.0.0.5"}, "ports": []interface{}{}},
		"status": map[string]interface{}{"loadBalancer": map[string]interface{}{}},
	})
	svc.SetUID("abc")
	svc.SetResourceVersion("42")
	svc.SetAnnotations(map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": "{}",
		"meta.helm.sh/release-name":                        "old",
	})
	if !prepareClusterObject(svc) {
		t.Fatal("Service must be kept")
	}
	if svc.GetUID() != "" || svc.GetResourceVersion() != "" || svc.GetAnnotations() != nil {
		t.Errorf("server fields left: %v", svc.Object["metadata"])
	}
	if _, found := svc.Object["status"]; found {
		t.Error("status left")
	}
	if _, found, _ := unstructured.NestedString(svc.Object, "spec", "clusterIP"); found {
		t.Error("allocated clusterIP left")
	}

	headless := liveObject("Service", "db", map[string]interface{}{"spec": map[string]interface{}{"clusterIP": "None"}})
	headless.SetAnnotations(map[string]string{"team": "data", "deployment.kubernetes.io/revision": "3"})
	prepareClusterObject(headless)
	if ip, _, _ := unstructured.NestedString(headless.Object, "spec", "clusterIP"); ip != "None" {
		t.Error("headless Service must keep clusterIP: None")
	}
	if a := headless.GetAnnotations(); len(a) != 1 || a["team"] != "data" {
		t.Errorf("user annotations must stay, got %v", a)
	}
}

func TestSecretStrategies(t *testing.T) {
	secret := func() *unstructured.Unstructured {
		return liveObject("Secret", "db", map[string]interface{}{
			"data":       map[string]interface{}{"password": "c2VjcmV0"},
			"stringData": map[string]interface{}{"user": "admin"},
		})
	}
	tests := []struct {
		name     string
		config   ClusterExtractorConfig
		opts     Options
		included bool
		want     string
	}{
		{"default skips", ClusterExtractorConfig{}, Options{}, false, ""},
		{"mask flag", ClusterExtractorConfig{}, Options{ClusterSecrets: "mask"}, true, "REDACTED"},
		{"include flag", ClusterExtractorConfig{}, Options{ClusterSecrets: "include"}, true, "c2VjcmV0"},
		{"config external-secret", ClusterExtractorConfig{IncludeSecrets: true, SecretStrategy: string(SecretStrategyExternalSecret)}, Options{}, true, "EXTERNAL_SECRET_REF"},
		{"config without strategy masks", ClusterExtractorConfig{IncludeSecrets: true}, Options{}, true, "REDACTED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewClusterExtractorWithConfig(tt.config)
			if got := e.includeSecrets(tt.opts); got != tt.included {
				t.Fatalf("includeSecrets = %v", got)
			}
			if !tt.included {
				return
			}
			obj := secret()
			e.applySecretStrategy(obj, tt.opts)
			if got, _, _ := unstructured.NestedString(obj.Object, "data", "password"); got != tt.want {
				t.Errorf("data.password = %q, want %q", got, tt.want)
			}
		})
	}
}
