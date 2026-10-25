package types

import (
	"fmt"
	"sort"
	"strings"
)

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

	// relFrom and relTo are adjacency indexes for O(1) relationship lookup.
	relFrom map[ResourceKey][]Relationship
	relTo   map[ResourceKey][]Relationship

	// Connections is a list of all detected connections.
	Connections []Connection
	Nodes       map[NodeID]*LogicalNode
	Topology    *Topology

	connFrom map[ResourceKey][]int
	connTo   map[string][]int
	connKey  map[string]int
}

// NewResourceGraph creates a new empty resource graph.
func NewResourceGraph() *ResourceGraph {
	return &ResourceGraph{
		Resources:     make(map[ResourceKey]*ProcessedResource),
		Relationships: make([]Relationship, 0),
		Groups:        make([]*ResourceGroup, 0),
		Orphans:       make([]*ProcessedResource, 0),
		relFrom:       make(map[ResourceKey][]Relationship),
		relTo:         make(map[ResourceKey][]Relationship),
		// Maps for connections and nodes are created lazily
	}
}

// SortedKeys returns the keys of the graph's resources in ResourceKey.Less
// order: walks that build output must not depend on map order.
func (g *ResourceGraph) SortedKeys() []ResourceKey {
	keys := make([]ResourceKey, 0, len(g.Resources))
	for k := range g.Resources {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })
	return keys
}

// AddResource adds a processed resource to the graph.
func (g *ResourceGraph) AddResource(r *ProcessedResource) {
	key := r.Original.ResourceKey()
	g.Resources[key] = r
}

// AddRelationship adds a relationship to the graph and updates the adjacency indexes.
func (g *ResourceGraph) AddRelationship(rel Relationship) {
	g.Relationships = append(g.Relationships, rel)
	g.relFrom[rel.From] = append(g.relFrom[rel.From], rel)
	g.relTo[rel.To] = append(g.relTo[rel.To], rel)
}

// GetResourceByKey returns a resource by its key.
func (g *ResourceGraph) GetResourceByKey(key ResourceKey) (*ProcessedResource, bool) {
	r, ok := g.Resources[key]
	return r, ok
}

// GetRelationshipsFrom returns all relationships from a given resource in O(1).
func (g *ResourceGraph) GetRelationshipsFrom(key ResourceKey) []Relationship {
	return g.relFrom[key]
}

// GetRelationshipsTo returns all relationships to a given resource in O(1).
func (g *ResourceGraph) GetRelationshipsTo(key ResourceKey) []Relationship {
	return g.relTo[key]
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

// AddConnection appends c, or merges it into an existing connection with the
// same From, Container, Type, To, Protocol, Port and Path: evidence is
// unioned (deduplicated, sorted) and the higher confidence kept. Maps are
// created lazily, so a zero ResourceGraph works.
func (g *ResourceGraph) AddConnection(c Connection) {
	if g.connKey == nil {
		g.connKey = make(map[string]int)
		g.connFrom = make(map[ResourceKey][]int)
		g.connTo = make(map[string][]int)
	}

	keyParts := []string{
		c.From.String(),
		c.Container,
		string(c.Type),
		c.To.String(),
		c.Protocol,
		fmt.Sprintf("%d", c.Port),
		c.Path,
	}
	key := strings.Join(keyParts, "\x00")

	idx, exists := g.connKey[key]
	if !exists {
		idx = len(g.Connections)
		// Copy attributes so we don't share map references
		attrCopy := make(map[string]string)
		for k, v := range c.Attributes {
			attrCopy[k] = v
		}
		c.Attributes = attrCopy
		c.Evidence = deduplicateEvidence(c.Evidence)
		g.Connections = append(g.Connections, c)
		g.connKey[key] = idx
		g.connFrom[c.From] = append(g.connFrom[c.From], idx)
		g.connTo[c.To.String()] = append(g.connTo[c.To.String()], idx)
		return
	}

	// Merge into existing
	existing := &g.Connections[idx]
	existing.Evidence = append(existing.Evidence, c.Evidence...)
	existing.Evidence = deduplicateEvidence(existing.Evidence)

	if c.Confidence.AtLeast(existing.Confidence) {
		existing.Confidence = c.Confidence
	}
	if c.TLS {
		existing.TLS = true
	}
	if existing.TargetPort == "" {
		existing.TargetPort = c.TargetPort
	}
	if len(c.Attributes) > 0 {
		if existing.Attributes == nil {
			existing.Attributes = make(map[string]string)
		}
		for k, v := range c.Attributes {
			if _, ok := existing.Attributes[k]; !ok {
				existing.Attributes[k] = v
			}
		}
	}
}

func deduplicateEvidence(ev []Evidence) []Evidence {
	if len(ev) <= 1 {
		return ev
	}
	sort.Slice(ev, func(i, j int) bool {
		s1, s2 := ev[i].Object.String(), ev[j].Object.String()
		if s1 != s2 {
			return s1 < s2
		}
		if ev[i].Field != ev[j].Field {
			return ev[i].Field < ev[j].Field
		}
		return ev[i].Key < ev[j].Key
	})

	res := make([]Evidence, 0, len(ev))
	res = append(res, ev[0])
	for i := 1; i < len(ev); i++ {
		last := res[len(res)-1]
		curr := ev[i]
		if last.Source == curr.Source && last.Object == curr.Object && last.Field == curr.Field && last.Key == curr.Key && last.Value == curr.Value {
			continue
		}
		res = append(res, curr)
	}
	return res
}

// AddNode stores n unless a node with the same ID exists; returns the stored node.
func (g *ResourceGraph) AddNode(n LogicalNode) *LogicalNode {
	if g.Nodes == nil {
		g.Nodes = make(map[NodeID]*LogicalNode)
	}
	if existing, ok := g.Nodes[n.ID]; ok {
		return existing
	}
	nCopy := n
	g.Nodes[n.ID] = &nCopy
	return &nCopy
}

func (g *ResourceGraph) Node(id NodeID) (*LogicalNode, bool) {
	if g.Nodes == nil {
		return nil, false
	}
	n, ok := g.Nodes[id]
	return n, ok
}

// ConnectionsFrom returns a copy of connections from key in insertion order.
func (g *ResourceGraph) ConnectionsFrom(key ResourceKey) []Connection {
	var res []Connection
	for _, idx := range g.connFrom[key] {
		res = append(res, g.Connections[idx])
	}
	return res
}

// ConnectionsTo returns a copy of connections to t in insertion order.
func (g *ResourceGraph) ConnectionsTo(t ConnectionTarget) []Connection {
	var res []Connection
	for _, idx := range g.connTo[t.String()] {
		res = append(res, g.Connections[idx])
	}
	return res
}
