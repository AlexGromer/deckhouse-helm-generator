package generator

import (
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ParallelOptions configures the parallel processing worker pool.
type ParallelOptions struct {
	// Workers is the number of goroutines. If 0, defaults to runtime.NumCPU().
	Workers int

	// BufferSize is the channel buffer size for results.
	BufferSize int

	// ErrorStrategy controls error handling:
	//   "fail-fast"   — stop on first error, return immediately.
	//   "collect-all" — continue processing, collect all errors.
	ErrorStrategy string
}

// processorFunc is the function signature for resource processors.
type processorFunc func(*types.ExtractedResource) (*types.ProcessedResource, error)

// indexedResult carries a processed result together with its original index
// so that results can be placed back in order.
type indexedResult struct {
	index  int
	result *types.ProcessedResource
	err    error
}

// ProcessParallel runs processor over every resource in resources using a worker
// pool, and returns results in the same order as the input slice.
//
// Rules:
//   - nil processor → immediate error
//   - empty resources → empty results, no error
//   - Workers == 0   → runtime.NumCPU() workers
//   - "fail-fast"    → return on first error
//   - "collect-all"  → continue, aggregate all errors
func ProcessParallel(
	resources []*types.ExtractedResource,
	processor processorFunc,
	opts ParallelOptions,
) ([]*types.ProcessedResource, error) {
	if processor == nil {
		return nil, errors.New("ProcessParallel: processor must not be nil")
	}
	if len(resources) == 0 {
		return []*types.ProcessedResource{}, nil
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > len(resources) {
		workers = len(resources)
	}

	bufSize := opts.BufferSize
	if bufSize <= 0 {
		bufSize = len(resources)
	}

	// jobCh sends (index, resource) pairs to workers.
	type job struct {
		index    int
		resource *types.ExtractedResource
	}
	jobCh := make(chan job, bufSize)

	// resultCh collects indexed results from workers.
	resultCh := make(chan indexedResult, bufSize)

	// errCh carries the first error in fail-fast mode.
	errCh := make(chan struct{}, 1)
	failFast := opts.ErrorStrategy == "fail-fast"

	// doneCh is closed when fail-fast is triggered so workers can stop.
	doneCh := make(chan struct{})
	var doneOnce sync.Once
	signalDone := func() {
		doneOnce.Do(func() { close(doneCh) })
	}

	// Launch workers.
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := range jobCh {
				// Respect fail-fast cancellation.
				select {
				case <-doneCh:
					// Drain remaining jobs without processing.
					continue
				default:
				}

				res, err := processor(j.resource)
				resultCh <- indexedResult{index: j.index, result: res, err: err}

				if err != nil && failFast {
					select {
					case errCh <- struct{}{}:
					default:
					}
					signalDone()
				}
			}
		}()
	}

	// Feed jobs.
	go func() {
		for i, r := range resources {
			select {
			case <-doneCh:
				// Drain remaining resources so workers can exit.
				for range resources[i+1:] {
					// nothing
				}
				break
			default:
				jobCh <- job{index: i, resource: r}
			}
		}
		close(jobCh)
	}()

	// Close resultCh once all workers are done.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results.
	ordered := make([]*types.ProcessedResource, len(resources))
	var errs []error

	for ir := range resultCh {
		if ir.err != nil {
			errs = append(errs, ir.err)
			if failFast {
				// Signal done so remaining workers stop.
				signalDone()
			}
		} else if ir.result != nil {
			ordered[ir.index] = ir.result
		}
	}

	if len(errs) > 0 {
		return ordered, joinErrors(errs)
	}
	return ordered, nil
}

// joinErrors combines multiple errors into one.
func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	combined := msgs[0]
	for _, m := range msgs[1:] {
		combined += "\n" + m
	}
	return fmt.Errorf("%s", combined)
}
