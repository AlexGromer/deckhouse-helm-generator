package k8s

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// KafkaTopicProcessor processes Strimzi KafkaTopic resources.
type KafkaTopicProcessor struct {
	processor.BaseProcessor
}

// NewKafkaTopicProcessor creates a new KafkaTopic processor.
func NewKafkaTopicProcessor() *KafkaTopicProcessor {
	return &KafkaTopicProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"kafkatopic",
			70,
			schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1", Kind: "KafkaTopic"},
			schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1beta2", Kind: "KafkaTopic"},
			schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1beta1", Kind: "KafkaTopic"},
			schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1alpha1", Kind: "KafkaTopic"},
		),
	}
}

// Process processes a KafkaTopic resource.
func (p *KafkaTopicProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	return processStrimziEntity(ctx, obj, "kafkaTopic", "kafkaTopics", p.extractValues)
}

func (p *KafkaTopicProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey, []string) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey
	var notes []string

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	if partitions, ok := nestedInt64(obj.Object, "spec", "partitions"); ok {
		values["partitions"] = partitions
	}

	if replicas, ok := nestedInt64(obj.Object, "spec", "replicas"); ok {
		values["replicas"] = replicas
	}

	if topicName, ok, _ := unstructured.NestedString(obj.Object, "spec", "topicName"); ok {
		values["topicName"] = topicName
	}

	if config, ok, _ := unstructured.NestedMap(obj.Object, "spec", "config"); ok {
		values["config"] = config
	}

	return values, deps, notes
}

// KafkaUserProcessor processes Strimzi KafkaUser resources.
type KafkaUserProcessor struct {
	processor.BaseProcessor
}

// NewKafkaUserProcessor creates a new KafkaUser processor.
func NewKafkaUserProcessor() *KafkaUserProcessor {
	return &KafkaUserProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"kafkauser",
			70,
			schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1", Kind: "KafkaUser"},
			schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1beta2", Kind: "KafkaUser"},
			schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1beta1", Kind: "KafkaUser"},
			schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1alpha1", Kind: "KafkaUser"},
		),
	}
}

// Process processes a KafkaUser resource.
func (p *KafkaUserProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	return processStrimziEntity(ctx, obj, "kafkaUser", "kafkaUsers", p.extractValues)
}

func (p *KafkaUserProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey, []string) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey
	var notes []string

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	if authType, ok, _ := unstructured.NestedString(obj.Object, "spec", "authentication", "type"); ok {
		values["authentication"] = map[string]interface{}{"type": authType}
	}

	if secretName, ok, _ := unstructured.NestedString(obj.Object, "spec", "authentication", "password", "valueFrom", "secretKeyRef", "name"); ok && secretName != "" {
		deps = append(deps, types.ResourceKey{
			GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
			Namespace: obj.GetNamespace(),
			Name:      secretName,
		})
	}

	if authzType, ok, _ := unstructured.NestedString(obj.Object, "spec", "authorization", "type"); ok {
		if _, ok := values["authorization"]; !ok {
			values["authorization"] = map[string]interface{}{}
		}
		values["authorization"].(map[string]interface{})["type"] = authzType
	}

	if acls, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "authorization", "acls"); ok {
		if _, ok := values["authorization"]; !ok {
			values["authorization"] = map[string]interface{}{}
		}
		values["authorization"].(map[string]interface{})["acls"] = acls

		for _, aclRaw := range acls {
			if acl, ok := aclRaw.(map[string]interface{}); ok {
				if _, hasOp := acl["operation"]; hasOp {
					notes = append(notes, fmt.Sprintf("KafkaUser %s/%s: spec.authorization.acls[].operation was removed in kafka.strimzi.io/v1; use operations", obj.GetNamespace(), obj.GetName()))
					break
				}
			}
		}
	}

	if quotas, ok, _ := unstructured.NestedMap(obj.Object, "spec", "quotas"); ok {
		values["quotas"] = quotas
	}

	if template, ok, _ := unstructured.NestedMap(obj.Object, "spec", "template"); ok {
		values["template"] = template
	}

	return values, deps, notes
}

