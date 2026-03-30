package generator

import (
	"strings"
	"testing"
)

// ── Test Plan ──────────────────────────────────────────────────────────────────
// 1. title annotation → org.opencontainers.image.title
// 2. description annotation → org.opencontainers.image.description
// 3. version annotation → org.opencontainers.image.version
// 4. source URL annotation → org.opencontainers.image.source
// 5. vendor annotation → org.opencontainers.image.vendor
// 6. authors → org.opencontainers.image.authors
// 7. licenses → org.opencontainers.image.licenses
// 8. NOTESTxt populated

const ociTitleKey = "org.opencontainers.image.title"
const ociDescKey = "org.opencontainers.image.description"
const ociVersionKey = "org.opencontainers.image.version"
const ociSourceKey = "org.opencontainers.image.source"
const ociVendorKey = "org.opencontainers.image.vendor"
const ociAuthorsKey = "org.opencontainers.image.authors"
const ociLicensesKey = "org.opencontainers.image.licenses"

func makeFullOCIOpts() OCIAnnotationOptions {
	return OCIAnnotationOptions{
		Title:       "My Application",
		Description: "A Kubernetes application packaged with DHG",
		Version:     "1.2.3",
		Source:      "https://github.com/example/my-app",
		Vendor:      "Example Corp",
		Authors:     []string{"Alice <alice@example.com>", "Bob <bob@example.com>"},
		Licenses:    []string{"Apache-2.0", "MIT"},
	}
}

func TestOCIAnnotations_TitleAnnotation(t *testing.T) {
	opts := makeFullOCIOpts()
	result := GenerateOCIAnnotations(opts)

	if result == nil {
		t.Fatal("GenerateOCIAnnotations returned nil")
	}

	val, ok := result.Annotations[ociTitleKey]
	if !ok {
		t.Fatalf("annotation %q not found in result", ociTitleKey)
	}
	if val != opts.Title {
		t.Errorf("title annotation = %q, want %q", val, opts.Title)
	}
}

func TestOCIAnnotations_DescriptionAnnotation(t *testing.T) {
	opts := makeFullOCIOpts()
	result := GenerateOCIAnnotations(opts)

	if result == nil {
		t.Fatal("GenerateOCIAnnotations returned nil")
	}

	val, ok := result.Annotations[ociDescKey]
	if !ok {
		t.Fatalf("annotation %q not found", ociDescKey)
	}
	if val != opts.Description {
		t.Errorf("description annotation = %q, want %q", val, opts.Description)
	}
}

func TestOCIAnnotations_VersionAnnotation(t *testing.T) {
	opts := makeFullOCIOpts()
	result := GenerateOCIAnnotations(opts)

	if result == nil {
		t.Fatal("GenerateOCIAnnotations returned nil")
	}

	val, ok := result.Annotations[ociVersionKey]
	if !ok {
		t.Fatalf("annotation %q not found", ociVersionKey)
	}
	if val != opts.Version {
		t.Errorf("version annotation = %q, want %q", val, opts.Version)
	}
}

func TestOCIAnnotations_SourceURLAnnotation(t *testing.T) {
	opts := makeFullOCIOpts()
	result := GenerateOCIAnnotations(opts)

	if result == nil {
		t.Fatal("GenerateOCIAnnotations returned nil")
	}

	val, ok := result.Annotations[ociSourceKey]
	if !ok {
		t.Fatalf("annotation %q not found", ociSourceKey)
	}
	if val != opts.Source {
		t.Errorf("source annotation = %q, want %q", val, opts.Source)
	}
}

func TestOCIAnnotations_VendorAnnotation(t *testing.T) {
	opts := makeFullOCIOpts()
	result := GenerateOCIAnnotations(opts)

	if result == nil {
		t.Fatal("GenerateOCIAnnotations returned nil")
	}

	val, ok := result.Annotations[ociVendorKey]
	if !ok {
		t.Fatalf("annotation %q not found", ociVendorKey)
	}
	if val != opts.Vendor {
		t.Errorf("vendor annotation = %q, want %q", val, opts.Vendor)
	}
}

func TestOCIAnnotations_AuthorsAnnotation(t *testing.T) {
	opts := makeFullOCIOpts()
	result := GenerateOCIAnnotations(opts)

	if result == nil {
		t.Fatal("GenerateOCIAnnotations returned nil")
	}

	val, ok := result.Annotations[ociAuthorsKey]
	if !ok {
		t.Fatalf("annotation %q not found", ociAuthorsKey)
	}

	// All authors must appear in the annotation value
	for _, author := range opts.Authors {
		if !strings.Contains(val, author) {
			t.Errorf("author %q not found in authors annotation %q", author, val)
		}
	}
}

func TestOCIAnnotations_LicensesAnnotation(t *testing.T) {
	opts := makeFullOCIOpts()
	result := GenerateOCIAnnotations(opts)

	if result == nil {
		t.Fatal("GenerateOCIAnnotations returned nil")
	}

	val, ok := result.Annotations[ociLicensesKey]
	if !ok {
		t.Fatalf("annotation %q not found", ociLicensesKey)
	}

	for _, lic := range opts.Licenses {
		if !strings.Contains(val, lic) {
			t.Errorf("license %q not found in licenses annotation %q", lic, val)
		}
	}
}

func TestOCIAnnotations_NOTESTxt(t *testing.T) {
	opts := makeFullOCIOpts()
	result := GenerateOCIAnnotations(opts)

	if result == nil {
		t.Fatal("GenerateOCIAnnotations returned nil")
	}

	if strings.TrimSpace(result.NOTESTxt) == "" {
		t.Error("NOTESTxt must not be empty — should explain OCI annotation usage")
	}
}
