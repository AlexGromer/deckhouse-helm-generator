package generator

import (
	"fmt"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/kafkaclient"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func ApplyKafkaClient(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	facts := kafkaclient.Extract(fc.Graph)

	if len(facts.Clients) == 0 {
		return chart, nil
	}

	for _, warning := range facts.Warnings {
		fmt.Println("Note: " + warning)
	}

	if chart.ValuesYAML == "" {
		chart.ValuesYAML = `kafkaClient:
  enabled: true
  securityProtocol: ""
  sasl:
    mechanism: ""
    jaasConfigSecret:
      name: ""
      key: sasl.jaas.config
  tls:
    truststore:
      secretName: ""
      type: PEM
`
	} else {
		chart.ValuesYAML += `kafkaClient:
  enabled: true
  securityProtocol: ""
  sasl:
    mechanism: ""
    jaasConfigSecret:
      name: ""
      key: sasl.jaas.config
  tls:
    truststore:
      secretName: ""
      type: PEM
`
	}

	if facts.Mode == kafkaclient.KafkaModeStrimzi {
		generateKafkaTopic(chart, facts, fc)
		generateKafkaUser(chart, facts, fc)
	} else {
		generateKafkaSecret(chart, facts, fc)
	}

	chart.Notes += "## Kafka Integration\nKafka integration has been generated with mode: " + string(facts.Mode) + "\n"

	return chart, nil
}

func generateKafkaTopic(chart *types.GeneratedChart, facts *kafkaclient.KafkaFacts, fc FeatureContext) {
	if chart.Templates == nil {
		chart.Templates = make(map[string]string)
	}
	if len(facts.Clients) > 0 {
		templateContent := `apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaTopic
metadata:
  name: generated-topic
spec:
  partitions: {{ .Values.kafkaClient.partitions | default 3 }}
  replicas: {{ .Values.kafkaClient.replicas | default 3 }}
`
		chart.Templates["templates/kafka-topic.yaml"] = templateContent
	}

}

func generateKafkaUser(chart *types.GeneratedChart, facts *kafkaclient.KafkaFacts, fc FeatureContext) {
	templateContent := `apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaUser
metadata:
  name: {{ .Values.kafkaClient.userName | default "generated-user" }}
spec:
  authentication:
    type: tls
`
	if chart.Templates == nil {
		chart.Templates = make(map[string]string)
	}
	chart.Templates["templates/kafka-user.yaml"] = templateContent
}

func generateKafkaSecret(chart *types.GeneratedChart, facts *kafkaclient.KafkaFacts, fc FeatureContext) {
	templateContent := `apiVersion: v1
kind: Secret
metadata:
  name: generated-kafka-secret
type: Opaque
data:
  sasl.jaas.config: ""
`
	if chart.Templates == nil {
		chart.Templates = make(map[string]string)
	}
	chart.Templates["templates/kafka-secret.yaml"] = templateContent
}