func processStrimziEntity(
	ctx processor.Context,
	obj *unstructured.Unstructured,
	entityKind string,
	valuesKey string,
	extractFunc func(*unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey, []string),
) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("%s object is nil", entityKind)
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()
	kind := obj.GetKind()

	apiVersion := obj.GetAPIVersion()
	if strings.HasSuffix(apiVersion, "/v1beta1") || strings.HasSuffix(apiVersion, "/v1alpha1") {
		apiVersion = "kafka.strimzi.io/v1beta2"
	}

	var notes []string

	if strings.HasSuffix(apiVersion, "/v1beta2") {
		notes = append(notes, fmt.Sprintf("%s %s/%s uses kafka.strimzi.io/v1beta2, removed in Strimzi 1.0.0; convert it to v1 (Strimzi v1 API Conversion Tool)", kind, namespace, name))
	}

	values, deps, extractNotes := extractFunc(obj)
	notes = append(notes, extractNotes...)

	values["enabled"] = true
	values["namespace"] = ""

	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	clusterName, hasCluster := labels["strimzi.io/cluster"]
	if hasCluster {
		values["cluster"] = clusterName
		delete(labels, "strimzi.io/cluster")
	} else {
		values["cluster"] = ""
		notes = append(notes, fmt.Sprintf("%s %s/%s has no strimzi.io/cluster label; set services.%s.%s.%s.cluster, the Strimzi operator ignores objects without it", kind, namespace, name, processor.SanitizeServiceName(serviceName), valuesKey, sanitizeName(name)))
	}

	// Check namespace mismatch
	if namespace != "" {
		hasWorkloadInDifferentNamespace := false
		for _, res := range ctx.AllResources {
			gvk := res.GVK
			resKind := gvk.Kind
			if (resKind == "Deployment" || resKind == "StatefulSet" || resKind == "DaemonSet" || resKind == "Job" || resKind == "CronJob") && res.Object.GetNamespace() != "" && res.Object.GetNamespace() != namespace {
				hasWorkloadInDifferentNamespace = true
				break
			}
		}
		if hasWorkloadInDifferentNamespace {
			notes = append(notes, fmt.Sprintf("%s %s/%s is rendered into the release namespace; the Topic/User Operator watches only its own namespace (by default the Kafka cluster's): set services.%s.%s.%s.namespace", kind, namespace, name, processor.SanitizeServiceName(serviceName), valuesKey, sanitizeName(name)))
		}
	}

	annotations := obj.GetAnnotations()
	if len(annotations) > 0 {
		values["annotations"] = annotations
	}

	// Make labels literal map
	var labelsDict strings.Builder
	labelsDict.WriteString("dict ")
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&labelsDict, "%q %q ", k, labels[k])
	}
	labelsDictStr := strings.TrimSpace(labelsDict.String())
	if labelsDictStr == "dict" {
		labelsDictStr = "dict"
	}

	var overlayKeys []string
	switch kind {
	case "KafkaTopic":
		overlayKeys = []string{"topicName", "partitions", "replicas", "config"}
	case "KafkaUser":
		overlayKeys = []string{"authentication", "authorization", "quotas", "template"}
	}

	var specBlock string
	if _, ok := values["spec"]; ok {
		specBlock = processor.SpecOverlay("$entity", overlayKeys...)
	}

	template := fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- $entity := $svc.%s.%s -}}
{{- if $entity.enabled }}
apiVersion: %s
kind: %s
metadata:
  name: %s
  namespace: {{ $entity.namespace | default $.Release.Namespace }}
  labels:
    {{- $dhgLabels := %s }}
    {{- with $entity.cluster }}
    {{- $_ := set $dhgLabels "strimzi.io/cluster" . }}
    {{- end }}
    {{- merge $dhgLabels (include "%s.labels" $ | fromYaml) | toYaml | nindent 4 }}
  {{- with $entity.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
%s{{- end }}
{{- end }}
`, processor.SanitizeServiceName(serviceName), valuesKey, sanitizeName(name), apiVersion, kind, processor.ObjectName(name), labelsDictStr, ctx.ChartName, specBlock)

	return &processor.Result{
		Processed:       true,
		ServiceName:     processor.SanitizeServiceName(serviceName),
		TemplatePath:    fmt.Sprintf("templates/%s-%s-%s.yaml", processor.SanitizeServiceName(serviceName), strings.ToLower(kind), name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.%s.%s", processor.SanitizeServiceName(serviceName), valuesKey, sanitizeName(name)),
		Values:          values,
		Dependencies:    deps,
		Notes:           notes,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}
