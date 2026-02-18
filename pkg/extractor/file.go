package extractor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// FileExtractor extracts Kubernetes resources from YAML files.
type FileExtractor struct{}

// NewFileExtractor creates a new file extractor.
func NewFileExtractor() *FileExtractor {
	return &FileExtractor{}
}

// Source returns the source type.
func (e *FileExtractor) Source() types.Source {
	return types.SourceFile
}

// Validate checks if the paths exist and are readable.
func (e *FileExtractor) Validate(ctx context.Context, opts Options) error {
	if len(opts.Paths) == 0 {
		return fmt.Errorf("at least one path is required")
	}

	for _, path := range opts.Paths {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot access path %s: %w", path, err)
		}

		if !info.IsDir() {
			// Check if it's a valid YAML file
			if !isYAMLFile(path) {
				return fmt.Errorf("file %s is not a YAML file", path)
			}
		}
	}

	return nil
}

// Extract extracts resources from YAML files.
func (e *FileExtractor) Extract(ctx context.Context, opts Options) (<-chan *types.ExtractedResource, <-chan error) {
	resources := make(chan *types.ExtractedResource, 100)
	errors := make(chan error, 10)

	go func() {
		defer close(resources)
		defer close(errors)

		for _, path := range opts.Paths {
			if err := ctx.Err(); err != nil {
				errors <- err
				return
			}

			if err := e.extractPath(ctx, path, opts, resources, errors); err != nil {
				errors <- err
			}
		}
	}()

	return resources, errors
}

func (e *FileExtractor) extractPath(ctx context.Context, path string, opts Options, resources chan<- *types.ExtractedResource, errors chan<- error) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot stat %s: %w", path, err)
	}

	if info.IsDir() {
		return e.extractDirectory(ctx, path, opts, resources, errors)
	}

	return e.extractFile(ctx, path, opts, resources, errors)
}

func (e *FileExtractor) extractDirectory(ctx context.Context, dir string, opts Options, resources chan<- *types.ExtractedResource, errors chan<- error) error {
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			errors <- fmt.Errorf("error walking %s: %w", path, err)
			return nil // Continue walking
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip directories if not recursive
		if info.IsDir() {
			if !opts.Recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process YAML files
		if !isYAMLFile(path) {
			return nil
		}

		if err := e.extractFile(ctx, path, opts, resources, errors); err != nil {
			errors <- err
		}

		return nil
	}

	return filepath.Walk(dir, walkFn)
}

func (e *FileExtractor) extractFile(ctx context.Context, path string, opts Options, resources chan<- *types.ExtractedResource, errors chan<- error) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", path, err)
	}
	defer file.Close()

	return e.parseYAMLStream(ctx, file, path, opts, resources, errors)
}

func (e *FileExtractor) parseYAMLStream(ctx context.Context, reader io.Reader, sourcePath string, opts Options, resources chan<- *types.ExtractedResource, errors chan<- error) error {
	// Read all content
	content, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", sourcePath, err)
	}

	// Split by YAML document separator
	documents := splitYAMLDocuments(content)

	for _, doc := range documents {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}

		// Skip comments-only documents
		if isCommentOnly(doc) {
			continue
		}

		obj := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(doc, &obj.Object); err != nil {
			errors <- fmt.Errorf("cannot parse YAML in %s: %w", sourcePath, err)
			continue
		}

		// Skip empty objects
		if obj.Object == nil || len(obj.Object) == 0 {
			continue
		}

		// Skip if apiVersion or kind is missing
		if obj.GetAPIVersion() == "" || obj.GetKind() == "" {
			continue
		}

		gvk := obj.GroupVersionKind()

		// Filter by kinds if specified
		if !e.matchesKindFilters(gvk.Kind, opts) {
			continue
		}

		// Filter by namespace if specified
		if !e.matchesNamespaceFilters(obj.GetNamespace(), opts) {
			continue
		}

		resource := &types.ExtractedResource{
			Object:     obj,
			Source:     types.SourceFile,
			SourcePath: sourcePath,
			GVK:        gvk,
		}

		select {
		case resources <- resource:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (e *FileExtractor) matchesKindFilters(kind string, opts Options) bool {
	// Check exclude list first
	for _, excluded := range opts.ExcludeKinds {
		if strings.EqualFold(kind, excluded) {
			return false
		}
	}

	// If include list is empty, include all (that weren't excluded)
	if len(opts.IncludeKinds) == 0 {
		return true
	}

	// Check include list
	for _, included := range opts.IncludeKinds {
		if strings.EqualFold(kind, included) {
			return true
		}
	}

	return false
}

func (e *FileExtractor) matchesNamespaceFilters(namespace string, opts Options) bool {
	// If no namespace filters, include all
	if opts.Namespace == "" && len(opts.Namespaces) == 0 {
		return true
	}

	// Check single namespace filter
	if opts.Namespace != "" {
		if namespace == opts.Namespace {
			return true
		}
		// Cluster-scoped resources (empty namespace) are included if filtering
		if namespace == "" {
			return true
		}
		return false
	}

	// Check multiple namespaces
	for _, ns := range opts.Namespaces {
		if namespace == ns {
			return true
		}
	}

	// Include cluster-scoped resources
	if namespace == "" {
		return true
	}

	return false
}

// splitYAMLDocuments splits YAML content by document separators (---).
func splitYAMLDocuments(content []byte) [][]byte {
	var documents [][]byte
	var currentDoc bytes.Buffer

	scanner := bufio.NewScanner(bytes.NewReader(content))
	// Increase buffer size for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			if currentDoc.Len() > 0 {
				documents = append(documents, bytes.Clone(currentDoc.Bytes()))
				currentDoc.Reset()
			}
		} else {
			currentDoc.WriteString(line)
			currentDoc.WriteString("\n")
		}
	}

	// Don't forget the last document
	if currentDoc.Len() > 0 {
		documents = append(documents, currentDoc.Bytes())
	}

	return documents
}

// isYAMLFile checks if a file has a YAML extension.
func isYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// isCommentOnly checks if a YAML document contains only comments and whitespace.
func isCommentOnly(doc []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(doc))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			return false
		}
	}
	return true
}

// Helper function for reading GVK from raw YAML without full unmarshal.
func ParseGVK(data []byte) (schema.GroupVersionKind, error) {
	var meta struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}

	if err := yaml.Unmarshal(data, &meta); err != nil {
		return schema.GroupVersionKind{}, err
	}

	gv, err := schema.ParseGroupVersion(meta.APIVersion)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}

	return gv.WithKind(meta.Kind), nil
}
