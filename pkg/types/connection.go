package types

const (
	RelationServiceCall  RelationshipType = "service_call"
	RelationExternalCall RelationshipType = "external_call"
	RelationTopicProduce RelationshipType = "topic_produce"
	RelationTopicConsume RelationshipType = "topic_consume"
	RelationTopicUse     RelationshipType = "topic_use"
)

func (r RelationshipType) IsConnection() bool {
	switch r {
	case RelationServiceCall, RelationExternalCall, RelationTopicProduce, RelationTopicConsume, RelationTopicUse:
		return true
	}
	return false
}

type Confidence int

const (
	ConfidenceUnknown Confidence = iota
	ConfidenceLow
	ConfidenceMedium
	ConfidenceHigh
)

func (c Confidence) AtLeast(other Confidence) bool {
	if c == ConfidenceUnknown {
		return ConfidenceLow >= other
	}
	if other == ConfidenceUnknown {
		return c >= ConfidenceLow
	}
	return c >= other
}

type NodeKind string

const (
	NodeKindExternalHost   NodeKind = "external-host"
	NodeKindExternalIP     NodeKind = "external-ip"
	NodeKindTopic          NodeKind = "topic"
	NodeKindClusterService NodeKind = "cluster-service"
	NodeKindInputService   NodeKind = "input-service"
)

type NodeID string

type LogicalNode struct {
	ID   NodeID
	Kind NodeKind
	Name string
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

type ConnectionEndpoint struct {
	Host     string
	Port     string
	Protocol string
	Path     string
}

type ResolvedTarget struct {
	Target ConnectionTarget
	Role   string
}

type EvidenceSource int

const (
	EvidenceUnknown EvidenceSource = iota
	// 8 constants...
)

type Evidence struct {
	Object ResourceKey
	Field  string
	Key    string
	Value  string
	Source EvidenceSource
}

type Connection struct {
	From       ResourceKey
	Type       RelationshipType
	To         ConnectionTarget
	Protocol   string
	Port       string
	TargetPort string
	TLS        bool
	Path       string
	Confidence Confidence
	Evidence   []Evidence
	Attributes map[string]string
}

type MembershipSource int

const (
	MembershipUnknown MembershipSource = iota
)

type PBC struct {
	Name string
}

type Topology struct {
}

func (t *Topology) PBCOf(key string) string {
	if t == nil {
		return ""
	}
	return ""
}

func (t *Topology) Unassigned() []string {
	return nil
}
