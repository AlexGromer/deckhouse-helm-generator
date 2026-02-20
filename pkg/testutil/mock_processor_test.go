package testutil

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

func TestNewMockProcessor(t *testing.T) {
	gvks := []schema.GroupVersionKind{
		{Group: "apps", Version: "v1", Kind: "Deployment"},
	}
	mock := NewMockProcessor("test-proc", gvks, 10)

	if mock.Name() != "test-proc" {
		t.Errorf("expected name 'test-proc', got '%s'", mock.Name())
	}

	supports := mock.Supports()
	if len(supports) != 1 {
		t.Fatalf("expected 1 GVK, got %d", len(supports))
	}
	if supports[0].Kind != "Deployment" {
		t.Errorf("expected Kind 'Deployment', got '%s'", supports[0].Kind)
	}

	if mock.Priority() != 10 {
		t.Errorf("expected priority 10, got %d", mock.Priority())
	}
}

func TestMockProcessor_Process_Default(t *testing.T) {
	mock := NewMockProcessor("test", nil, 0)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "test"},
		},
	}

	ctx := processor.Context{}
	result, err := mock.Process(ctx, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Processed {
		t.Error("default mock should return Processed=false")
	}
	if mock.CallCount != 1 {
		t.Errorf("expected CallCount 1, got %d", mock.CallCount)
	}
	if mock.LastObject != obj {
		t.Error("LastObject should be set")
	}
}

func TestMockProcessor_Reset(t *testing.T) {
	mock := NewMockProcessor("test", nil, 0)
	mock.CallCount = 5
	mock.LastObject = &unstructured.Unstructured{}

	mock.Reset()

	if mock.CallCount != 0 {
		t.Error("CallCount should be 0 after Reset")
	}
	if mock.LastObject != nil {
		t.Error("LastObject should be nil after Reset")
	}
	if mock.LastContext != nil {
		t.Error("LastContext should be nil after Reset")
	}
}

func TestNewMockProcessorWithResult(t *testing.T) {
	result := &processor.Result{
		Processed: true,
		Values:    map[string]interface{}{"key": "value"},
	}

	mock := NewMockProcessorWithResult("result-proc", result, nil)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]interface{}{"name": "test"},
		},
	}

	got, err := mock.Process(processor.Context{}, obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Processed {
		t.Error("expected Processed=true")
	}
	if got.Values["key"] != "value" {
		t.Error("expected values to match")
	}
}

func TestNewMockProcessorWithResult_Error(t *testing.T) {
	expectedErr := fmt.Errorf("processing failed")
	mock := NewMockProcessorWithResult("err-proc", nil, expectedErr)

	_, err := mock.Process(processor.Context{}, &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   map[string]interface{}{"name": "test"},
		},
	})
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestMockProcessor_DefaultBehavior(t *testing.T) {
	// Test with no funcs set (bare MockProcessor)
	mock := &MockProcessor{}

	if mock.Name() != "MockProcessor" {
		t.Errorf("expected default name 'MockProcessor', got '%s'", mock.Name())
	}

	supports := mock.Supports()
	if len(supports) != 0 {
		t.Errorf("expected empty supports, got %d", len(supports))
	}

	if mock.Priority() != 0 {
		t.Errorf("expected default priority 0, got %d", mock.Priority())
	}
}
