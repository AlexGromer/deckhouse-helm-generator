package connect

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"net"
	"strings"
)

// ClusterIndex indexes resources for resolution.
type ClusterIndex struct {
	services  map[string]*types.ProcessedResource
	workloads map[types.ResourceKey]*types.ProcessedResource
	podLabels map[types.ResourceKey]map[string]string

	// Fast lookup mappings
	svcExternalNames map[string]string
	svcClusterIPs    map[string]*types.ProcessedResource
	svcNames         map[string][]*types.ProcessedResource

	ingressHosts map[string]*types.ProcessedResource
	kafkaTopics  map[string]*types.ProcessedResource // key: cluster/namespace/topicName
	kafkaBrokers map[string]*types.ProcessedResource // keys: cluster-kafka-bootstrap, cluster-kafka-brokers
}

// NewIndex creates a new ClusterIndex from a list of processed resources.
func NewIndex(resources []*types.ProcessedResource) *ClusterIndex {
	idx := &ClusterIndex{
		services:         make(map[string]*types.ProcessedResource),
		workloads:        make(map[types.ResourceKey]*types.ProcessedResource),
		podLabels:        make(map[types.ResourceKey]map[string]string),
		svcExternalNames: make(map[string]string),
		svcClusterIPs:    make(map[string]*types.ProcessedResource),
		svcNames:         make(map[string][]*types.ProcessedResource),
		ingressHosts:     make(map[string]*types.ProcessedResource),
		kafkaTopics:      make(map[string]*types.ProcessedResource),
		kafkaBrokers:     make(map[string]*types.ProcessedResource),
	}

	for _, r := range resources {
		if r == nil || r.Original == nil || r.Original.Object == nil {
			continue
		}

		key := r.Original.ResourceKey()
		kind := key.GVK.Kind

		switch kind {
		case "Service":
			idx.services[key.Namespace+"/"+key.Name] = r
			idx.svcNames[key.Name] = append(idx.svcNames[key.Name], r)

			// Extract clusterIP
			ip, _, _ := unstructured.NestedString(r.Original.Object.Object, "spec", "clusterIP")
			if ip != "" && ip != "None" {
				idx.svcClusterIPs[ip] = r
			}

			// Extract ExternalName
			svcType, _, _ := unstructured.NestedString(r.Original.Object.Object, "spec", "type")
			if svcType == "ExternalName" {
				extName, _, _ := unstructured.NestedString(r.Original.Object.Object, "spec", "externalName")
				if extName != "" {
					idx.svcExternalNames[key.Namespace+"/"+key.Name] = extName
					idx.svcExternalNames[key.Name] = extName
				}
			}

		case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "Rollout":
			idx.workloads[key] = r

			// Extract pod labels for workload
			var podLabels map[string]string
			if kind == "CronJob" {
				labels, _, _ := unstructured.NestedStringMap(r.Original.Object.Object, "spec", "jobTemplate", "spec", "template", "metadata", "labels")
				podLabels = labels
			} else {
				labels, _, _ := unstructured.NestedStringMap(r.Original.Object.Object, "spec", "template", "metadata", "labels")
				podLabels = labels
			}

			if podLabels != nil {
				idx.podLabels[key] = podLabels
			}

		case "Ingress":
			rules, _, _ := unstructured.NestedSlice(r.Original.Object.Object, "spec", "rules")
			for _, rule := range rules {
				if rMap, ok := rule.(map[string]interface{}); ok {
					if host, ok := rMap["host"].(string); ok && host != "" {
						idx.ingressHosts[host] = r
					}
				}
			}

		case "HTTPRoute", "GRPCRoute":
			hostnames, _, _ := unstructured.NestedStringSlice(r.Original.Object.Object, "spec", "hostnames")
			for _, h := range hostnames {
				idx.ingressHosts[h] = r
			}

		case "VirtualService":
			hosts, _, _ := unstructured.NestedStringSlice(r.Original.Object.Object, "spec", "hosts")
			for _, h := range hosts {
				idx.ingressHosts[h] = r
			}

		case "ServiceEntry":
			hosts, _, _ := unstructured.NestedStringSlice(r.Original.Object.Object, "spec", "hosts")
			for _, h := range hosts {
				idx.ingressHosts[h] = r
			}

		case "Kafka":
			idx.kafkaBrokers[key.Name+"-kafka-bootstrap"] = r
			idx.kafkaBrokers[key.Name+"-kafka-brokers"] = r

		case "KafkaTopic":
			topicName, _, _ := unstructured.NestedString(r.Original.Object.Object, "spec", "topicName")
			if topicName == "" {
				topicName = key.Name
			}
			labels := r.Original.Object.GetLabels()
			cluster := labels["strimzi.io/cluster"]

			// Build a unique key combining cluster, namespace, and topicName
			k := cluster + "/" + key.Namespace + "/" + topicName
			idx.kafkaTopics[k] = r
		}
	}

	return idx
}

