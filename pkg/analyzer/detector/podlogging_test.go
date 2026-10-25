package detector

import (
	"context"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestPodLoggingDetector_Detect(t *testing.T) {
	podLogging := makeProcessedResource(
		"deckhouse.io/v1alpha1", "PodLoggingConfig", "orders-logs", "default",
		nil, nil,
		map[string]interface{}{
			"labelSelector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "orders",
				},
			},
		},
	)

	deployMatch := makeProcessedResource(
		"apps/v1", "Deployment", "orders-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "orders",
					},
				},
			},
		},
	)

	deployNoMatch := makeProcessedResource(
		"apps/v1", "Deployment", "legacy-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "legacy",
					},
				},
			},
		},
	)

	allResources := buildAllResources(podLogging, deployMatch, deployNoMatch)

	d := NewPodLoggingDetector()
	rels := d.Detect(context.Background(), podLogging, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationLabelSelector {
		t.Errorf("Type = %q; want %q", rel.Type, types.RelationLabelSelector)
	}
	if rel.To != deployMatch.Original.ResourceKey() {
		t.Errorf("To = %v; want %v", rel.To, deployMatch.Original.ResourceKey())
	}
}

func TestPodLoggingDetector_MatchExpressions(t *testing.T) {
	podLogging := makeProcessedResource(
		"deckhouse.io/v1alpha1", "PodLoggingConfig", "orders-logs", "default",
		nil, nil,
		map[string]interface{}{
			"labelSelector": map[string]interface{}{
				"matchExpressions": []interface{}{
					map[string]interface{}{
						"key":      "app",
						"operator": "In",
						"values":   []interface{}{"orders"},
					},
				},
			},
		},
	)

	deployMatch := makeProcessedResource(
		"apps/v1", "Deployment", "orders-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "orders",
					},
				},
			},
		},
	)

	allResources := buildAllResources(podLogging, deployMatch)

	d := NewPodLoggingDetector()
	rels := d.Detect(context.Background(), podLogging, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}
}

func TestPodLoggingDetector_CrossNamespace(t *testing.T) {
	podLogging := makeProcessedResource(
		"deckhouse.io/v1alpha1", "PodLoggingConfig", "orders-logs", "ns-a",
		nil, nil,
		map[string]interface{}{
			"labelSelector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "orders",
				},
			},
		},
	)

	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "orders-deploy", "ns-b",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "orders",
					},
				},
			},
		},
	)

	allResources := buildAllResources(podLogging, deploy)

	d := NewPodLoggingDetector()
	rels := d.Detect(context.Background(), podLogging, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships, got %d", len(rels))
	}
}

func TestPodLoggingDetector_NoSelector(t *testing.T) {
	podLogging := makeProcessedResource(
		"deckhouse.io/v1alpha1", "PodLoggingConfig", "orders-logs", "default",
		nil, nil,
		nil, // no spec fields
	)

	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "orders-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "orders",
					},
				},
			},
		},
	)

	allResources := buildAllResources(podLogging, deploy)

	d := NewPodLoggingDetector()
	rels := d.Detect(context.Background(), podLogging, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships, got %d", len(rels))
	}
}
func TestPodLoggingDetector_NameAndPriority(t *testing.T) {
	d := NewPodLoggingDetector()
	if d.Name() != "pod_logging_selector" {
		t.Errorf("expected pod_logging_selector, got %s", d.Name())
	}
	if d.Priority() != 100 {
		t.Errorf("expected 100, got %d", d.Priority())
	}
}
