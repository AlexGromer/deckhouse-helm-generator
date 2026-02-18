package testutil

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NewDeployment creates a test Deployment with sensible defaults
// Options can be applied via functional options pattern
func NewDeployment(name, namespace string, opts ...DeploymentOption) *appsv1.Deployment {
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: "nginx:latest",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 80,
									Protocol:      corev1.ProtocolTCP,
								},
							},
						},
					},
				},
			},
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(deployment)
	}

	return deployment
}

// DeploymentOption is a functional option for configuring test Deployments
type DeploymentOption func(*appsv1.Deployment)

// WithReplicas sets the replica count
func WithReplicas(replicas int32) DeploymentOption {
	return func(d *appsv1.Deployment) {
		d.Spec.Replicas = &replicas
	}
}

// WithImage sets the container image
func WithImage(image string) DeploymentOption {
	return func(d *appsv1.Deployment) {
		if len(d.Spec.Template.Spec.Containers) > 0 {
			d.Spec.Template.Spec.Containers[0].Image = image
		}
	}
}

// WithLabels sets labels on the Deployment
func WithLabels(labels map[string]string) DeploymentOption {
	return func(d *appsv1.Deployment) {
		if d.ObjectMeta.Labels == nil {
			d.ObjectMeta.Labels = make(map[string]string)
		}
		for k, v := range labels {
			d.ObjectMeta.Labels[k] = v
		}
	}
}

// WithPodLabels sets labels on the Pod template
func WithPodLabels(labels map[string]string) DeploymentOption {
	return func(d *appsv1.Deployment) {
		if d.Spec.Template.ObjectMeta.Labels == nil {
			d.Spec.Template.ObjectMeta.Labels = make(map[string]string)
		}
		for k, v := range labels {
			d.Spec.Template.ObjectMeta.Labels[k] = v
		}
	}
}

// WithAnnotations sets annotations on the Deployment
func WithAnnotations(annotations map[string]string) DeploymentOption {
	return func(d *appsv1.Deployment) {
		if d.ObjectMeta.Annotations == nil {
			d.ObjectMeta.Annotations = make(map[string]string)
		}
		for k, v := range annotations {
			d.ObjectMeta.Annotations[k] = v
		}
	}
}

// NewService creates a test Service with sensible defaults
func NewService(name, namespace string, opts ...ServiceOption) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(service)
	}

	return service
}

// ServiceOption is a functional option for configuring test Services
type ServiceOption func(*corev1.Service)

// WithServiceType sets the service type
func WithServiceType(svcType corev1.ServiceType) ServiceOption {
	return func(s *corev1.Service) {
		s.Spec.Type = svcType
	}
}

// WithServicePort adds a port to the service
func WithServicePort(name string, port int32, targetPort int) ServiceOption {
	return func(s *corev1.Service) {
		s.Spec.Ports = append(s.Spec.Ports, corev1.ServicePort{
			Name:       name,
			Port:       port,
			TargetPort: intstr.FromInt(targetPort),
			Protocol:   corev1.ProtocolTCP,
		})
	}
}

// WithSelector sets the service selector
func WithSelector(selector map[string]string) ServiceOption {
	return func(s *corev1.Service) {
		s.Spec.Selector = selector
	}
}

// NewConfigMap creates a test ConfigMap with sensible defaults
func NewConfigMap(name, namespace string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
		Data: data,
	}
}

// NewSecret creates a test Secret with sensible defaults
func NewSecret(name, namespace string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
}

// NewStatefulSet creates a test StatefulSet with sensible defaults
func NewStatefulSet(name, namespace string, opts ...StatefulSetOption) *appsv1.StatefulSet {
	replicas := int32(1)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: name + "-headless",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(sts)
	}

	return sts
}

// StatefulSetOption is a functional option for configuring test StatefulSets
type StatefulSetOption func(*appsv1.StatefulSet)

// WithStatefulSetReplicas sets the replica count for StatefulSet
func WithStatefulSetReplicas(replicas int32) StatefulSetOption {
	return func(sts *appsv1.StatefulSet) {
		sts.Spec.Replicas = &replicas
	}
}
