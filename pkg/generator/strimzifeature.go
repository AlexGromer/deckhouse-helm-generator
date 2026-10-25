package generator

import (
	"fmt"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/kafkaclient"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func init() {
	RegisterFeature(Feature{
		Name:        "strimzi",
		Description: "Generate KafkaTopic and KafkaUser based on workloads and connections for Strimzi Entity Operator",
		Params: map[string]string{
			"cluster":          "",
			"namespace":        "",
			"auth":             "scram-sha-512",
			"access":           "same-namespace",
			"idempotent-write": "false",
		},
		Apply: applyStrimziFeature,
	})
}

func applyStrimziFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {

	// First collect facts from chart workloads using kafkaclient
	facts := kafkaclient.ExtractFromGraph(fc.Graph)

	// Also use FromContainer for more detailed fact extraction directly from Graph objects
	for key, res := range fc.Graph.Resources {
		if key.GVK.Kind == "Deployment" || key.GVK.Kind == "StatefulSet" || key.GVK.Kind == "DaemonSet" || key.GVK.Kind == "Job" || key.GVK.Kind == "CronJob" {
			if res.Original != nil && res.Original.Object != nil {
				containers, _, _ := unstructured.NestedSlice(res.Original.Object.Object, "spec", "template", "spec", "containers")
				for _, c := range containers {
					if cMap, ok := c.(map[string]interface{}); ok {
						props := kafkaclient.FromContainer(cMap, nil)
						fw := kafkaclient.DetectFramework(props)
						containerFacts := kafkaclient.ExtractFacts(props, fw)

						clientFact := kafkaclient.KafkaClientFact{
							WorkloadKey:   key,
							ContainerName: cMap["name"].(string),
						}

						for _, t := range containerFacts.Topics {
							op := kafkaclient.KafkaTopicOpUnknown
							if t.Direction == kafkaclient.DirectionProduce {
								op = kafkaclient.KafkaTopicOpProduce
							}
							if t.Direction == kafkaclient.DirectionConsume {
								op = kafkaclient.KafkaTopicOpConsume
							}

							clientFact.Topics = append(clientFact.Topics, kafkaclient.KafkaTopicFact{
								Name: t.Name,
								Op:   op,
							})
						}

						if len(clientFact.Topics) > 0 {
							facts.Clients = append(facts.Clients, clientFact)
						}
					}
				}
			}
		}
	}

	// Create values block
	strimziBlock := map[string]interface{}{
		"enabled":   true,
		"cluster":   fc.Params["cluster"],
		"namespace": fc.Params["namespace"],
	}

	if fc.Params["cluster"] == "" {
		fc.Note("strimzi cluster name not set; setting it to empty; KafkaTopic/KafkaUser resources will not be watched by Strimzi operators without a cluster label")
	}

	topicsBlock := make(map[string]interface{})
	usersBlock := make(map[string]interface{})

	// Add topics from facts (deduplicated)
	for _, clientFact := range facts.Clients {
		for _, topicFact := range clientFact.Topics {
			topicsBlock[topicFact.Name] = map[string]interface{}{
				"enabled":   true,
				"topicName": topicFact.Name,
			}
		}
	}

	// Iterate over connections to find topics and users
	for _, conn := range fc.Graph.Connections {
		if conn.Type == types.RelationTopicProduce || conn.Type == types.RelationTopicConsume || conn.Type == types.RelationTopicUse {
			var topicName string
			if conn.To.Resource != nil {
				topicName = conn.To.Resource.Name
			} else {
				topicName = string(conn.To.Node)
			}
			enabled := true

			if conn.Type == types.RelationTopicUse || conn.Confidence == types.ConfidenceLow {
				enabled = false
				fc.Note(fmt.Sprintf("topic %s guessed or type is topic_use, adding with enabled: false", topicName))
			}

			if _, ok := topicsBlock[topicName]; !ok {
				topicsBlock[topicName] = map[string]interface{}{
					"enabled":   enabled,
					"topicName": topicName,
				}
			}

			// User ACLs
			if conn.From.GVK.Kind != "" && (conn.From.GVK.Kind == "Deployment" || conn.From.GVK.Kind == "StatefulSet" || conn.From.GVK.Kind == "DaemonSet" || conn.From.GVK.Kind == "Job" || conn.From.GVK.Kind == "CronJob") {
				userName := conn.From.Name
				if _, ok := usersBlock[userName]; !ok {
					usersBlock[userName] = map[string]interface{}{
						"enabled":        true,
						"name":           userName,
						"authentication": fc.Params["auth"],
						"acls":           []interface{}{},
					}
				}

				userMap := usersBlock[userName].(map[string]interface{})
				acls := userMap["acls"].([]interface{})

				// Minimal ACLs
				ops := []string{"Describe"}
				switch conn.Type {
				case types.RelationTopicProduce:
					ops = append(ops, "Write")
				case types.RelationTopicConsume:
					ops = append(ops, "Read")
					// Add group ACL for consumer
					acls = append(acls, map[string]interface{}{
						"resource": map[string]interface{}{
							"type":        "group",
							"name":        userName, // default group name
							"patternType": "literal",
						},
						"operations": []string{"Read"},
					})
				}
				for _, op := range ops {
					acls = append(acls, map[string]interface{}{
						"resource": map[string]interface{}{
							"type":        "topic",
							"name":        topicName,
							"patternType": "literal",
						},
						"operations": []string{op},
					})
				}

				userMap["acls"] = acls
			}
		}
	}

	strimziBlock["topics"] = topicsBlock
	strimziBlock["users"] = usersBlock
	strimziBlock["inject"] = map[string]interface{}{}
	strimziBlock["access"] = map[string]interface{}{
		"mode": fc.Params["access"],
	}

	// Inject values
	strimziYaml, err := yaml.Marshal(map[string]interface{}{"strimzi": strimziBlock})
	if err != nil {
		return chart, err
	}

	if chart.ValuesYAML == "" {
		chart.ValuesYAML = string(strimziYaml)
	} else {
		chart.ValuesYAML += "\n" + string(strimziYaml)
	}

	// Add templates
	if chart.Templates == nil {
		chart.Templates = make(map[string]string)
	}

	templateContent := `{{- if .Values.strimzi.enabled }}
{{- range $name, $topic := .Values.strimzi.topics }}
{{- if $topic.enabled }}
---
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaTopic
metadata:
  name: {{ $name | replace "_" "-" | lower }}
  namespace: {{ $.Values.strimzi.namespace | default $.Release.Namespace }}
  labels:
    {{- if $.Values.strimzi.cluster }}
    strimzi.io/cluster: {{ $.Values.strimzi.cluster }}
    {{- end }}
spec:
  topicName: {{ $topic.topicName | default $name }}
{{- end }}
{{- end }}
{{- range $name, $user := .Values.strimzi.users }}
{{- if $user.enabled }}
---
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaUser
metadata:
  name: {{ $name | replace "_" "-" | lower }}
  namespace: {{ $.Values.strimzi.namespace | default $.Release.Namespace }}
  labels:
    {{- if $.Values.strimzi.cluster }}
    strimzi.io/cluster: {{ $.Values.strimzi.cluster }}
    {{- end }}
spec:
  authentication:
    type: {{ $user.authentication }}
  authorization:
    type: simple
    acls:
{{ toYaml $user.acls | indent 6 }}
{{- end }}
{{- end }}
{{- end }}
`

	if fc.Params["access"] == "kafka-access" {
		templateContent += `
{{- if eq .Values.strimzi.access.mode "kafka-access" }}
---
apiVersion: access.strimzi.io/v1alpha1
kind: KafkaAccess
metadata:
  name: kafka-access
{{- end }}
`
	}

	chart.Templates["templates/strimzi-entities.yaml"] = templateContent

	return chart, nil
}
