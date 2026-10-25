package detector

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/analyzer"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ConnectionDetector extracts connections from workloads and registers them in the graph.
type ConnectionDetector struct{}

// NewConnectionDetector creates a new instance of ConnectionDetector.
func NewConnectionDetector() analyzer.GraphPass {
	return &ConnectionDetector{}
}

// Name returns the name of the detector.
func (d *ConnectionDetector) Name() string {
	return "ConnectionDetector"
}

// Priority returns the execution priority of the detector.
func (d *ConnectionDetector) Priority() int {
	return 100 // High priority
}

func isWorkload(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob", "Pod", "Rollout":
		return true
	}
	return false
}

func extractPodSpec(obj *unstructured.Unstructured) (map[string]interface{}, bool, string) {
	kind := obj.GetKind()
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if !found || err != nil {
		return nil, false, ""
	}
	if kind == "Pod" {
		return spec, true, "spec"
	}
	if kind == "CronJob" {
		podSpec, found, err := unstructured.NestedMap(spec, "jobTemplate", "spec", "template", "spec")
		if !found || err != nil {
			return nil, false, ""
		}
		return podSpec, true, "spec.jobTemplate.spec.template.spec"
	}
	podSpec, found, err := unstructured.NestedMap(spec, "template", "spec")
	if !found || err != nil {
		return nil, false, ""
	}
	return podSpec, true, "spec.template.spec"
}

// simple URL parse fallback for extracting hosts
func simpleParseValue(val string) (protocol, host, path string, port int32, tls bool) {
	if strings.HasPrefix(strings.ToLower(val), "jdbc:") {
		val = val[len("jdbc:"):]
	}
	if strings.Contains(val, "://") {
		u, err := url.Parse(val)
		if err == nil {
			protocol = strings.ToLower(u.Scheme)
			host = u.Hostname()
			p := u.Port()
			if p != "" {
				if n, err := strconv.ParseUint(p, 10, 16); err == nil {
					port = int32(n)
				}
			}
			path = u.Path
			tls = strings.HasSuffix(protocol, "s")
			return
		}
	}
	return
}

func (d *ConnectionDetector) buildServiceIndex(graph map[types.ResourceKey]*types.ProcessedResource) map[string]types.ResourceKey {
	idx := make(map[string]types.ResourceKey)
	for key := range graph {
		if key.GVK.Kind == "Service" {
			idx[key.Name] = key
			if key.Namespace != "" {
				idx[key.Name+"."+key.Namespace] = key
			}
		}
	}
	return idx
}

func (d *ConnectionDetector) Run(graph *types.ResourceGraph) error {
	serviceIdx := d.buildServiceIndex(graph.Resources)
	for _, res := range graph.Resources {
		if res.Original == nil || res.Original.Object == nil {
			continue
		}
		obj := res.Original.Object
		if !isWorkload(obj.GetKind()) {
			continue
		}
		podSpec, found, pathPrefix := extractPodSpec(obj)
		if !found {
			continue
		}
		d.processContainers(podSpec, "initContainers", pathPrefix, res.Original.ResourceKey(), serviceIdx, graph)
		d.processContainers(podSpec, "containers", pathPrefix, res.Original.ResourceKey(), serviceIdx, graph)
	}
	return nil
}

