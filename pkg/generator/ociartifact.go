package generator

import (
	"strings"
)

// OCIAnnotationOptions configures OCI image annotation generation.
type OCIAnnotationOptions struct {
	// Title is the human-readable name of the image (org.opencontainers.image.title).
	Title string

	// Description is a short description of the image (org.opencontainers.image.description).
	Description string

	// Version is the version of the packaged software (org.opencontainers.image.version).
	Version string

	// Source is the URL to get source code for the image (org.opencontainers.image.source).
	Source string

	// Vendor is the name of the distributing entity (org.opencontainers.image.vendor).
	Vendor string

	// Authors is a list of image authors (org.opencontainers.image.authors).
	Authors []string

	// Licenses is a list of SPDX license expressions (org.opencontainers.image.licenses).
	Licenses []string
}

// OCIAnnotationResult contains the generated OCI annotations.
type OCIAnnotationResult struct {
	// Annotations maps OCI annotation keys to their values.
	Annotations map[string]string

	// Labels is an alias suitable for Docker labels or Helm chart labels.
	Labels map[string]string

	// NOTESTxt describes how to use OCI annotations.
	NOTESTxt string
}

// GenerateOCIAnnotations generates org.opencontainers.image.* annotations
// from the provided options. Never returns nil.
func GenerateOCIAnnotations(opts OCIAnnotationOptions) *OCIAnnotationResult {
	annotations := make(map[string]string)

	if opts.Title != "" {
		annotations["org.opencontainers.image.title"] = opts.Title
	}
	if opts.Description != "" {
		annotations["org.opencontainers.image.description"] = opts.Description
	}
	if opts.Version != "" {
		annotations["org.opencontainers.image.version"] = opts.Version
	}
	if opts.Source != "" {
		annotations["org.opencontainers.image.source"] = opts.Source
	}
	if opts.Vendor != "" {
		annotations["org.opencontainers.image.vendor"] = opts.Vendor
	}
	if len(opts.Authors) > 0 {
		annotations["org.opencontainers.image.authors"] = strings.Join(opts.Authors, ", ")
	}
	if len(opts.Licenses) > 0 {
		annotations["org.opencontainers.image.licenses"] = strings.Join(opts.Licenses, " AND ")
	}

	// Build a labels copy (same key-value pairs, suitable for Docker/Helm).
	labels := make(map[string]string, len(annotations))
	for k, v := range annotations {
		labels[k] = v
	}

	notesTxt := `OCI Image Annotations
=====================

These annotations conform to the OCI Image Spec (https://specs.opencontainers.org/image-spec/annotations/).

Usage
-----
In a Dockerfile:
  LABEL org.opencontainers.image.title="My App"
  LABEL org.opencontainers.image.version="1.0.0"

In a Helm chart (values.yaml):
  podAnnotations:
    org.opencontainers.image.source: "https://github.com/example/my-app"

With docker build:
  docker build \
    --label org.opencontainers.image.title="My App" \
    --label org.opencontainers.image.version="1.0.0" \
    .

Standard keys
-------------
  org.opencontainers.image.title       — Human-readable name
  org.opencontainers.image.description — Short description
  org.opencontainers.image.version     — Software version
  org.opencontainers.image.source      — Source repository URL
  org.opencontainers.image.vendor      — Distributing entity
  org.opencontainers.image.authors     — Image authors
  org.opencontainers.image.licenses    — SPDX license expression(s)

For the full specification see https://specs.opencontainers.org/image-spec/annotations/
`

	return &OCIAnnotationResult{
		Annotations: annotations,
		Labels:      labels,
		NOTESTxt:    notesTxt,
	}
}
