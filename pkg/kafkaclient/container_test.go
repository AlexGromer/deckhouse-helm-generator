package kafkaclient

import (
	"reflect"
	"testing"
)

func TestFromContainer(t *testing.T) {
	container := map[string]interface{}{
		"envFrom": []interface{}{
			map[string]interface{}{
				"prefix": "APP_",
				"configMapRef": map[string]interface{}{
					"name": "my-config",
				},
			},
		},
		"env": []interface{}{
			map[string]interface{}{
				"name":  "DIRECT_VAR",
				"value": "direct-value",
			},
			map[string]interface{}{
				"name": "SECRET_VAR",
				"valueFrom": map[string]interface{}{
					"secretKeyRef": map[string]interface{}{
						"name": "my-secret",
					},
				},
			},
			map[string]interface{}{
				"name": "CM_VAR",
				"valueFrom": map[string]interface{}{
					"configMapKeyRef": map[string]interface{}{
						"name": "my-config-2",
						"key":  "my-key",
					},
				},
			},
		},
	}

	configMaps := map[string]map[string]string{
		"my-config": {
			"TOPIC": "orders",
		},
		"my-config-2": {
			"my-key": "cm-value",
		},
	}

	props := FromContainer(container, configMaps)

	// envFrom is processed first, so APP_TOPIC should be first
	expected := []Property{
		{Key: "APP_TOPIC", Value: "orders", Origin: "configMap my-config key TOPIC"},
		{Key: "DIRECT_VAR", Value: "direct-value", Origin: "env DIRECT_VAR"},
		{Key: "SECRET_VAR", Value: "", FromSecret: true, Origin: "env SECRET_VAR"},
		{Key: "CM_VAR", Value: "cm-value", Origin: "env CM_VAR"},
	}

	if !reflect.DeepEqual(props, expected) {
		t.Errorf("FromContainer() = %v, want %v", props, expected)
	}
}
