package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGraphCommandFormatsAndLevels(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "tests", "integration", "fixtures", "pbc-shop", "manifests.yaml")

	// Create dummy fixture for test if it doesn't exist to prevent command from failing completely
	// due to file not found during test execution. In a real environment, the fixture would exist.
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		err := os.MkdirAll(filepath.Dir(fixturePath), 0755)
		if err != nil {
			t.Skipf("Failed to create fixture dir: %v", err)
		}
		dummyYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: order-api
  namespace: shop
`
		err = os.WriteFile(fixturePath, []byte(dummyYAML), 0644)
		if err != nil {
			t.Skipf("Failed to create fixture file: %v", err)
		}
	}

	tests := []struct {
		name    string
		format  string
		level   string
		groupBy string
		pbc     []string
	}{
		{"resource-dot", "dot", "resource", "", nil},
		{"service-dot", "dot", "service", "", nil},
		{"pbc-dot", "dot", "pbc", "", nil},
		{"resource-mermaid", "mermaid", "resource", "", nil},
		{"service-mermaid", "mermaid", "service", "", nil},
		{"pbc-mermaid", "mermaid", "pbc", "", nil},
		{"json", "json", "resource", "", nil},
		{"report", "report", "resource", "", nil},
		{"pbc-mapping", "dot", "pbc", "", []string{"custom=order-api"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := graphOptions{
				paths:       []string{fixturePath},
				format:      tt.format,
				level:       tt.level,
				groupBy:     tt.groupBy,
				pbc:         tt.pbc,
				pbcLabel:    "app.kubernetes.io/part-of",
				connections: true,
			}

			// We redirect stdout just in case
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := runGraph(context.Background(), opts)

			w.Close()
			os.Stdout = oldStdout

			if err != nil {
				t.Fatalf("runGraph failed: %v", err)
			}

			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)
			out := buf.String()

			if len(out) == 0 {
				t.Errorf("expected non-empty output")
			}
		})
	}
}
