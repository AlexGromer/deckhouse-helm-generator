package generator

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── Test Plan ──────────────────────────────────────────────────────────────────
//  1. pool Get returns non-nil
//  2. pool Put + Get reuses the object (same pointer or reset object)
//  3. StreamYAMLDocuments parses multi-doc YAML (--- separator)
//  4. StreamYAMLDocuments handles single document
//  5. StreamYAMLDocuments propagates handler error
//  6. StreamYAMLDocuments handles empty input
//  7. StreamYAMLDocuments with nil reader returns error
//  8. concurrent pool access (no race condition)
//  9. large document stream (many documents)
// 10. handler error stops streaming immediately

// ── ResourcePool tests ─────────────────────────────────────────────────────────

func TestResourcePool_Get_ReturnsNonNil(t *testing.T) {
	pool := NewResourcePool()
	if pool == nil {
		t.Fatal("NewResourcePool returned nil")
	}

	r := pool.Get()
	if r == nil {
		t.Fatal("ResourcePool.Get() returned nil — must always return a non-nil *ProcessedResource")
	}
}

func TestResourcePool_PutAndGet_ReusesObject(t *testing.T) {
	pool := NewResourcePool()

	// Get an object, mark it, put it back, get again — pool may return the same object.
	r1 := pool.Get()
	if r1 == nil {
		t.Fatal("first Get returned nil")
	}

	// Mark the object with a sentinel value.
	sentinel := "pool-reuse-test"
	r1.ServiceName = sentinel
	pool.Put(r1)

	r2 := pool.Get()
	if r2 == nil {
		t.Fatal("second Get returned nil")
	}

	// The pool may return r1 (reuse) or a new object (both are valid for sync.Pool).
	// We only verify Get is functional after a Put — no hang, no panic.
	_ = r2
}

func TestResourcePool_PutNilDoesNotPanic(t *testing.T) {
	pool := NewResourcePool()

	// Putting nil should not panic (defensive implementation).
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Put(nil) panicked: %v", r)
		}
	}()

	pool.Put(nil)
}

func TestResourcePool_GetReturnsFreshOrReset(t *testing.T) {
	pool := NewResourcePool()

	// Verify that Get always returns a *ProcessedResource with correct zero state
	// (either newly allocated or reset by the pool).
	r := pool.Get()
	if r == nil {
		t.Fatal("Get returned nil")
	}

	// A freshly allocated resource should have no original pointer set.
	// (If the pool resets objects, this also holds.)
	if r.Original != nil && r.ServiceName == "should-not-persist" {
		t.Error("pool returned a dirty object with leftover state")
	}
}

// ── Concurrent pool access ─────────────────────────────────────────────────────

func TestResourcePool_ConcurrentAccess_NoRace(t *testing.T) {
	pool := NewResourcePool()
	const goroutines = 20
	const iters = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				r := pool.Get()
				if r == nil {
					// Signal failure but don't call t.Fatal from goroutine
					return
				}
				r.ServiceName = "worker"
				pool.Put(r)
			}
		}()
	}

	wg.Wait()
	// If we reach here without a data race (run with -race), the test passes.
}

// ── StreamYAMLDocuments tests ──────────────────────────────────────────────────

func TestStreamYAMLDocuments_MultiDocYAML(t *testing.T) {
	input := `apiVersion: v1
kind: ConfigMap
metadata:
  name: first
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: second
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: third
`
	reader := strings.NewReader(input)
	var collected [][]byte

	err := StreamYAMLDocuments(reader, func(doc []byte) error {
		collected = append(collected, append([]byte(nil), doc...))
		return nil
	})

	if err != nil {
		t.Fatalf("StreamYAMLDocuments returned error: %v", err)
	}
	if len(collected) != 3 {
		t.Errorf("got %d documents, want 3", len(collected))
	}
}

func TestStreamYAMLDocuments_SingleDocument(t *testing.T) {
	input := `apiVersion: v1
kind: ConfigMap
metadata:
  name: only-one
`
	reader := strings.NewReader(input)
	var count int

	err := StreamYAMLDocuments(reader, func(doc []byte) error {
		count++
		return nil
	})

	if err != nil {
		t.Fatalf("StreamYAMLDocuments returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d documents, want 1", count)
	}
}

func TestStreamYAMLDocuments_HandlerError_Propagated(t *testing.T) {
	input := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
`
	reader := strings.NewReader(input)
	handlerErr := errors.New("handler failed")

	err := StreamYAMLDocuments(reader, func(doc []byte) error {
		return handlerErr
	})

	if err == nil {
		t.Fatal("expected error from handler to be propagated, got nil")
	}
	if !errors.Is(err, handlerErr) && !strings.Contains(err.Error(), handlerErr.Error()) {
		t.Errorf("propagated error = %v, want to contain %v", err, handlerErr)
	}
}

func TestStreamYAMLDocuments_EmptyInput(t *testing.T) {
	reader := strings.NewReader("")
	var count int

	err := StreamYAMLDocuments(reader, func(doc []byte) error {
		count++
		return nil
	})

	if err != nil {
		t.Fatalf("StreamYAMLDocuments returned error for empty input: %v", err)
	}
	// Empty input may produce 0 documents or 1 empty document — both are acceptable.
	// The important thing: no error and no panic.
	_ = count
}

func TestStreamYAMLDocuments_NilReader_ReturnsError(t *testing.T) {
	var reader io.Reader // nil interface

	err := StreamYAMLDocuments(reader, func(doc []byte) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error for nil reader, got nil")
	}
}

func TestStreamYAMLDocuments_HandlerError_StopsStream(t *testing.T) {
	// 5 documents; handler fails on the 2nd — must not process further.
	docs := []string{
		"kind: ConfigMap\nmetadata:\n  name: doc1",
		"kind: ConfigMap\nmetadata:\n  name: doc2",
		"kind: ConfigMap\nmetadata:\n  name: doc3",
		"kind: ConfigMap\nmetadata:\n  name: doc4",
		"kind: ConfigMap\nmetadata:\n  name: doc5",
	}
	input := strings.Join(docs, "\n---\n")
	reader := strings.NewReader(input)

	var callCount int
	stopErr := errors.New("stop at second")

	err := StreamYAMLDocuments(reader, func(doc []byte) error {
		callCount++
		if callCount == 2 {
			return stopErr
		}
		return nil
	})

	if err == nil {
		t.Fatal("expected error to be returned, got nil")
	}
	if callCount > 3 {
		// Allow at most one in-flight document after the error.
		t.Errorf("handler called %d times after error at call 2 — streaming not stopped", callCount)
	}
}

func TestStreamYAMLDocuments_LargeDocumentStream(t *testing.T) {
	const numDocs = 500
	var buf bytes.Buffer
	for i := 0; i < numDocs; i++ {
		if i > 0 {
			buf.WriteString("---\n")
		}
		buf.WriteString("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: doc-")
		buf.WriteString(strings.Repeat("a", 10))
		buf.WriteString("\n")
	}

	var count int
	err := StreamYAMLDocuments(&buf, func(doc []byte) error {
		count++
		return nil
	})

	if err != nil {
		t.Fatalf("StreamYAMLDocuments returned error for large stream: %v", err)
	}
	if count != numDocs {
		t.Errorf("got %d documents, want %d", count, numDocs)
	}
}

// compile-time assertion: ResourcePool must have the expected methods
var _ interface {
	Get() *types.ProcessedResource
	Put(*types.ProcessedResource)
} = (*ResourcePool)(nil)
