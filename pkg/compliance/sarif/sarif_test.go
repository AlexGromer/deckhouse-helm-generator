package sarif

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/compliance/pss"
)

func TestFormatSARIF(t *testing.T) {
	report := Report{
		ToolName:    "dhg",
		ToolVersion: "v1.0.0",
		Findings: []Finding{
			{
				RuleID:   "HostProcess",
				Level:    "error",
				Message:  "Violation found",
				Resource: "Pod/test",
				FilePath: "templates/pod.yaml",
				Line:     10,
			},
		},
	}
	out, err := FormatSARIF(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"name": "dhg"`) {
		t.Errorf("expected dhg in output, got %s", s)
	}
	if !strings.Contains(s, `"ruleId": "HostProcess"`) {
		t.Errorf("expected ruleId HostProcess, got %s", s)
	}
}

func TestFormatHTML(t *testing.T) {
	report := Report{
		ToolName:    "dhg",
		ToolVersion: "v1.0.0",
		Findings: []Finding{
			{
				RuleID:   "HostProcess",
				Level:    "error",
				Message:  "Violation found",
				Resource: "Pod/test",
			},
		},
	}
	out, err := FormatHTML(report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "Audit Report - dhg v1.0.0") {
		t.Errorf("expected title in output")
	}
	if !strings.Contains(s, "HostProcess") {
		t.Errorf("expected HostProcess in output")
	}
}

func TestFromPSSResults(t *testing.T) {
	res := []WorkloadResult{
		{
			Kind:    "Pod",
			Name:    "test-pod",
			Profile: pss.ProfileBaseline,
			Findings: []pss.Finding{
				{
					Profile: pss.ProfileRestricted,
					Control: "Volume Types",
					Field:   "spec.volumes",
					Value:   "hostPath",
				},
				{
					Profile: pss.ProfileBaseline,
					Control: "HostProcess",
					Field:   "spec.securityContext",
					Value:   "true",
				},
			},
		},
	}

	findings := FromPSSResults(res)
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}

	// Test restricted -> warning
	if findings[0].RuleID != "Volume Types" {
		t.Errorf("expected Volume Types, got %v", findings[0].RuleID)
	}
	if findings[0].Level != "warning" {
		t.Errorf("expected warning, got %v", findings[0].Level)
	}

	// Test baseline -> error
	if findings[1].RuleID != "HostProcess" {
		t.Errorf("expected HostProcess, got %v", findings[1].RuleID)
	}
	if findings[1].Level != "error" {
		t.Errorf("expected error, got %v", findings[1].Level)
	}
}
