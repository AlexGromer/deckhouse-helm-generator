package types

import "sort"

// Runtime connections. They are stored in ResourceGraph.Connections, never in
// ResourceGraph.Relationships (structural links between objects).
const (
	// RelationServiceCall: a workload calls a Service — one from the input
	// (To.Resource) or one of the cluster that is not in the input
	// (To.Node of kind NodeClusterService).
	RelationServiceCall RelationshipType = "service_call"
	// RelationExternalCall: a workload calls a host outside the cluster
	// (To.Node of kind NodeExternalHost or NodeExternalIP).
	RelationExternalCall RelationshipType = "external_call"
	// RelationTopicProduce: a workload writes to a broker destination (To.Node of kind NodeTopic).
	RelationTopicProduce RelationshipType = "topic_produce"
	// RelationTopicConsume: a workload reads from a broker destination (To.Node of kind NodeTopic).
	RelationTopicConsume RelationshipType = "topic_consume"
	// RelationTopicUse: the input names the destination but not the direction.
	RelationTopicUse RelationshipType = "topic_use"
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

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// AtLeast reports whether c >= min (high > medium > low). An empty
// Confidence is treated as low.
func (c Confidence) AtLeast(min Confidence) bool {
	val := func(conf Confidence) int {
		switch conf {
		case ConfidenceHigh:
			return 3
		case ConfidenceMedium:
			return 2
		default:
			return 1
		}
	}
	return val(c) >= val(min)
}

type NodeKind string

const (
	NodeExternalHost   NodeKind = "external-host"   // DNS name outside the cluster
	NodeExternalIP     NodeKind = "external-ip"     // IPv4/IPv6 literal
	NodeClusterService NodeKind = "cluster-service" // <svc>.<ns> of the cluster, absent from the input
	NodeTopic          NodeKind = "topic"           // broker destination (Kafka topic, AMQP address)
)

// NodeID is the stable identifier of a logical node:
//
//	external-host:<host>                e.g. external-host:api.bank.example.com
//	external-ip:<ip>                    e.g. external-ip:10.20.0.5
//	cluster-service:<namespace>/<name>  e.g. cluster-service:kafka/my-cluster-kafka-bootstrap
//	topic:<brokerID>/<name>             e.g. topic:cluster:kafka/my-cluster-kafka-bootstrap:9092/shop.orders.created
//
// Hosts are lower case without a trailing dot. brokerID is defined in design §4.7 ("" when unknown).
type NodeID string

type LogicalNode struct {
	ID        NodeID
	Kind      NodeKind
	Name      string       // host, IP, Service name or destination name
	Namespace string       // NodeClusterService only
	Broker    string       // NodeTopic only: brokerID ("" = unknown)
	Protocol  string       // NodeTopic only: "kafka", "amqp" or "" (unknown)
	Resource  *ResourceKey // NodeTopic: the KafkaTopic of the input it matches, if any
}

// ConnectionTarget: exactly one of Resource and Node is set.
type ConnectionTarget struct {
	Resource *ResourceKey // a Service of the input (RelationServiceCall)
	Node     NodeID       // a logical node otherwise
}

// String: Resource.String() or string(Node). Used for sorting and as a map key.
func (t ConnectionTarget) String() string {
	if t.Resource != nil {
		return t.Resource.String()
	}
	return string(t.Node)
}

type EvidenceSource string

const (
	EvidenceEnv          EvidenceSource = "env"              // containers[].env[].value
	EvidenceConfigMapEnv EvidenceSource = "configmap-env"    // env valueFrom / envFrom of a ConfigMap
	EvidenceSecretEnv    EvidenceSource = "secret-env"       // env valueFrom / envFrom of a Secret (value never copied)
	EvidenceConfigFile   EvidenceSource = "config-file"      // property in a mounted ConfigMap file (application.yml, ...)
	EvidenceArgs         EvidenceSource = "args"             // --key=value in command/args
	EvidenceJavaOpts     EvidenceSource = "java-opts"        // -Dkey=value in JAVA_OPTS, JAVA_TOOL_OPTIONS, JDK_JAVA_OPTIONS
	EvidenceAppJSON      EvidenceSource = "application-json" // SPRING_APPLICATION_JSON
	EvidenceSynthesized  EvidenceSource = "synthesized"      // project property read by pkg/synth (task 15)
)

type Evidence struct {
	Source EvidenceSource
	Object ResourceKey // object that holds the value: workload, ConfigMap or Secret
	Field  string      // path inside Object
	Key    string      // env var name or property key
	Value  string      // redacted value; "" for EvidenceSecretEnv
}

type Connection struct {
	From       ResourceKey      // workload: Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, CronJob, Pod
	Container  string           // container or init container name
	Type       RelationshipType // one of the five connection types
	To         ConnectionTarget
	Protocol   string // connect.Proto* ("http", "postgresql", "kafka", ...); "tcp" when unknown; "" for topics of unknown broker
	TLS        bool   // scheme says TLS
	Role       string // connect.Role* service role: "http-api", "database", "cache", "broker", "oidc-issuer", ...
	Port       int32  // port as addressed (Service port for Services); 0 = unknown
	TargetPort string // in-input Service only: resolved pod port, decimal number or port name; "" = unknown
	Path       string // URL path without query ("/api/v1"); "" when none
	Attributes map[string]string
	Confidence Confidence
	Evidence   []Evidence // at least one; sorted by (Object, Field, Key)
}

type MembershipSource string

const (
	MembershipMapping MembershipSource = "mapping" // --pbc
	MembershipLabel   MembershipSource = "label"   // --pbc-label (default app.kubernetes.io/part-of)
)

type PBC struct {
	Name     string           // DNS-1123 label: chart name, values key, directory
	RawName  string           // label value before sanitizing ("" for mapping)
	Source   MembershipSource // how the PBC was formed (mapping wins over label)
	Services []string         // service group names (generator.ServiceGroup.Name), sorted
}

type Topology struct {
	Product     string            // --chart-name
	ServicePBC  map[string]string // service group name -> PBC name ("" for unassigned)
	PBCs        map[string]*PBC   // by name
	Connections []Connection      // cross-PBC only? Note says "Topology is the PBC layer over the service groups"
}

// PBCOf returns the PBC name for the given service group, or "" if unassigned or topology is nil.
func (t *Topology) PBCOf(service string) string {
	if t == nil || t.ServicePBC == nil {
		return ""
	}
	return t.ServicePBC[service]
}

// Unassigned returns the names of service groups that do not belong to any PBC, sorted.
func (t *Topology) Unassigned() []string {
	var res []string
	if t == nil {
		return res
	}
	for s, pbc := range t.ServicePBC {
		if pbc == "" {
			res = append(res, s)
		}
	}
	sort.Strings(res)
	return res
}