func (d *ConnectionDetector) processContainers(podSpec map[string]interface{}, containerType string, pathPrefix string, fromKey types.ResourceKey, serviceIdx map[string]types.ResourceKey, graph *types.ResourceGraph) {
	containers, found, err := unstructured.NestedSlice(podSpec, containerType)
	if !found || err != nil {
		return
	}

	for i, containerRaw := range containers {
		container, ok := containerRaw.(map[string]interface{})
		if !ok {
			continue
		}

		name, _, _ := unstructured.NestedString(container, "name")
		if name == "" {
			continue
		}

		envFroms, _, _ := unstructured.NestedSlice(container, "envFrom")
		for _, efRaw := range envFroms {
			ef, ok := efRaw.(map[string]interface{})
			if !ok {
				continue
			}

			cmRef, ok1 := ef["configMapRef"].(map[string]interface{})
			secRef, ok2 := ef["secretRef"].(map[string]interface{})

			prefix, _, _ := unstructured.NestedString(ef, "prefix")

			if ok1 {
				if cmName, ok := cmRef["name"].(string); ok {
					cmKey := types.ResourceKey{GVK: schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, Namespace: fromKey.Namespace, Name: cmName}
					if cm, ok := graph.Resources[cmKey]; ok {
						data, _, _ := unstructured.NestedStringMap(cm.Original.Object.Object, "data")
						for k, v := range data {
							d.extractValue(prefix+k, v, name, fmt.Sprintf("%s.containers[%d].envFrom", pathPrefix, i), fromKey, serviceIdx, graph, types.EvidenceConfigMapEnv, cmKey, fmt.Sprintf("data[%s]", k))
						}
					}
				}
			}
			if ok2 {
				if secName, ok := secRef["name"].(string); ok {
					secKey := types.ResourceKey{GVK: schema.GroupVersionKind{Version: "v1", Kind: "Secret"}, Namespace: fromKey.Namespace, Name: secName}
					if sec, ok := graph.Resources[secKey]; ok {
						dataStr, _, _ := unstructured.NestedStringMap(sec.Original.Object.Object, "stringData")
						for k, v := range dataStr {
							d.extractValue(prefix+k, v, name, fmt.Sprintf("%s.containers[%d].envFrom", pathPrefix, i), fromKey, serviceIdx, graph, types.EvidenceSecretEnv, secKey, fmt.Sprintf("stringData[%s]", k))
						}
						dataMap, _, _ := unstructured.NestedMap(sec.Original.Object.Object, "data")
						for k, vInterface := range dataMap {
							if vStr, ok := vInterface.(string); ok {
								decoded, err := base64.StdEncoding.DecodeString(vStr)
								if err == nil {
									d.extractValue(prefix+k, string(decoded), name, fmt.Sprintf("%s.containers[%d].envFrom", pathPrefix, i), fromKey, serviceIdx, graph, types.EvidenceSecretEnv, secKey, fmt.Sprintf("data[%s]", k))
								}
							}
						}
					}
				}
			}
		}

		envs, _, _ := unstructured.NestedSlice(container, "env")
		for _, eRaw := range envs {
			e, ok := eRaw.(map[string]interface{})
			if !ok {
				continue
			}
			nameEnv, _, _ := unstructured.NestedString(e, "name")
			value, found, _ := unstructured.NestedString(e, "value")
			if !found || nameEnv == "" || value == "" {
				continue
			}

			field := fmt.Sprintf("%s.%s[%d].env[%s]", pathPrefix, containerType, i, nameEnv)
			d.extractValue(nameEnv, value, name, field, fromKey, serviceIdx, graph, types.EvidenceEnv, fromKey, field)
		}

		javaOpts, _, _ := unstructured.NestedString(container, "env", "JAVA_OPTS")
		if javaOpts == "" {
			javaOpts, _, _ = unstructured.NestedString(container, "env", "JAVA_TOOL_OPTIONS")
		}
		if javaOpts != "" {
			opts := strings.Split(javaOpts, " ")
			for _, opt := range opts {
				if strings.HasPrefix(opt, "-D") {
					parts := strings.SplitN(strings.TrimPrefix(opt, "-D"), "=", 2)
					if len(parts) == 2 {
						d.extractValue(parts[0], parts[1], name, fmt.Sprintf("%s.%s[%d].env[JAVA_OPTS]", pathPrefix, containerType, i), fromKey, serviceIdx, graph, types.EvidenceJavaOpts, fromKey, fmt.Sprintf("%s.%s[%d].env[JAVA_OPTS]", pathPrefix, containerType, i))
					}
				}
			}
		}

		command, _, _ := unstructured.NestedStringSlice(container, "command")
		args, _, _ := unstructured.NestedStringSlice(container, "args")
		cmdArgs := append(command, args...)
		for _, arg := range cmdArgs {
			if strings.HasPrefix(arg, "--") && strings.Contains(arg, "=") {
				parts := strings.SplitN(strings.TrimPrefix(arg, "--"), "=", 2)
				if len(parts) == 2 {
					d.extractValue(parts[0], parts[1], name, fmt.Sprintf("%s.%s[%d].args", pathPrefix, containerType, i), fromKey, serviceIdx, graph, types.EvidenceArgs, fromKey, fmt.Sprintf("%s.%s[%d].args", pathPrefix, containerType, i))
				}
			}
		}
	}
}

var kafkaEnvRegexp = regexp.MustCompile(`(?i)(kafka.*bootstrap.*servers)|(spring\.kafka.*)`)
var pgEnvRegexp = regexp.MustCompile(`(?i)(db.*host)|(postgres.*)`)

