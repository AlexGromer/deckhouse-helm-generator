package generator

import (
	"encoding/json"
	"fmt"
)

// SLSAOptions configures SLSA provenance generation.
type SLSAOptions struct {
	// BuilderID is the URI identifying the builder entity.
	BuilderID string

	// SourceURI is the URI of the source repository (e.g. git+https://...).
	SourceURI string

	// BuildType is the URI identifying the build type schema.
	BuildType string

	// Level is the SLSA compliance level (1, 2, or 3).
	Level int
}

// SLSAResult contains generated SLSA provenance artifacts.
type SLSAResult struct {
	// ProvenanceFiles maps filename → file content (JSON).
	ProvenanceFiles map[string]string

	// NOTESTxt describes how to verify the provenance.
	NOTESTxt string
}

// slsaBuilder represents the builder section of an in-toto provenance statement.
type slsaBuilder struct {
	ID string `json:"id"`
}

// slsaMaterial represents a material (source) in the provenance.
type slsaMaterial struct {
	URI    string            `json:"uri"`
	Digest map[string]string `json:"digest,omitempty"`
}

// slsaMetadata holds build metadata.
type slsaMetadata struct {
	BuildInvocationID string `json:"buildInvocationId,omitempty"`
	Completeness      struct {
		Parameters  bool `json:"parameters"`
		Environment bool `json:"environment"`
		Materials   bool `json:"materials"`
	} `json:"completeness"`
	Reproducible bool `json:"reproducible"`
}

// slsaPredicate represents the SLSA provenance predicate.
type slsaPredicate struct {
	Builder   slsaBuilder    `json:"builder"`
	BuildType string         `json:"buildType"`
	Materials []slsaMaterial `json:"materials"`
	Metadata  slsaMetadata   `json:"metadata"`
}

// slsaSubject represents a subject of the provenance statement.
type slsaSubject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

// slsaStatement is the top-level in-toto statement.
type slsaStatement struct {
	Type          string        `json:"_type"`
	PredicateType string        `json:"predicateType"`
	Subject       []slsaSubject `json:"subject"`
	Predicate     slsaPredicate `json:"predicate"`
}

// GenerateSLSAProvenance generates an in-toto SLSA provenance JSON for the given options.
// It never panics on zero/nil input and always returns a non-nil *SLSAResult.
func GenerateSLSAProvenance(opts SLSAOptions) *SLSAResult {
	builderID := opts.BuilderID
	if builderID == "" {
		builderID = "https://github.com/slsa-framework/slsa-github-generator"
	}
	sourceURI := opts.SourceURI
	if sourceURI == "" {
		sourceURI = "git+https://github.com/unknown/repo"
	}
	buildType := opts.BuildType
	if buildType == "" {
		buildType = "https://slsa.dev/provenance/v0.2"
	}
	level := opts.Level
	if level <= 0 {
		level = 1
	}

	stmt := slsaStatement{
		Type:          "https://in-toto.io/Statement/v0.1",
		PredicateType: "https://slsa.dev/provenance/v0.2",
		Subject: []slsaSubject{
			{
				Name: "artifact",
				Digest: map[string]string{
					"sha256": "0000000000000000000000000000000000000000000000000000000000000000",
				},
			},
		},
		Predicate: slsaPredicate{
			Builder: slsaBuilder{
				ID: builderID,
			},
			BuildType: buildType,
			Materials: []slsaMaterial{
				{
					URI: sourceURI,
				},
			},
		},
	}

	raw, err := json.MarshalIndent(stmt, "", "  ")
	if err != nil {
		// Fallback: produce minimal valid JSON
		raw = []byte(fmt.Sprintf(
			`{"_type":"https://in-toto.io/Statement/v0.1","predicateType":%q,"subject":[],"predicate":{"builder":{"id":%q},"buildType":%q,"materials":[{"uri":%q}]}}`,
			"https://slsa.dev/provenance/v0.2", builderID, buildType, sourceURI,
		))
	}

	notesTxt := fmt.Sprintf(`SLSA Provenance (Level %d)
==========================

This file contains an in-toto provenance statement conforming to the SLSA
framework (https://slsa.dev).

Builder : %s
Source  : %s
Type    : %s

How to verify
-------------
1. Install slsa-verifier:
     go install github.com/slsa-framework/slsa-verifier/v2/cli/slsa-verifier@latest

2. Run:
     slsa-verifier verify-artifact <artifact> \
       --provenance-path provenance.json \
       --source-uri %s \
       --builder-id %s

For more information see https://slsa.dev/provenance/v0.2
`, level, builderID, sourceURI, buildType, sourceURI, builderID)

	return &SLSAResult{
		ProvenanceFiles: map[string]string{
			"provenance.json": string(raw),
		},
		NOTESTxt: notesTxt,
	}
}
