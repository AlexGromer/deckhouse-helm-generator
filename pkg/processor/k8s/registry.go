package k8s

import (
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// RegisterAll registers all standard Kubernetes processors with the registry.
func RegisterAll(r *processor.Registry) {
	// Core workloads
	r.Register(NewDeploymentProcessor())
	r.Register(NewStatefulSetProcessor())
	r.Register(NewDaemonSetProcessor())

	// Services and networking
	r.Register(NewServiceProcessor())
	r.Register(NewIngressProcessor())
	r.Register(NewNetworkPolicyProcessor())

	// Configuration
	r.Register(NewConfigMapProcessor())
	r.Register(NewSecretProcessor())

	// Storage
	r.Register(NewPVCProcessor())

	// Autoscaling
	r.Register(NewHPAProcessor())

	// Disruption budget
	r.Register(NewPDBProcessor())

	// Batch workloads
	r.Register(NewCronJobProcessor())
	r.Register(NewJobProcessor())

	// RBAC and identity
	r.Register(NewServiceAccountProcessor())
	r.Register(NewRoleProcessor())
	r.Register(NewClusterRoleProcessor())
	r.Register(NewRoleBindingProcessor())
	r.Register(NewClusterRoleBindingProcessor())
}
