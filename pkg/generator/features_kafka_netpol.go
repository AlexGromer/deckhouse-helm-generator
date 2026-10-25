package generator

import (
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/kafkaclient"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"sigs.k8s.io/yaml"
)

func init() {
	RegisterFeature(Feature{
		Name:        "kafka-netpol",
		Description: "Generate NetworkPolicy for egress traffic to Kafka brokers",
		Params: map[string]string{
			"broker-namespace": "kafka",
			"broker-pod-label": "strimzi.io/kind=Kafka",
			"ports":            "9092,9093",
		},
		Apply: applyKafkaNetpolFeature,
	})
}

func applyKafkaNetpolFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	// First collect facts from chart workloads using kafkaclient
	facts := kafkaclient.ExtractFromGraph(fc.Graph)
	hasClients := len(facts.Clients) > 0

	// Also check connections for Kafka usage
	if !hasClients {
		for _, conn := range fc.Graph.Connections {
			if conn.Type == types.RelationTopicProduce || conn.Type == types.RelationTopicConsume || conn.Type == types.RelationTopicUse {
				hasClients = true
				break
			}
			if conn.Protocol == "kafka" || conn.Role == "broker" {
				hasClients = true
				break
			}
		}
	}

	if !hasClients {
		return chart, nil
	}

	// Create values block
	var ports []interface{}
	for _, portStr := range strings.Split(fc.Params["ports"], ",") {
		ports = append(ports, strings.TrimSpace(portStr))
	}

	kafkaNetpolBlock := map[string]interface{}{
		"enabled":         true,
		"brokerNamespace": fc.Params["broker-namespace"],
		"brokerPodLabels": fc.Params["broker-pod-label"],
		"ports":           ports,
	}

	blockYaml, err := yaml.Marshal(map[string]interface{}{"kafkaNetworkPolicy": kafkaNetpolBlock})
	if err != nil {
		return chart, err
	}

	if chart.ValuesYAML == "" {
		chart.ValuesYAML = string(blockYaml)
	} else {
		chart.ValuesYAML += "\n" + string(blockYaml)
	}

	// Add template
	if chart.Templates == nil {
		chart.Templates = make(map[string]string)
	}

	templateContent := `{{- if .Values.kafkaNetworkPolicy.enabled }}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ $.Release.Name }}-kafka-egress
  labels:
    app.kubernetes.io/name: {{ $.Chart.Name }}
    app.kubernetes.io/instance: {{ $.Release.Name }}
spec:
  podSelector:
    matchLabels: {}
  policyTypes:
    - Egress
  egress:
    - ports:
        - protocol: UDP
          port: 53
        - protocol: TCP
          port: 53
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: {{ .Values.kafkaNetworkPolicy.brokerNamespace }}
          podSelector:
            matchLabels:
              {{- range $pair := splitList "," .Values.kafkaNetworkPolicy.brokerPodLabels }}
              {{- $kv := split "=" $pair }}
              {{ $kv._0 }}: {{ $kv._1 }}
              {{- end }}
      ports:
        {{- range .Values.kafkaNetworkPolicy.ports }}
        - protocol: TCP
          port: {{ . }}
        {{- end }}
{{- end }}
`
	chart.Templates["templates/kafka-networkpolicy.yaml"] = templateContent

	return chart, nil
}
