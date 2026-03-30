package generator

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"sync"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ResourcePool is a concurrent-safe pool of *types.ProcessedResource objects
// backed by sync.Pool to reduce allocations in hot paths.
type ResourcePool struct {
	pool sync.Pool
}

// NewResourcePool creates a new ResourcePool with a factory that allocates fresh
// *types.ProcessedResource values.
func NewResourcePool() *ResourcePool {
	return &ResourcePool{
		pool: sync.Pool{
			New: func() interface{} {
				return &types.ProcessedResource{}
			},
		},
	}
}

// Get retrieves a *types.ProcessedResource from the pool (or allocates a new one).
// Always returns a non-nil pointer.
func (p *ResourcePool) Get() *types.ProcessedResource {
	r, _ := p.pool.Get().(*types.ProcessedResource)
	if r == nil {
		r = &types.ProcessedResource{}
	}
	return r
}

// Put returns a *types.ProcessedResource to the pool so it can be reused.
// Passing nil is safe and is a no-op.
func (p *ResourcePool) Put(r *types.ProcessedResource) {
	if r == nil {
		return
	}
	// Reset the object before returning it to the pool so the next caller
	// does not see stale state.
	*r = types.ProcessedResource{}
	p.pool.Put(r)
}

// yamlSeparator is the standard YAML document separator.
var yamlSeparator = []byte("---")

// StreamYAMLDocuments reads YAML documents separated by "---" lines from reader
// and calls handler for each document. Streaming stops immediately if handler
// returns an error.
//
// Behaviour:
//   - nil reader → returns an error immediately (no panic)
//   - empty input → calls handler 0 times, returns nil
//   - handler error → propagated immediately, streaming stops
func StreamYAMLDocuments(reader io.Reader, handler func([]byte) error) error {
	if reader == nil {
		return errors.New("StreamYAMLDocuments: reader must not be nil")
	}

	scanner := bufio.NewScanner(reader)
	// Increase buffer capacity for large documents.
	const maxBuf = 4 * 1024 * 1024 // 4 MiB
	scanner.Buffer(make([]byte, 64*1024), maxBuf)

	var buf bytes.Buffer

	flushDoc := func() error {
		doc := bytes.TrimSpace(buf.Bytes())
		buf.Reset()
		if len(doc) == 0 {
			return nil
		}
		docCopy := make([]byte, len(doc))
		copy(docCopy, doc)
		return handler(docCopy)
	}

	for scanner.Scan() {
		line := scanner.Bytes()

		if bytes.Equal(bytes.TrimRight(line, "\r"), yamlSeparator) {
			if err := flushDoc(); err != nil {
				return err
			}
			continue
		}

		buf.Write(line)
		buf.WriteByte('\n')
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Flush last document (no trailing "---").
	return flushDoc()
}