func (d *ConnectionDetector) extractValue(keyName, value, containerName, fieldPath string, fromKey types.ResourceKey, serviceIdx map[string]types.ResourceKey, graph *types.ResourceGraph, source types.EvidenceSource, objKey types.ResourceKey, objField string) {
	// Redact inline passwords if URL contains user:pass
	if strings.Contains(value, "://") && strings.Contains(value, "@") {
		if u, err := url.Parse(value); err == nil && u.User != nil {
			if _, hasPass := u.User.Password(); hasPass {
				fmt.Printf("Note: inline password detected in URL %s, please use Secrets\n", keyName)
				u.User = url.User(u.User.Username())
				value = u.String()
			}
		}
	}

	conf := types.ConfidenceLow
	var relType types.RelationshipType
	var target types.ConnectionTarget

	proto, host, path, port, tls := simpleParseValue(value)

	if host == "" {
		if strings.Contains(value, ":") && !strings.Contains(value, "://") && !strings.Contains(value, " ") {
			parts := strings.Split(value, ":")
			if len(parts) == 2 {
				host = parts[0]
				if n, err := strconv.ParseUint(parts[1], 10, 16); err == nil {
					port = int32(n)
				}
			}
		} else {
			if kafkaEnvRegexp.MatchString(keyName) {
				host = value
				proto = "kafka"
				port = 9092
				conf = types.ConfidenceHigh
			} else if pgEnvRegexp.MatchString(keyName) {
				host = value
				proto = "postgresql"
				port = 5432
				conf = types.ConfidenceHigh
			}
		}

		upperKey := strings.ToUpper(keyName)
		hasSuffix := false
		for _, s := range []string{"_URL", "_URI", "_ENDPOINT", "_ADDR", "_ADDRESS", "_HOST"} {
			if strings.HasSuffix(upperKey, s) || strings.HasPrefix(upperKey, "API_") {
				hasSuffix = true
				break
			}
		}

		if hasSuffix && host == "" {
			if !strings.Contains(value, " ") && strings.Contains(value, ".") && !strings.Contains(value, "redis") {
				host = value
				if port == 0 && !strings.Contains(strings.ToLower(keyName), "smtp") {
					port = 443
					proto = "https"
				} else if port == 0 && strings.Contains(strings.ToLower(keyName), "smtp") {
					port = 587
					proto = "tcp"
				}
			}
		}

		if strings.Contains(strings.ToUpper(keyName), "TOPIC") {
			host = value
			proto = "kafka"
			conf = types.ConfidenceHigh
		}
	}

	if host == "" {
		return
	}

	if svcKey, found := serviceIdx[host]; found {
		target = types.ConnectionTarget{Resource: &svcKey}
		relType = types.RelationServiceCall
		conf = types.ConfidenceHigh
	} else {
		if proto == "kafka" || strings.Contains(keyName, "KAFKA") {
			if strings.Contains(keyName, "TOPIC") {
				nodeID := types.NodeID(fmt.Sprintf("topic::%s", value))
				graph.AddNode(types.LogicalNode{ID: nodeID, Kind: types.NodeTopic, Name: value, Protocol: "kafka"})
				target = types.ConnectionTarget{Node: nodeID}
				relType = types.RelationTopicUse

				if strings.Contains(strings.ToLower(keyName), "produce") {
					relType = types.RelationTopicProduce
				} else if strings.Contains(strings.ToLower(keyName), "consume") {
					relType = types.RelationTopicConsume
				}
				conf = types.ConfidenceHigh
			} else {
				nodeID := types.NodeID(fmt.Sprintf("cluster-service:kafka/%s", host))
				graph.AddNode(types.LogicalNode{ID: nodeID, Kind: types.NodeClusterService, Name: host, Namespace: "kafka"})
				target = types.ConnectionTarget{Node: nodeID}
				relType = types.RelationServiceCall
				if conf == types.ConfidenceLow {
					conf = types.ConfidenceHigh
				}
			}
		} else {
			nodeID := types.NodeID(fmt.Sprintf("external-host:%s", host))
			graph.AddNode(types.LogicalNode{ID: nodeID, Kind: types.NodeExternalHost, Name: host})
			target = types.ConnectionTarget{Node: nodeID}
			relType = types.RelationExternalCall
			if conf == types.ConfidenceLow {
				conf = types.ConfidenceMedium
			}
		}
	}

	if proto == "" {
		proto = "tcp"
	}

	redactedValue := value
	if source == types.EvidenceSecretEnv {
		redactedValue = ""
	}

	c := types.Connection{
		From:       fromKey,
		Container:  containerName,
		Type:       relType,
		To:         target,
		Protocol:   proto,
		TLS:        tls,
		Port:       port,
		Path:       path,
		Confidence: conf,
		Evidence: []types.Evidence{
			{
				Source: source,
				Object: objKey,
				Field:  objField,
				Key:    keyName,
				Value:  redactedValue,
			},
		},
	}

	if proto == "kafka" {
		c.Role = "broker"
	} else if proto == "postgresql" || proto == "mysql" || proto == "jdbc" {
		c.Role = "database"
	} else if strings.Contains(keyName, "ISSUER") || strings.Contains(keyName, "OIDC") {
		c.Role = "oidc-issuer"
	}

	graph.AddConnection(c)

	if c.To.Resource != nil {
		graph.AddRelationship(types.Relationship{
			From:  c.From,
			To:    *c.To.Resource,
			Type:  types.RelationEnvValueFrom,
			Field: fieldPath,
		})
	}
}
