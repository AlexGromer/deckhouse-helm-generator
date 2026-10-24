package types

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
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
	NodeExternalHost   NodeKind = "external-host"
	NodeExternalIP     NodeKind = "external-ip"
	NodeClusterService NodeKind = "cluster-service"
	NodeTopic          NodeKind = "topic"
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
}

type EvidenceSource string

const (
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

const (
	RelationServiceCall  RelationshipType = "service_call"
	RelationExternalCall RelationshipType = "external_call"
	RelationTopicProduce RelationshipType = "topic_produce"
	RelationTopicConsume RelationshipType = "topic_consume"
	RelationTopicUse     RelationshipType = "topic_use"
)

func (t RelationshipType) IsConnection() bool {
	switch t {
	case RelationServiceCall, RelationExternalCall, RelationTopicProduce, RelationTopicConsume, RelationTopicUse:
		return true
	}
	return false
}

type MembershipSource string

const (
	MembershipMapping MembershipSource = "mapping"
	MembershipLabel   MembershipSource = "label"
)

type PBC struct {
	Name     string
	RawName  string
	Source   MembershipSource
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
	// Sort could be applied here
	return unassigned
}
