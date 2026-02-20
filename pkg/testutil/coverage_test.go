package testutil

import (
	"testing"
)

func TestParseCoverageOutput_Valid(t *testing.T) {
	output := `ok  	github.com/example/pkg	0.5s	coverage: 75.3% of statements`
	cov := parseCoverageOutput(output)
	if cov < 75.0 || cov > 76.0 {
		t.Errorf("expected ~75.3, got %f", cov)
	}
}

func TestParseCoverageOutput_MultiLine(t *testing.T) {
	output := `=== RUN   TestFoo
--- PASS: TestFoo (0.00s)
ok  	github.com/example/pkg	0.5s	coverage: 92.1% of statements`
	cov := parseCoverageOutput(output)
	if cov < 92.0 || cov > 93.0 {
		t.Errorf("expected ~92.1, got %f", cov)
	}
}

func TestParseCoverageOutput_NoCoverage(t *testing.T) {
	output := `ok  	github.com/example/pkg	0.5s`
	cov := parseCoverageOutput(output)
	if cov != -1 {
		t.Errorf("expected -1, got %f", cov)
	}
}

func TestParseCoverageOutput_Empty(t *testing.T) {
	cov := parseCoverageOutput("")
	if cov != -1 {
		t.Errorf("expected -1 for empty input, got %f", cov)
	}
}

func TestCoverageReport_String(t *testing.T) {
	report := &CoverageReport{
		Packages: []PackageCoverage{
			{Package: "pkg/a", Coverage: 85.0, Pass: true, MinRequired: 80.0},
			{Package: "pkg/b", Coverage: 50.0, Pass: false, MinRequired: 70.0},
		},
	}

	str := report.String()
	if str == "" {
		t.Fatal("expected non-empty string")
	}
	if len(str) < 10 {
		t.Error("expected meaningful report string")
	}
}

func TestCoverageReport_AllPass_True(t *testing.T) {
	report := &CoverageReport{
		Packages: []PackageCoverage{
			{Pass: true},
			{Pass: true},
		},
	}
	if !report.AllPass() {
		t.Error("expected AllPass=true")
	}
}

func TestCoverageReport_AllPass_False(t *testing.T) {
	report := &CoverageReport{
		Packages: []PackageCoverage{
			{Pass: true},
			{Pass: false},
		},
	}
	if report.AllPass() {
		t.Error("expected AllPass=false")
	}
}

func TestCoverageReport_AllPass_Empty(t *testing.T) {
	report := &CoverageReport{
		Packages: []PackageCoverage{},
	}
	if !report.AllPass() {
		t.Error("expected AllPass=true for empty report")
	}
}
