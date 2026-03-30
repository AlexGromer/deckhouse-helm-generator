package generator

import (
	"encoding/json"
	"strings"
	"testing"
)

// ── Test Plan ──────────────────────────────────────────────────────────────────
// 1. provenance JSON is generated (not empty)
// 2. builder ID present in output
// 3. source URI present in output
// 4. build type present in output
// 5. level 2 build metadata fields present
// 6. nil/zero opts produce sane defaults (no panic)
// 7. NOTESTxt is populated
// 8. JSON structure is valid (unmarshalable)

func TestSLSAProvenance_Generated(t *testing.T) {
	opts := SLSAOptions{
		BuilderID: "https://github.com/slsa-framework/slsa-github-generator/.github/workflows/builder_go_slsa3.yml@refs/tags/v1.0.0",
		SourceURI: "git+https://github.com/example/repo@refs/heads/main",
		BuildType: "https://slsa.dev/provenance/v0.2",
		Level:     2,
	}

	result := GenerateSLSAProvenance(opts)

	if result == nil {
		t.Fatal("GenerateSLSAProvenance returned nil")
	}
	if len(result.ProvenanceFiles) == 0 {
		t.Error("ProvenanceFiles must not be empty — at least one provenance file expected")
	}
}

func TestSLSAProvenance_BuilderIDInOutput(t *testing.T) {
	builderID := "https://github.com/slsa-framework/slsa-github-generator@v1.5.0"
	opts := SLSAOptions{
		BuilderID: builderID,
		SourceURI: "git+https://github.com/example/repo",
		BuildType: "https://slsa.dev/provenance/v0.2",
		Level:     2,
	}

	result := GenerateSLSAProvenance(opts)
	if result == nil {
		t.Fatal("GenerateSLSAProvenance returned nil")
	}

	found := false
	for _, content := range result.ProvenanceFiles {
		if strings.Contains(content, builderID) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("builder ID %q not found in any provenance file", builderID)
	}
}

func TestSLSAProvenance_SourceURIInOutput(t *testing.T) {
	sourceURI := "git+https://github.com/example/my-app@refs/heads/main"
	opts := SLSAOptions{
		BuilderID: "https://example.com/builder",
		SourceURI: sourceURI,
		BuildType: "https://slsa.dev/provenance/v0.2",
		Level:     2,
	}

	result := GenerateSLSAProvenance(opts)
	if result == nil {
		t.Fatal("GenerateSLSAProvenance returned nil")
	}

	found := false
	for _, content := range result.ProvenanceFiles {
		if strings.Contains(content, sourceURI) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("source URI %q not found in any provenance file", sourceURI)
	}
}

func TestSLSAProvenance_BuildTypeInOutput(t *testing.T) {
	buildType := "https://slsa.dev/provenance/v0.2"
	opts := SLSAOptions{
		BuilderID: "https://example.com/builder",
		SourceURI: "git+https://github.com/example/repo",
		BuildType: buildType,
		Level:     2,
	}

	result := GenerateSLSAProvenance(opts)
	if result == nil {
		t.Fatal("GenerateSLSAProvenance returned nil")
	}

	found := false
	for _, content := range result.ProvenanceFiles {
		if strings.Contains(content, buildType) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("build type %q not found in any provenance file", buildType)
	}
}

func TestSLSAProvenance_Level2Metadata(t *testing.T) {
	// SLSA Level 2+ must include builder.id and materials (source)
	opts := SLSAOptions{
		BuilderID: "https://example.com/builder@v1.0",
		SourceURI: "git+https://github.com/example/repo@abc123",
		BuildType: "https://slsa.dev/provenance/v0.2",
		Level:     2,
	}

	result := GenerateSLSAProvenance(opts)
	if result == nil {
		t.Fatal("GenerateSLSAProvenance returned nil")
	}

	// At Level 2, the provenance must contain builder and material fields
	combined := ""
	for _, content := range result.ProvenanceFiles {
		combined += content
	}

	// in-toto / SLSA provenance expected fields
	for _, required := range []string{"builder", "buildType", "materials"} {
		if !strings.Contains(combined, required) {
			t.Errorf("level-2 provenance missing required field %q", required)
		}
	}
}

func TestSLSAProvenance_ZeroOptsDefaults(t *testing.T) {
	// Passing zero-value opts must not panic and must return a result
	opts := SLSAOptions{}

	var result *SLSAResult
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("GenerateSLSAProvenance panicked with zero opts: %v", r)
		}
	}()

	result = GenerateSLSAProvenance(opts)
	if result == nil {
		t.Fatal("GenerateSLSAProvenance returned nil for zero opts")
	}
}

func TestSLSAProvenance_NOTESTxt(t *testing.T) {
	opts := SLSAOptions{
		BuilderID: "https://example.com/builder",
		SourceURI: "git+https://github.com/example/repo",
		BuildType: "https://slsa.dev/provenance/v0.2",
		Level:     2,
	}

	result := GenerateSLSAProvenance(opts)
	if result == nil {
		t.Fatal("GenerateSLSAProvenance returned nil")
	}

	if strings.TrimSpace(result.NOTESTxt) == "" {
		t.Error("NOTESTxt must not be empty — should describe how to verify provenance")
	}
}

func TestSLSAProvenance_JSONValidStructure(t *testing.T) {
	opts := SLSAOptions{
		BuilderID: "https://example.com/builder@v1.0",
		SourceURI: "git+https://github.com/example/repo",
		BuildType: "https://slsa.dev/provenance/v0.2",
		Level:     2,
	}

	result := GenerateSLSAProvenance(opts)
	if result == nil {
		t.Fatal("GenerateSLSAProvenance returned nil")
	}

	// At least one provenance file must be valid JSON
	jsonFound := false
	for name, content := range result.ProvenanceFiles {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		jsonFound = true
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(content), &parsed); err != nil {
			t.Errorf("provenance file %q contains invalid JSON: %v", name, err)
		}
	}
	if !jsonFound {
		t.Error("no .json provenance file found — SLSA provenance must be a JSON file")
	}
}
