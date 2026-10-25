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

type TransportProtocol string

type EvidenceConfidence string

const (
	ConfidenceHigh   EvidenceConfidence = "high"
	ConfidenceMedium EvidenceConfidence = "medium"
	ConfidenceLow    EvidenceConfidence = "low"
)

type ConnectionEndpoint struct {
	Host     string
	Port     int
	Path     string
	Database string
	Protocol TransportProtocol
	Scheme   string
}

type ConnectionEvidence struct {
	Source string
	Key    string
	Value  string
}

type ResolvedTarget struct {
	Name      string
	Namespace string
	Kind      string
}

type DetectedConnection struct {
	Source     ResourceKey
	Target     ResolvedTarget
	Endpoints  []ConnectionEndpoint
	Kind       ConnectionKind
	Evidence   []ConnectionEvidence
	Confidence EvidenceConfidence
}

func (c DetectedConnection) ID() string {
	targetID := fmt.Sprintf("%s/%s/%s", c.Target.Kind, c.Target.Namespace, c.Target.Name)

	var eps []string
	for _, ep := range c.Endpoints {
		eps = append(eps, fmt.Sprintf("%s://%s:%d%s", ep.Protocol, ep.Host, ep.Port, ep.Path))
	}
	sort.Strings(eps)

	return fmt.Sprintf("%s|%s|%s|%s", c.Source.String(), c.Kind, targetID, strings.Join(eps, ","))
}

type ServiceNode struct {
	ID   string
	Name string
}

type PBCGroup struct {
	Name     string
	Services []string
}

type ServiceTopology struct {
	Groups []PBCGroup
}
