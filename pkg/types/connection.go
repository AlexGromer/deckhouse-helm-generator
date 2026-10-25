package types

import (
	"fmt"
	"sort"
	"strings"
)

type ConnectionKind string

const (
	ConnectionDatabase   ConnectionKind = "database"
	ConnectionCache      ConnectionKind = "cache"
	ConnectionBroker     ConnectionKind = "broker"
	ConnectionHTTP       ConnectionKind = "http"
	ConnectionGRPC       ConnectionKind = "grpc"
	ConnectionStorage    ConnectionKind = "storage"
	ConnectionDirectory  ConnectionKind = "directory"
	ConnectionSMTP       ConnectionKind = "smtp"
	ConnectionMonitoring ConnectionKind = "monitoring"
	ConnectionTCP        ConnectionKind = "tcp"
	ConnectionUnknown    ConnectionKind = "unknown"
)

const (
	RelationServiceCall  RelationshipType = "service_call"
	RelationExternalCall RelationshipType = "external_call"
	RelationTopicProduce RelationshipType = "topic_produce"
	RelationTopicConsume RelationshipType = "topic_consume"
	RelationTopicUse     RelationshipType = "topic_use"
)

// IsConnection reports whether t is one of the runtime connection types above.
func (t RelationshipType) IsConnection() bool {
	switch t {
	case RelationServiceCall, RelationExternalCall, RelationTopicProduce, RelationTopicConsume, RelationTopicUse:
		return true
	}
	return false
}

type Confidence string

type EvidenceConfidence = Confidence

const (
	ConfidenceUnknown Confidence = ""
	ConfidenceLow     Confidence = "low"
	ConfidenceMedium  Confidence = "medium"
	ConfidenceHigh    Confidence = "high"
)

func (c Confidence) AtLeast(min Confidence) bool {
	if c == "" {
		c = ConfidenceLow
	}
	if min == "" {
		min = ConfidenceLow
	}
	score := map[Confidence]int{ConfidenceLow: 1, ConfidenceMedium: 2, ConfidenceHigh: 3}
	return score[c] >= score[min]
}

type NodeKind string

const (
	NodeExternalHost       NodeKind = "external-host"
	NodeExternalIP         NodeKind = "external-ip"
	NodeClusterService     NodeKind = "cluster-service"
	NodeTopic              NodeKind = "topic"
	NodeKindExternalHost   NodeKind = NodeExternalHost
	NodeKindExternalIP     NodeKind = NodeExternalIP
	NodeKindClusterService NodeKind = NodeClusterService
	NodeKindTopic          NodeKind = NodeTopic
	NodeKindInputService   NodeKind = "input-service"
)

type NodeID string

type LogicalNode struct {
	ID        NodeID
	Kind      NodeKind
	Name      string
	Namespace string
	Broker    string
	Protocol  string
	Resource  *ResourceKey
}

type ServiceNode struct {
	ID   string
	Name string
}

type TransportProtocol = string

type ConnectionEndpoint struct {
	Host     string
	Port     int
	Path     string
	Database string
	Protocol TransportProtocol
	Scheme   string
}

type ConnectionTarget struct {
	Resource *ResourceKey
	Node     NodeID
}

func (t ConnectionTarget) String() string {
	if t.Resource != nil {
		return t.Resource.String()
	}
	return string(t.Node)
}

type ResolvedTarget struct {
	Target    ConnectionTarget
	Role      string
	Name      string
	Namespace string
	Kind      string
}

type EvidenceSource string

const (
	EvidenceUnknown      EvidenceSource = ""
	EvidenceEnv          EvidenceSource = "env"
	EvidenceConfigMapEnv EvidenceSource = "configmap-env"
	EvidenceSecretEnv    EvidenceSource = "secret-env"
	EvidenceConfigFile   EvidenceSource = "config-file"
	EvidenceArgs         EvidenceSource = "args"
	EvidenceJavaOpts     EvidenceSource = "java-opts"
	EvidenceAppJSON      EvidenceSource = "application-json"
	EvidenceSynthesized  EvidenceSource = "synthesized"
)

type Evidence struct {
	Source EvidenceSource
	Object ResourceKey
	Field  string
	Key    string
	Value  string
}

type ConnectionEvidence = Evidence

type Connection struct {
	From       ResourceKey
	Container  string
	Type       RelationshipType
	To         ConnectionTarget
	Protocol   string
	TLS        bool
	Role       string
	Port       int32
	TargetPort string
	Path       string
	Attributes map[string]string
	Confidence Confidence
	Evidence   []Evidence

	Source    ResourceKey
	Target    ResolvedTarget
	Endpoints []ConnectionEndpoint
	Kind      ConnectionKind
}

type DetectedConnection = Connection

func (c Connection) ID() string {
	if c.Source.Name != "" || c.Kind != "" || len(c.Endpoints) > 0 {
		targetID := fmt.Sprintf("%s/%s/%s", c.Target.Kind, c.Target.Namespace, c.Target.Name)
		if c.Target.Kind == "" && c.Target.Name == "" && c.Target.Target.String() != "" {
			targetID = c.Target.Target.String()
		}
		var eps []string
		for _, ep := range c.Endpoints {
			eps = append(eps, fmt.Sprintf("%s://%s:%d%s", ep.Protocol, ep.Host, ep.Port, ep.Path))
		}
		sort.Strings(eps)
		return fmt.Sprintf("%s|%s|%s|%s", c.Source.String(), c.Kind, targetID, strings.Join(eps, ","))
	}
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%s",
		c.From.String(), c.Container, c.Type, c.To.String(), c.Protocol, c.Port, c.Path)
}

type MembershipSource string

const (
	MembershipUnknown MembershipSource = ""
	MembershipMapping MembershipSource = "mapping"
	MembershipLabel   MembershipSource = "label"
)

type PBC struct {
	Name     string
	RawName  string
	Source   MembershipSource
	Services []string
}

type PBCGroup struct {
	Name     string
	Services []string
}

type Topology struct {
	Product     string
	Label       string
	PBCs        []PBC
	ServicePBC  map[string]string
	ResourcePBC map[ResourceKey]string
}

func (t *Topology) PBCOf(key ResourceKey) string {
	if t == nil {
		return ""
	}
	if pbc, ok := t.ResourcePBC[key]; ok {
		return pbc
	}
	return ""
}

func (t *Topology) PBC(name string) (PBC, bool) {
	if t == nil {
		return PBC{}, false
	}
	for _, pbc := range t.PBCs {
		if pbc.Name == name {
			return pbc, true
		}
	}
	return PBC{}, false
}

func (t *Topology) Unassigned() []string {
	if t == nil {
		return nil
	}
	var unassigned []string
	for svc, pbc := range t.ServicePBC {
		if pbc == "" {
			unassigned = append(unassigned, svc)
		}
	}
	sort.Strings(unassigned)
	return unassigned
}

// ServiceTopology represents the grouping of services into PBCs (Packaged Business Capabilities).
type ServiceTopology struct {
	// ServicePBC maps a service group name to its assigned PBC.
	ServicePBC map[string]string

	// PBCToServices maps a PBC name to a list of service group names.
	PBCToServices map[string][]string

	// PlatformServices is a list of services assigned to the "platform" PBC (e.g., shared databases).
	PlatformServices []string

	// Groups holds explicit PBC group entries.
	Groups []PBCGroup
}
