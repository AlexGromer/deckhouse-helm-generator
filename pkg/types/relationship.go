package types

// RelationshipType represents the type of relationship between resources.
type RelationshipType string

const (
	// RelationLabelSelector indicates a label-based selector relationship.
	// Example: Service selecting Deployment pods by labels.
	RelationLabelSelector RelationshipType = "label_selector"

	// RelationNameReference indicates a direct name reference.
	// Example: Ingress referencing a Service by name.
	RelationNameReference RelationshipType = "name_reference"

	// RelationVolumeMount indicates a volume mount relationship.
	// Example: Deployment mounting a ConfigMap or Secret.
	RelationVolumeMount RelationshipType = "volume_mount"

	// RelationEnvFrom indicates an environment variable reference.
	// Example: Deployment using envFrom to load ConfigMap/Secret.
	RelationEnvFrom RelationshipType = "env_from"

	// RelationEnvValueFrom indicates a single env var from ConfigMap/Secret.
	// Example: Deployment using valueFrom to reference a key.
	RelationEnvValueFrom RelationshipType = "env_value_from"

	// RelationAnnotation indicates an annotation-based relationship.
	// Example: Ingress referencing cert-manager ClusterIssuer.
	RelationAnnotation RelationshipType = "annotation"

	// RelationServiceAccount indicates a ServiceAccount binding.
	// Example: Deployment referencing a ServiceAccount.
	RelationServiceAccount RelationshipType = "service_account"

	// RelationOwnerReference indicates a Kubernetes owner reference.
	// Example: ReplicaSet owned by Deployment.
	RelationOwnerReference RelationshipType = "owner_reference"

	// RelationImagePullSecret indicates an image pull secret reference.
	// Example: Deployment referencing imagePullSecrets.
	RelationImagePullSecret RelationshipType = "image_pull_secret"

	// RelationClusterRoleBinding indicates a ClusterRoleBinding relationship.
	// Example: ClusterRoleBinding binding ClusterRole to ServiceAccount.
	RelationClusterRoleBinding RelationshipType = "cluster_role_binding"

	// RelationRoleBinding indicates a RoleBinding relationship.
	// Example: RoleBinding binding Role to ServiceAccount.
	RelationRoleBinding RelationshipType = "role_binding"

	// RelationPVC indicates a PersistentVolumeClaim reference.
	// Example: Deployment mounting a PVC.
	RelationPVC RelationshipType = "pvc"

	// RelationIngressClass indicates an IngressClass reference.
	// Example: Ingress referencing an IngressClass.
	RelationIngressClass RelationshipType = "ingress_class"

	// RelationServiceMonitor indicates a ServiceMonitor selecting a Service.
	RelationServiceMonitor RelationshipType = "service_monitor"

	// RelationDeckhouse indicates a Deckhouse-specific relationship.
	RelationDeckhouse RelationshipType = "deckhouse"

	// RelationGatewayRoute indicates an HTTPRoute → Gateway relationship.
	RelationGatewayRoute RelationshipType = "gateway_route"

	// RelationScaleTarget indicates a ScaledObject → target Deployment/StatefulSet.
	RelationScaleTarget RelationshipType = "scale_target"

	// RelationStorageClass indicates a StorageClass reference.
	// Example: PVC referencing a StorageClass.
	RelationStorageClass RelationshipType = "storage_class"

	// RelationCustomDependency indicates a custom dependency declared via annotation.
	// Example: Resource with dhg.deckhouse.io/depends-on annotation.
	RelationCustomDependency RelationshipType = "custom_dependency"
)

// Relationship represents a detected relationship between two resources.
type Relationship struct {
	// From is the source resource key.
	From ResourceKey

	// To is the target resource key.
	To ResourceKey

	// Type is the relationship type.
	Type RelationshipType

	// Field indicates which field in the source resource contains the reference.
	Field string

	// Details contains additional information about the relationship.
	Details map[string]string
}

// ResourceGraph represents a graph of resources and their relationships.
type ResourceGraph struct {
	// Resources is a map of resource key to processed resource.
	Resources map[ResourceKey]*ProcessedResource

	// Relationships is a list of all detected relationships.
	Relationships []Relationship

	// Groups is a list of resource groups (services).
	Groups []*ResourceGroup

	// orphans contains resources that couldn't be grouped.
	Orphans []*ProcessedResource
}

// NewResourceGraph creates a new empty resource graph.
func NewResourceGraph() *ResourceGraph {
	return &ResourceGraph{
		Resources:     make(map[ResourceKey]*ProcessedResource),
		Relationships: make([]Relationship, 0),
		Groups:        make([]*ResourceGroup, 0),
		Orphans:       make([]*ProcessedResource, 0),
	}
}

// AddResource adds a processed resource to the graph.
func (g *ResourceGraph) AddResource(r *ProcessedResource) {
	key := r.Original.ResourceKey()
	g.Resources[key] = r
}

// AddRelationship adds a relationship to the graph.
func (g *ResourceGraph) AddRelationship(rel Relationship) {
	g.Relationships = append(g.Relationships, rel)
}

// GetResourceByKey returns a resource by its key.
func (g *ResourceGraph) GetResourceByKey(key ResourceKey) (*ProcessedResource, bool) {
	r, ok := g.Resources[key]
	return r, ok
}

// GetRelationshipsFrom returns all relationships from a given resource.
func (g *ResourceGraph) GetRelationshipsFrom(key ResourceKey) []Relationship {
	var result []Relationship
	for _, rel := range g.Relationships {
		if rel.From == key {
			result = append(result, rel)
		}
	}
	return result
}

// GetRelationshipsTo returns all relationships to a given resource.
func (g *ResourceGraph) GetRelationshipsTo(key ResourceKey) []Relationship {
	var result []Relationship
	for _, rel := range g.Relationships {
		if rel.To == key {
			result = append(result, rel)
		}
	}
	return result
}

// GetResourcesByKind returns all resources of a given kind.
func (g *ResourceGraph) GetResourcesByKind(kind string) []*ProcessedResource {
	var result []*ProcessedResource
	for key, r := range g.Resources {
		if key.GVK.Kind == kind {
			result = append(result, r)
		}
	}
	return result
}

// AddGroup adds a resource group to the graph.
func (g *ResourceGraph) AddGroup(group *ResourceGroup) {
	g.Groups = append(g.Groups, group)
}

// AddOrphan adds an orphan resource to the graph.
func (g *ResourceGraph) AddOrphan(r *ProcessedResource) {
	g.Orphans = append(g.Orphans, r)
}
