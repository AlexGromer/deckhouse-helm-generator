package testutil

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestNewDeployment_Defaults(t *testing.T) {
	d := NewDeployment("web", "default")

	if d.Name != "web" {
		t.Errorf("expected name 'web', got '%s'", d.Name)
	}
	if d.Namespace != "default" {
		t.Errorf("expected namespace 'default', got '%s'", d.Namespace)
	}
	if d.Spec.Replicas == nil || *d.Spec.Replicas != 1 {
		t.Error("expected default replicas 1")
	}
	if len(d.Spec.Template.Spec.Containers) != 1 {
		t.Fatal("expected 1 container")
	}
	if d.Spec.Template.Spec.Containers[0].Image != "nginx:latest" {
		t.Error("expected default image nginx:latest")
	}
}

func TestNewDeployment_WithReplicas(t *testing.T) {
	d := NewDeployment("web", "default", WithReplicas(3))
	if *d.Spec.Replicas != 3 {
		t.Errorf("expected 3 replicas, got %d", *d.Spec.Replicas)
	}
}

func TestNewDeployment_WithImage(t *testing.T) {
	d := NewDeployment("web", "default", WithImage("myapp:v2"))
	if d.Spec.Template.Spec.Containers[0].Image != "myapp:v2" {
		t.Errorf("expected image 'myapp:v2', got '%s'", d.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestNewDeployment_WithLabels(t *testing.T) {
	d := NewDeployment("web", "default", WithLabels(map[string]string{"env": "prod"}))
	if d.Labels["env"] != "prod" {
		t.Error("expected label env=prod")
	}
	// Original label should still be there
	if d.Labels["app"] != "web" {
		t.Error("expected default label app=web to remain")
	}
}

func TestNewDeployment_WithPodLabels(t *testing.T) {
	d := NewDeployment("web", "default", WithPodLabels(map[string]string{"version": "v1"}))
	if d.Spec.Template.Labels["version"] != "v1" {
		t.Error("expected pod label version=v1")
	}
}

func TestNewDeployment_WithAnnotations(t *testing.T) {
	d := NewDeployment("web", "default", WithAnnotations(map[string]string{"note": "test"}))
	if d.Annotations["note"] != "test" {
		t.Error("expected annotation note=test")
	}
}

func TestNewService_Defaults(t *testing.T) {
	s := NewService("web", "default")
	if s.Name != "web" {
		t.Errorf("expected name 'web', got '%s'", s.Name)
	}
	if s.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Error("expected default type ClusterIP")
	}
	if len(s.Spec.Ports) != 1 {
		t.Fatal("expected 1 port")
	}
	if s.Spec.Ports[0].Port != 80 {
		t.Error("expected default port 80")
	}
}

func TestNewService_WithServiceType(t *testing.T) {
	s := NewService("web", "default", WithServiceType(corev1.ServiceTypeNodePort))
	if s.Spec.Type != corev1.ServiceTypeNodePort {
		t.Error("expected NodePort type")
	}
}

func TestNewService_WithServicePort(t *testing.T) {
	s := NewService("web", "default", WithServicePort("https", 443, 8443))
	if len(s.Spec.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(s.Spec.Ports))
	}
	if s.Spec.Ports[1].Port != 443 {
		t.Error("expected added port 443")
	}
}

func TestNewService_WithSelector(t *testing.T) {
	s := NewService("web", "default", WithSelector(map[string]string{"tier": "frontend"}))
	if s.Spec.Selector["tier"] != "frontend" {
		t.Error("expected selector tier=frontend")
	}
}

func TestNewConfigMap(t *testing.T) {
	cm := NewConfigMap("config", "default", map[string]string{"key": "value"})
	if cm.Name != "config" {
		t.Errorf("expected name 'config', got '%s'", cm.Name)
	}
	if cm.Data["key"] != "value" {
		t.Error("expected data key=value")
	}
}

func TestNewSecret(t *testing.T) {
	s := NewSecret("creds", "default", map[string][]byte{"pass": []byte("secret")})
	if s.Name != "creds" {
		t.Errorf("expected name 'creds', got '%s'", s.Name)
	}
	if s.Type != corev1.SecretTypeOpaque {
		t.Error("expected type Opaque")
	}
	if string(s.Data["pass"]) != "secret" {
		t.Error("expected secret data")
	}
}

func TestNewStatefulSet_Defaults(t *testing.T) {
	sts := NewStatefulSet("db", "default")
	if sts.Name != "db" {
		t.Errorf("expected name 'db', got '%s'", sts.Name)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 1 {
		t.Error("expected default 1 replica")
	}
	if sts.Spec.ServiceName != "db-headless" {
		t.Errorf("expected service name 'db-headless', got '%s'", sts.Spec.ServiceName)
	}
}

func TestNewStatefulSet_WithReplicas(t *testing.T) {
	sts := NewStatefulSet("db", "default", WithStatefulSetReplicas(3))
	if *sts.Spec.Replicas != 3 {
		t.Errorf("expected 3 replicas, got %d", *sts.Spec.Replicas)
	}
}
