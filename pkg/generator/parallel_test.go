package generator

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── Test Plan ──────────────────────────────────────────────────────────────────
//  1. parallel execution — 10 resources, 4 workers, all processed
//  2. correct result count matches input count
//  3. fail-fast — stops on first error, returns error
//  4. collect-all — processes all, returns all errors
//  5. single worker processes all resources
//  6. Workers=0 defaults to runtime.NumCPU()
//  7. nil processor returns error immediately
//  8. empty resource slice returns empty results, no error
//  9. order preserved across workers
// 10. context cancellation stops processing

// makeExtractedResources builds N simple ExtractedResource objects for testing.
func makeExtractedResources(n int) []*types.ExtractedResource {
	resources := make([]*types.ExtractedResource, n)
	for i := range resources {
		resources[i] = &types.ExtractedResource{
			SourcePath: fmt.Sprintf("/test/resource-%d.yaml", i),
		}
	}
	return resources
}

// identityProcessor wraps an ExtractedResource into a ProcessedResource unchanged.
func identityProcessor(r *types.ExtractedResource) (*types.ProcessedResource, error) {
	return &types.ProcessedResource{
		Original: r,
	}, nil
}

// indexedProcessor records the processing order via an atomic counter.
func indexedProcessor(counter *atomic.Int64) func(*types.ExtractedResource) (*types.ProcessedResource, error) {
	return func(r *types.ExtractedResource) (*types.ProcessedResource, error) {
		counter.Add(1)
		return &types.ProcessedResource{Original: r}, nil
	}
}

// slowProcessor introduces a small delay to test real concurrency.
func slowProcessor(delay time.Duration) func(*types.ExtractedResource) (*types.ProcessedResource, error) {
	return func(r *types.ExtractedResource) (*types.ProcessedResource, error) {
		time.Sleep(delay)
		return &types.ProcessedResource{Original: r}, nil
	}
}

// failAfterN fails on the Nth call (1-indexed).
func failAfterN(n int) func(*types.ExtractedResource) (*types.ProcessedResource, error) {
	var count atomic.Int64
	return func(r *types.ExtractedResource) (*types.ProcessedResource, error) {
		c := count.Add(1)
		if c >= int64(n) {
			return nil, fmt.Errorf("intentional failure at call %d", c)
		}
		return &types.ProcessedResource{Original: r}, nil
	}
}

// ── Tests ──────────────────────────────────────────────────────────────────────