// Resolve resolves a connection endpoint to a target resource.
func (idx *ClusterIndex) Resolve(ep types.ConnectionEndpoint, sourceNamespace string) types.ResolvedTarget {
	host := ep.Host

	// 1. IP Literals check
	if net.ParseIP(host) != nil {
		if r, ok := idx.svcClusterIPs[host]; ok {
			key := r.Original.ResourceKey()
			return types.ResolvedTarget{
				Target: types.ConnectionTarget{Resource: &key},
				Role:   "service",
			}
		}
		// If it's an IP but not a known cluster IP, it's an external IP
		return types.ResolvedTarget{
			Target: types.ConnectionTarget{Node: types.NodeID("external-ip:" + host)},
			Role:   "external",
		}
	}

	// 2. Ingress & Route hostnames
	if r, ok := idx.ingressHosts[host]; ok {
		key := r.Original.ResourceKey()
		return types.ResolvedTarget{
			Target: types.ConnectionTarget{Resource: &key},
			Role:   "ingress",
		}
	}

	// 3. FQDN matching: <svc>.<ns>.svc.cluster.local or <svc>.<ns>.svc
	if strings.HasSuffix(host, ".svc.cluster.local") || strings.HasSuffix(host, ".svc") {
		cleanHost := strings.TrimSuffix(host, ".cluster.local") // make it just <...>.svc if it had .cluster.local
		parts := strings.Split(cleanHost, ".")                  // split on dots

		if len(parts) >= 3 && parts[len(parts)-1] == "svc" {
			// Check for <pod>.<headless>.<ns>.svc
			if len(parts) >= 4 {
				svcName := parts[len(parts)-3]
				ns := parts[len(parts)-2]
				if r, ok := idx.services[ns+"/"+svcName]; ok {
					key := r.Original.ResourceKey()
					return types.ResolvedTarget{
						Target: types.ConnectionTarget{Resource: &key},
						Role:   "service",
					}
				}
			}

			// Normal <svc>.<ns>.svc case
			svcName := parts[0]
			ns := parts[1]

			if r, ok := idx.services[ns+"/"+svcName]; ok {
				key := r.Original.ResourceKey()
				return types.ResolvedTarget{
					Target: types.ConnectionTarget{Resource: &key},
					Role:   "service",
				}
			}

			// If not found as service, might be external or unindexed
			return types.ResolvedTarget{
				Target: types.ConnectionTarget{Node: types.NodeID("cluster-service:" + host)},
				Role:   "unknown-service",
			}
		}
	}

	// 4. `<svc>.<ns>` matching
	if strings.Contains(host, ".") && len(strings.Split(host, ".")) == 2 {
		parts := strings.Split(host, ".")
		svcName := parts[0]
		ns := parts[1]
		if r, ok := idx.services[ns+"/"+svcName]; ok {
			key := r.Original.ResourceKey()
			return types.ResolvedTarget{
				Target: types.ConnectionTarget{Resource: &key},
				Role:   "service",
			}
		}
	}

	// 5. Short names in sourceNamespace
	if r, ok := idx.services[sourceNamespace+"/"+host]; ok {
		key := r.Original.ResourceKey()
		return types.ResolvedTarget{
			Target: types.ConnectionTarget{Resource: &key},
			Role:   "service",
		}
	}

	// 6. Short names across all (if single namespace or unambiguous)
	if svcs, ok := idx.svcNames[host]; ok && len(svcs) == 1 {
		key := svcs[0].Original.ResourceKey()
		return types.ResolvedTarget{
			Target: types.ConnectionTarget{Resource: &key},
			Role:   "service",
		}
	}

	// 7. ExternalName redirection check
	if ext, ok := idx.svcExternalNames[sourceNamespace+"/"+host]; ok {
		return idx.Resolve(types.ConnectionEndpoint{Host: ext, Port: ep.Port, Protocol: ep.Protocol}, sourceNamespace)
	}
	if ext, ok := idx.svcExternalNames[host]; ok {
		return idx.Resolve(types.ConnectionEndpoint{Host: ext, Port: ep.Port, Protocol: ep.Protocol}, sourceNamespace)
	}

	// 8. Kafka bootstrap matching
	// The host might end with -kafka-bootstrap or -kafka-brokers, possibly followed by .namespace.svc...
	baseHost := strings.Split(host, ".")[0]
	if r, ok := idx.kafkaBrokers[baseHost]; ok {
		key := r.Original.ResourceKey()
		return types.ResolvedTarget{
			Target: types.ConnectionTarget{Resource: &key},
			Role:   "broker",
		}
	}

	// Default: Treat as external host
	return types.ResolvedTarget{
		Target: types.ConnectionTarget{Node: types.NodeID("external-host:" + host)},
		Role:   "external",
	}
}

// SelectsWorkload finds workloads selected by a service's pod selector.
func (idx *ClusterIndex) SelectsWorkload(svc types.ResourceKey) []types.ResourceKey {
	var selected []types.ResourceKey

	r, ok := idx.services[svc.Namespace+"/"+svc.Name]
	if !ok {
		return selected
	}

	selector, _, _ := unstructured.NestedStringMap(r.Original.Object.Object, "spec", "selector")
	if len(selector) == 0 {
		return selected
	}

	// Match against workloads in the same namespace
	for wlKey, labels := range idx.podLabels {
		if wlKey.Namespace != svc.Namespace {
			continue
		}

		match := true
		for k, v := range selector {
			if labels[k] != v {
				match = false
				break
			}
		}
		if match {
			selected = append(selected, wlKey)
		}
	}

	return selected
}

// KafkaTopic finds a KafkaTopic by name.
func (idx *ClusterIndex) KafkaTopic(cluster, topicName, namespace string) (types.ResourceKey, bool) {
	k := cluster + "/" + namespace + "/" + topicName
	if r, ok := idx.kafkaTopics[k]; ok {
		return r.Original.ResourceKey(), true
	}

	// Fallback to match by topic name across all clusters/namespaces if unambiguous
	var found *types.ProcessedResource
	count := 0
	for key, r := range idx.kafkaTopics {
		parts := strings.Split(key, "/")
		if len(parts) == 3 && parts[2] == topicName {
			if cluster != "" && parts[0] != cluster {
				continue
			}
			if namespace != "" && parts[1] != namespace {
				continue
			}
			found = r
			count++
		}
	}

	if count == 1 {
		return found.Original.ResourceKey(), true
	}

	return types.ResourceKey{}, false
}
