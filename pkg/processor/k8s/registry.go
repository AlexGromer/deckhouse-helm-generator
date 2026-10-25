package k8s

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
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
	r.Register(NewVPAProcessor())

	// Scheduling
	r.Register(NewPriorityClassProcessor())

	// Resource management
	r.Register(NewLimitRangeProcessor())
	r.Register(NewResourceQuotaProcessor())

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

	// Argo Rollouts
	r.Register(NewRolloutProcessor())

	// cert-manager
	r.Register(NewCertificateProcessor())
	r.Register(NewClusterIssuerProcessor())

	// KEDA
	r.Register(NewScaledObjectProcessor())
	r.Register(NewTriggerAuthenticationProcessor())

	// Gateway API
	r.Register(NewHTTPRouteProcessor())
	r.Register(NewGatewayProcessor())
	r.Register(NewGRPCRouteProcessor())
	r.Register(NewTLSRouteProcessor())
	r.Register(NewListenerSetProcessor())
	r.Register(NewReferenceGrantProcessor())

	// Flagger
	r.Register(NewFlaggerCanaryProcessor())

	// Strimzi
	r.Register(NewKafkaTopicProcessor())
	r.Register(NewKafkaUserProcessor())

	// Monitoring (Prometheus Operator + Grafana)
	r.Register(NewServiceMonitorProcessor())
	r.Register(NewPodMonitorProcessor())
	r.Register(NewPrometheusRuleProcessor())
	r.Register(NewGrafanaDashboardProcessor())

	// Deckhouse CRDs
	r.Register(NewModuleConfigProcessor())
	r.Register(NewPodLoggingConfigProcessor())
	r.Register(NewClusterLoggingConfigProcessor())
	r.Register(NewIngressNginxControllerProcessor())
	r.Register(NewClusterAuthorizationRuleProcessor())
	r.Register(NewNodeGroupProcessor())
	r.Register(NewDexAuthenticatorProcessor())
	r.Register(NewUserProcessor())
	r.Register(NewGroupProcessor())

	// Argo CD
	r.Register(NewArgoCDApplicationProcessor())
	r.Register(NewArgoCDApplicationSetProcessor())

	// Deckhouse cloud InstanceClasses
	r.Register(NewOpenstackInstanceClassProcessor())
	r.Register(NewAWSInstanceClassProcessor())
	r.Register(NewGCPInstanceClassProcessor())
	r.Register(NewAzureInstanceClassProcessor())
	r.Register(NewYandexInstanceClassProcessor())
}
