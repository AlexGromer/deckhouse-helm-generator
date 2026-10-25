package generator

import (
	"sort"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/kafkaclient"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"sigs.k8s.io/yaml"
)

func init() {
	RegisterFeature(Feature{
		Name:        "kafka-alerts",
		Description: "Generate PrometheusRule alerts for Kafka consumers and clients",
		Params: map[string]string{
			"lag-threshold": "1000",
			"for":           "5m",
			"severity":      "warning",
		},
		Apply: applyKafkaAlertsFeature,
	})
}

func applyKafkaAlertsFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	// Collect facts from chart workloads using kafkaclient
	facts := kafkaclient.ExtractFromGraph(fc.Graph)
	hasClients := len(facts.Clients) > 0

	consumerGroupsMap := make(map[string]bool)
	topicsMap := make(map[string]bool)

	for _, clientFact := range facts.Clients {
		if clientFact.GroupID != "" {
			consumerGroupsMap[clientFact.GroupID] = true
		}
		for _, topicFact := range clientFact.Topics {
			topicsMap[topicFact.Name] = true
		}
	}

	// Check connections for Kafka consumer usage
	for _, conn := range fc.Graph.Connections {
		if conn.Type == types.RelationTopicProduce || conn.Type == types.RelationTopicConsume || conn.Type == types.RelationTopicUse {
			hasClients = true
			var topicName string
			if conn.To.Resource != nil {
				topicName = conn.To.Resource.Name
			} else {
				topicName = string(conn.To.Node)
			}
			topicsMap[topicName] = true
		}

		if conn.Type == types.RelationTopicConsume {
			if group, ok := conn.Attributes["group"]; ok && group != "" {
				consumerGroupsMap[group] = true
			} else {
				// Fallback to service name if group is not explicitly in attributes
				consumerGroupsMap[conn.From.Name] = true
			}
		}
	}

	if !hasClients {
		return chart, nil
	}

	var consumerGroups []string
	for g := range consumerGroupsMap {
		consumerGroups = append(consumerGroups, g)
	}

	var topics []string
	for t := range topicsMap {
		topics = append(topics, t)
	}

	// Ensure deterministic output
	sort.Strings(consumerGroups)
	sort.Strings(topics)

	// Create values block
	kafkaAlertsBlock := map[string]interface{}{
		"enabled":      true,
		"lagThreshold": fc.Params["lag-threshold"],
		"for":          fc.Params["for"],
		"severity":     fc.Params["severity"],
	}

	if len(consumerGroups) > 0 {
		kafkaAlertsBlock["consumerGroups"] = consumerGroups
	} else {
		kafkaAlertsBlock["consumerGroups"] = []string{}
	}

	if len(topics) > 0 {
		kafkaAlertsBlock["topics"] = topics
	} else {
		kafkaAlertsBlock["topics"] = []string{}
	}

	blockYaml, err := yaml.Marshal(map[string]interface{}{"kafkaAlerts": kafkaAlertsBlock})
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

	templateContent := `{{- if .Values.kafkaAlerts.enabled }}
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: {{ $.Release.Name }}-kafka-alerts
  labels:
    app.kubernetes.io/name: {{ $.Chart.Name }}
    app.kubernetes.io/instance: {{ $.Release.Name }}
spec:
  groups:
    - name: KafkaConsumerAlerts
      rules:
        {{- if .Values.kafkaAlerts.consumerGroups }}
        {{- range .Values.kafkaAlerts.consumerGroups }}
        - alert: KafkaConsumerLagHigh
          expr: kafka_consumergroup_lag{consumergroup="{{ . }}"} > {{ $.Values.kafkaAlerts.lagThreshold }}
          for: {{ $.Values.kafkaAlerts.for }}
          labels:
            severity: {{ $.Values.kafkaAlerts.severity }}
          annotations:
            summary: Kafka consumer lag is too high
            description: Consumer group {{ . }} has lag > {{ $.Values.kafkaAlerts.lagThreshold }}
        {{- end }}
        {{- end }}
        - alert: KafkaClientConnectionErrors
          expr: rate(kafka_client_connection_errors_total[5m]) > 0.05
          for: {{ $.Values.kafkaAlerts.for }}
          labels:
            severity: {{ $.Values.kafkaAlerts.severity }}
          annotations:
            summary: Kafka client connection errors
            description: Kafka client has > 5% connection errors in the last 5 minutes
{{- end }}
`
	chart.Templates["templates/kafka-prometheusrules.yaml"] = templateContent

	return chart, nil
}
