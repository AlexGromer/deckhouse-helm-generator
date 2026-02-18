package testutil

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// MockProcessor is a test implementation of processor.Processor
// Allows controlling behavior for testing processor chains and registry
type MockProcessor struct {
	// ProcessFunc is called when Process() is invoked
	ProcessFunc func(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error)

	// SupportsFunc returns the GVKs this mock supports
	SupportsFunc func() []schema.GroupVersionKind

	// PriorityFunc returns the priority
	PriorityFunc func() int

	// NameFunc returns the processor name
	NameFunc func() string

	// CallCount tracks how many times Process was called
	CallCount int

	// LastContext stores the last context passed to Process
	LastContext *processor.Context

	// LastObject stores the last object passed to Process
	LastObject *unstructured.Unstructured
}

// Process implements processor.Processor
func (m *MockProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	m.CallCount++
	m.LastContext = &ctx
	m.LastObject = obj

	if m.ProcessFunc != nil {
		return m.ProcessFunc(ctx, obj)
	}

	// Default: return processed=false (doesn't handle this resource)
	return &processor.Result{Processed: false}, nil
}

// Supports implements processor.Processor
func (m *MockProcessor) Supports() []schema.GroupVersionKind {
	if m.SupportsFunc != nil {
		return m.SupportsFunc()
	}

	// Default: empty list (supports nothing)
	return []schema.GroupVersionKind{}
}

// Priority implements processor.Processor
func (m *MockProcessor) Priority() int {
	if m.PriorityFunc != nil {
		return m.PriorityFunc()
	}

	// Default: priority 0
	return 0
}

// Name implements processor.Processor
func (m *MockProcessor) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}

	// Default: "MockProcessor"
	return "MockProcessor"
}

// Reset clears call tracking data
func (m *MockProcessor) Reset() {
	m.CallCount = 0
	m.LastContext = nil
	m.LastObject = nil
}

// NewMockProcessor creates a mock processor with default behavior
func NewMockProcessor(name string, gvks []schema.GroupVersionKind, priority int) *MockProcessor {
	return &MockProcessor{
		NameFunc: func() string {
			return name
		},
		SupportsFunc: func() []schema.GroupVersionKind {
			return gvks
		},
		PriorityFunc: func() int {
			return priority
		},
	}
}

// NewMockProcessorWithResult creates a mock that always returns the given result
func NewMockProcessorWithResult(name string, result *processor.Result, err error) *MockProcessor {
	return &MockProcessor{
		NameFunc: func() string {
			return name
		},
		ProcessFunc: func(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
			return result, err
		},
		SupportsFunc: func() []schema.GroupVersionKind {
			return []schema.GroupVersionKind{}
		},
		PriorityFunc: func() int {
			return 0
		},
	}
}
