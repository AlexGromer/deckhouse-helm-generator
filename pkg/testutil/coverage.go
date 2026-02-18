package testutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// AssertCoverage asserts that a package has at least the minimum coverage percentage
// Runs `go test -cover` for the specified package and checks the coverage
func AssertCoverage(t *testing.T, pkg string, minCoverage float64) {
	t.Helper()

	// Run go test with coverage
	cmd := exec.Command("go", "test", "-cover", "-coverprofile=coverage.out", pkg)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to run coverage for %s: %v\nStderr: %s", pkg, err, stderr.String())
	}

	// Parse coverage output
	output := stdout.String()
	coverage := parseCoverageOutput(output)

	if coverage < 0 {
		t.Fatalf("Failed to parse coverage output for %s: %s", pkg, output)
	}

	if coverage < minCoverage {
		t.Errorf("Package %s coverage %.1f%% is below minimum %.1f%%", pkg, coverage, minCoverage)
	} else {
		t.Logf("✅ Package %s coverage: %.1f%% (≥ %.1f%%)", pkg, coverage, minCoverage)
	}
}

// AssertCoverageRange asserts that coverage is within a range
func AssertCoverageRange(t *testing.T, pkg string, minCoverage, maxCoverage float64) {
	t.Helper()

	cmd := exec.Command("go", "test", "-cover", "-coverprofile=coverage.out", pkg)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to run coverage for %s: %v", pkg, err)
	}

	coverage := parseCoverageOutput(stdout.String())

	if coverage < 0 {
		t.Fatalf("Failed to parse coverage for %s", pkg)
	}

	if coverage < minCoverage {
		t.Errorf("Coverage %.1f%% below minimum %.1f%%", coverage, minCoverage)
	}

	if coverage > maxCoverage {
		t.Errorf("Coverage %.1f%% above maximum %.1f%% (possibly over-mocked tests)", coverage, maxCoverage)
	}
}

// GetPackageCoverage returns the coverage percentage for a package
// Returns -1 if coverage cannot be determined
func GetPackageCoverage(pkg string) float64 {
	cmd := exec.Command("go", "test", "-cover", "-coverprofile=coverage.out", pkg)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return -1
	}

	return parseCoverageOutput(stdout.String())
}

// parseCoverageOutput extracts coverage percentage from go test output
// Example output: "coverage: 75.3% of statements"
func parseCoverageOutput(output string) float64 {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "coverage:") {
			// Extract percentage: "coverage: 75.3% of statements"
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "coverage:" && i+1 < len(parts) {
					coverageStr := strings.TrimSuffix(parts[i+1], "%")
					if coverage, err := strconv.ParseFloat(coverageStr, 64); err == nil {
						return coverage
					}
				}
			}
		}
	}
	return -1
}

// RequireMinCoverage is like AssertCoverage but fails the test immediately (t.Fatal)
func RequireMinCoverage(t *testing.T, pkg string, minCoverage float64) {
	t.Helper()

	coverage := GetPackageCoverage(pkg)
	if coverage < 0 {
		t.Fatalf("Failed to get coverage for package %s", pkg)
	}

	if coverage < minCoverage {
		t.Fatalf("BLOCKING: Package %s coverage %.1f%% < required %.1f%%", pkg, coverage, minCoverage)
	}

	t.Logf("✅ Coverage requirement met: %.1f%% ≥ %.1f%%", coverage, minCoverage)
}

// CoverageReport generates a coverage report for multiple packages
type CoverageReport struct {
	Packages []PackageCoverage
}

// PackageCoverage holds coverage info for a single package
type PackageCoverage struct {
	Package  string
	Coverage float64
	Pass     bool
	MinRequired float64
}

// GenerateCoverageReport generates a coverage report for multiple packages
func GenerateCoverageReport(packages map[string]float64) *CoverageReport {
	report := &CoverageReport{
		Packages: make([]PackageCoverage, 0, len(packages)),
	}

	for pkg, minCoverage := range packages {
		coverage := GetPackageCoverage(pkg)
		report.Packages = append(report.Packages, PackageCoverage{
			Package:     pkg,
			Coverage:    coverage,
			Pass:        coverage >= minCoverage,
			MinRequired: minCoverage,
		})
	}

	return report
}

// String returns a formatted coverage report
func (r *CoverageReport) String() string {
	var buf bytes.Buffer
	buf.WriteString("Coverage Report:\n")
	buf.WriteString("================\n\n")

	for _, pkg := range r.Packages {
		status := "✅"
		if !pkg.Pass {
			status = "❌"
		}
		buf.WriteString(fmt.Sprintf("%s %s: %.1f%% (required: %.1f%%)\n",
			status, pkg.Package, pkg.Coverage, pkg.MinRequired))
	}

	return buf.String()
}

// AllPass returns true if all packages meet their coverage requirements
func (r *CoverageReport) AllPass() bool {
	for _, pkg := range r.Packages {
		if !pkg.Pass {
			return false
		}
	}
	return true
}
