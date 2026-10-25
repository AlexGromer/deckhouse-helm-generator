package generator

import "github.com/AlexGromer/deckhouse-helm-generator/pkg/types"

func init() {
	RegisterFeature(Feature{
		Name:        "kafka-client",
		Description: "Configure Kafka client security (security.protocol, SASL, truststore/keystore from existing Secrets) for workloads that use Kafka",
		Params: map[string]string{
			"mode":            "auto",
			"strimzi-version": "v1beta2",
			"cluster":         "",
			"auth":            "auto",
			"partitions":      "3",
			"replicas":        "3",
		},
		Apply: applyKafkaClientFeature,
	})
}

func applyKafkaClientFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	return ApplyKafkaClient(chart, fc)
}