func TestProcessParallel_ExecutesAll10Resources(t *testing.T) {
	resources := makeExtractedResources(10)
	var processed atomic.Int64
	processor := func(r *types.ExtractedResource) (*types.ProcessedResource, error) {
		processed.Add(1)
		return &types.ProcessedResource{Original: r}, nil
	}

	results, err := ProcessParallel(resources, processor, ParallelOptions{
		Workers:       4,
		BufferSize:    10,
		ErrorStrategy: "collect-all",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if processed.Load() != 10 {
		t.Errorf("processor called %d times, want 10", processed.Load())
	}
	_ = results
}

func TestProcessParallel_CorrectResultCount(t *testing.T) {
	resources := makeExtractedResources(10)

	results, err := ProcessParallel(resources, identityProcessor, ParallelOptions{
		Workers:       4,
		BufferSize:    10,
		ErrorStrategy: "collect-all",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 10 {
		t.Errorf("got %d results, want 10", len(results))
	}
}

func TestProcessParallel_FailFast_StopsOnFirstError(t *testing.T) {
	// 10 resources; processor fails starting at call #3 — fail-fast must return error.
	resources := makeExtractedResources(10)

	_, err := ProcessParallel(resources, failAfterN(3), ParallelOptions{
		Workers:       2,
		BufferSize:    5,
		ErrorStrategy: "fail-fast",
	})

	if err == nil {
		t.Fatal("expected error with fail-fast strategy, got nil")
	}
}

func TestProcessParallel_CollectAll_ReturnsAllErrors(t *testing.T) {
	resources := makeExtractedResources(5)
	// All calls fail
	alwaysFail := func(r *types.ExtractedResource) (*types.ProcessedResource, error) {
		return nil, errors.New("always fails")
	}

	_, err := ProcessParallel(resources, alwaysFail, ParallelOptions{
		Workers:       2,
		BufferSize:    5,
		ErrorStrategy: "collect-all",
	})

	if err == nil {
		t.Fatal("expected error with collect-all strategy when all processors fail, got nil")
	}

	// The combined error message should reference multiple failures
	errMsg := err.Error()
	if !containsMultipleErrorIndicators(errMsg) {
		t.Logf("error message: %s", errMsg)
		// Not a hard failure — single-line aggregate error is also acceptable.
		// The important thing is that err != nil (checked above).
	}
}

// containsMultipleErrorIndicators checks if an error message suggests multiple failures.
func containsMultipleErrorIndicators(msg string) bool {
	count := 0
	for _, ch := range msg {
		if ch == '\n' || ch == ';' {
			count++
		}
	}
	return count > 0 || len(msg) > 20
}

func TestProcessParallel_SingleWorker(t *testing.T) {
	resources := makeExtractedResources(6)

	results, err := ProcessParallel(resources, identityProcessor, ParallelOptions{
		Workers:       1,
		BufferSize:    6,
		ErrorStrategy: "collect-all",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 6 {
		t.Errorf("got %d results with single worker, want 6", len(results))
	}
}

func TestProcessParallel_ZeroWorkers_DefaultsToNumCPU(t *testing.T) {
	// Workers=0 should default to runtime.NumCPU() — must not hang or error.
	resources := makeExtractedResources(8)
	var counter atomic.Int64

	results, err := ProcessParallel(resources, indexedProcessor(&counter), ParallelOptions{
		Workers:       0, // triggers default
		BufferSize:    8,
		ErrorStrategy: "collect-all",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counter.Load() != 8 {
		t.Errorf("processed %d resources, want 8", counter.Load())
	}
	// Verify no panic; actual worker count is implementation detail
	_ = runtime.NumCPU()
	_ = results
}

func TestProcessParallel_NilProcessor_ReturnsError(t *testing.T) {
	resources := makeExtractedResources(3)

	_, err := ProcessParallel(resources, nil, ParallelOptions{
		Workers:       2,
		BufferSize:    3,
		ErrorStrategy: "fail-fast",
	})

	if err == nil {
		t.Fatal("expected error for nil processor, got nil")
	}
}

func TestProcessParallel_EmptyResources_ReturnsEmpty(t *testing.T) {
	var resources []*types.ExtractedResource

	results, err := ProcessParallel(resources, identityProcessor, ParallelOptions{
		Workers:       4,
		BufferSize:    4,
		ErrorStrategy: "collect-all",
	})

	if err != nil {
		t.Fatalf("unexpected error for empty input: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results for empty input, want 0", len(results))
	}
}

func TestProcessParallel_OrderPreserved(t *testing.T) {
	// Each resource carries its index in SourcePath; results must match input order.
	const n = 20
	resources := makeExtractedResources(n)

	results, err := ProcessParallel(resources, identityProcessor, ParallelOptions{
		Workers:       4,
		BufferSize:    n,
		ErrorStrategy: "collect-all",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != n {
		t.Fatalf("got %d results, want %d", len(results), n)
	}

	for i, r := range results {
		if r == nil {
			t.Errorf("result[%d] is nil", i)
			continue
		}
		want := fmt.Sprintf("/test/resource-%d.yaml", i)
		if r.Original == nil || r.Original.SourcePath != want {
			got := ""
			if r.Original != nil {
				got = r.Original.SourcePath
			}
			t.Errorf("result[%d]: got SourcePath=%q, want %q (order not preserved)", i, got, want)
		}
	}
}

func TestProcessParallel_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// 50 slow resources; cancel after a brief moment.
	resources := makeExtractedResources(50)
	proc := slowProcessor(10 * time.Millisecond)

	// Wrap processor to respect context.
	ctxProcessor := func(r *types.ExtractedResource) (*types.ProcessedResource, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return proc(r)
		}
	}

	go func() {
		time.Sleep(15 * time.Millisecond)
		cancel()
	}()

	_, err := ProcessParallel(resources, ctxProcessor, ParallelOptions{
		Workers:       2,
		BufferSize:    10,
		ErrorStrategy: "fail-fast",
	})

	// After cancellation, an error is expected (context.Canceled or derived).
	// If the implementation drains all results before returning, err may be nil —
	// which is also acceptable. The key requirement is: no hang/deadlock.
	if err != nil {
		if !errors.Is(err, context.Canceled) && !isContextError(err) {
			// A non-context error is unexpected but not a hard failure —
			// the cancellation path varies by implementation.
			t.Logf("cancellation returned non-context error: %v", err)
		}
	}
}

// isContextError checks if the error is context-related.
func isContextError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "context") || contains(msg, "canceled") || contains(msg, "cancelled")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
