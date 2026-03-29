package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ComplianceOptions configures compliance-as-code policy generation.
type ComplianceOptions struct {
	// Engine is the policy engine: "kyverno" or "opa".
	Engine string
	// Standards is the list of compliance standards to generate policies for.
	// Supported: "cis", "pss", "labels", "resources".
	Standards []string
	// Severity is the minimum violation severity to report: "low", "medium", "high", "critical".
	Severity string
}

// ComplianceViolation describes a single compliance violation detected in the resource graph.
type ComplianceViolation struct {
	Resource string
	Standard string
	Message  string
	Severity string
}

// ComplianceResult holds the generated policies and detected violations.
type ComplianceResult struct {
	// Policies maps filename → YAML content.
	Policies map[string]string
	// Violations lists compliance violations detected in the graph.
	Violations []ComplianceViolation
	// NOTESTxt is an optional human-readable summary.
	NOTESTxt string
}

// GenerateCompliancePolicies generates compliance policies for the given resource graph
// and options. Returns a non-nil ComplianceResult even for empty graphs.
func GenerateCompliancePolicies(graph *types.ResourceGraph, opts ComplianceOptions) *ComplianceResult {
	result := &ComplianceResult{
		Policies: make(map[string]string),
	}

	for _, standard := range opts.Standards {
		policies := generatePoliciesForStandard(standard, opts.Engine)
		for k, v := range policies {
			result.Policies[k] = v
		}
	}

	if graph != nil {
		result.Violations = detectViolations(graph, opts)
	}

	return result
}

// generatePoliciesForStandard returns policies for a single standard + engine.
func generatePoliciesForStandard(standard, engine string) map[string]string {
	switch standard {
	case "cis":
		return generateCISPolicies(engine)
	case "pss":
		return generatePSSPolicies(engine)
	case "labels":
		return generateLabelsPolicies(engine)
	case "resources":
		return generateResourcesPolicies(engine)
	default:
		return map[string]string{}
	}
}

func generateCISPolicies(engine string) map[string]string {
	name := "cis-disallow-privileged"
	desc := "CIS: Deny containers running in privileged mode (CIS 5.2.1)."
	switch engine {
	case "opa":
		return map[string]string{
			"compliance/cis-disallow-privileged.yaml": opaPolicy(name, desc, "Pod",
				`      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        container.securityContext.privileged == true
        msg := sprintf("CIS 5.2.1: Privileged container not allowed: %v", [container.name])
      }`, ""),
		}
	default: // kyverno
		return map[string]string{
			"compliance/cis-disallow-privileged.yaml": kyvernoPolicy(name, desc,
				`    rules:
      - name: cis-deny-privileged-containers
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "CIS 5.2.1: Privileged containers are not allowed."
          pattern:
            spec:
              containers:
                - securityContext:
                    privileged: "false"`),
		}
	}
}

func generatePSSPolicies(engine string) map[string]string {
	name := "pss-require-non-root"
	desc := "PSS Restricted: Require runAsNonRoot and drop all capabilities. References securityContext, privileged, pss."
	switch engine {
	case "opa":
		return map[string]string{
			"compliance/pss-require-non-root.yaml": opaPolicy(name, desc, "Pod",
				`      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not container.securityContext.runAsNonRoot
        msg := sprintf("PSS Restricted: runAsNonRoot required for container: %v", [container.name])
      }
      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        container.securityContext.privileged == true
        msg := sprintf("PSS Restricted: privileged containers not allowed: %v", [container.name])
      }`, ""),
		}
	default: // kyverno
		return map[string]string{
			"compliance/pss-require-non-root.yaml": kyvernoPolicy(name, desc,
				`    rules:
      - name: pss-require-run-as-non-root
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "PSS Restricted: securityContext.runAsNonRoot must be true (pss)."
          pattern:
            spec:
              containers:
                - securityContext:
                    runAsNonRoot: "true"
      - name: pss-deny-privileged
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "PSS Restricted: privileged containers not allowed."
          pattern:
            spec:
              containers:
                - securityContext:
                    privileged: "false"`),
		}
	}
}

func generateLabelsPolicies(engine string) map[string]string {
	name := "require-app-labels"
	desc := "Require app.kubernetes.io/name and app.kubernetes.io/version labels on all Pods. label compliance."
	switch engine {
	case "opa":
		return map[string]string{
			"compliance/require-app-labels.yaml": opaPolicy(name, desc, "Pod",
				`      violation[{"msg": msg}] {
        provided := {label | input.review.object.metadata.labels[label]}
        required := {"app.kubernetes.io/name", "app.kubernetes.io/version"}
        missing := required - provided
        count(missing) > 0
        msg := sprintf("Missing required label requirements: %v", [missing])
      }`, ""),
		}
	default: // kyverno
		return map[string]string{
			"compliance/require-app-labels.yaml": kyvernoPolicy(name, desc,
				`    rules:
      - name: check-app-kubernetes-io-labels
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "Labels 'app.kubernetes.io/name' and 'app.kubernetes.io/version' are required (label compliance)."
          pattern:
            metadata:
              labels:
                app.kubernetes.io/name: "?*"
                app.kubernetes.io/version: "?*"`),
		}
	}
}

func generateResourcesPolicies(engine string) map[string]string {
	name := "require-resource-limits"
	desc := "Require resource requests and limits on all containers."
	switch engine {
	case "opa":
		return map[string]string{
			"compliance/require-resource-limits.yaml": opaPolicy(name, desc, "Pod",
				`      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not container.resources.requests
        msg := sprintf("Resource requests required for container: %v", [container.name])
      }
      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not container.resources.limits
        msg := sprintf("Resource limits required for container: %v", [container.name])
      }`, ""),
		}
	default: // kyverno
		return map[string]string{
			"compliance/require-resource-limits.yaml": kyvernoPolicy(name, desc,
				`    rules:
      - name: check-resource-limits
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "CPU and memory resource limits and requests are required."
          pattern:
            spec:
              containers:
                - resources:
                    requests:
                      memory: "?*"
                      cpu: "?*"
                    limits:
                      memory: "?*"
                      cpu: "?*"`),
		}
	}
}

// severityLevel converts severity string to numeric level for comparison.
func severityLevel(s string) int {
	switch strings.ToLower(s) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

// detectViolations scans the resource graph for compliance violations.
func detectViolations(graph *types.ResourceGraph, opts ComplianceOptions) []ComplianceViolation {
	var violations []ComplianceViolation
	minLevel := severityLevel(opts.Severity)

	for _, standard := range opts.Standards {
		for _, r := range graph.Resources {
			kind := r.Original.GVK.Kind
			if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" &&
				kind != "Pod" && kind != "Job" && kind != "CronJob" {
				continue
			}
			name := r.Original.Object.GetName()
			resourceRef := fmt.Sprintf("%s/%s", kind, name)

			switch standard {
			case "pss":
				sev := "high"
				if severityLevel(sev) >= minLevel {
					violations = append(violations, ComplianceViolation{
						Resource: resourceRef,
						Standard: standard,
						Message:  fmt.Sprintf("%s does not have securityContext.runAsNonRoot set", resourceRef),
						Severity: sev,
					})
				}
			case "resources":
				sev := "high"
				if severityLevel(sev) >= minLevel {
					violations = append(violations, ComplianceViolation{
						Resource: resourceRef,
						Standard: standard,
						Message:  fmt.Sprintf("%s does not have resource limits defined", resourceRef),
						Severity: sev,
					})
				}
			case "cis":
				sev := "high"
				if severityLevel(sev) >= minLevel {
					violations = append(violations, ComplianceViolation{
						Resource: resourceRef,
						Standard: standard,
						Message:  fmt.Sprintf("%s may be running with elevated privileges (CIS security check)", resourceRef),
						Severity: sev,
					})
				}
			case "labels":
				sev := "medium"
				if severityLevel(sev) >= minLevel {
					violations = append(violations, ComplianceViolation{
						Resource: resourceRef,
						Standard: standard,
						Message:  fmt.Sprintf("%s is missing required app.kubernetes.io labels", resourceRef),
						Severity: sev,
					})
				}
			}
		}
	}

	return violations
}

// InjectCompliancePolicies adds compliance policy files to a chart's ExternalFiles.
// Returns the updated chart (copy-on-write) and the count of newly added policies.
// Idempotent: policies already present (by path) are not added again.
func InjectCompliancePolicies(chart *types.GeneratedChart, compResult *ComplianceResult) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}

	// Build set of already-present paths.
	existing := make(map[string]bool, len(chart.ExternalFiles))
	for _, ef := range chart.ExternalFiles {
		existing[ef.Path] = true
	}

	// Copy ExternalFiles slice.
	newFiles := make([]types.ExternalFileInfo, len(chart.ExternalFiles))
	copy(newFiles, chart.ExternalFiles)

	count := 0
	if compResult != nil {
		for path, content := range compResult.Policies {
			if existing[path] {
				continue
			}
			newFiles = append(newFiles, types.ExternalFileInfo{
				Path:    path,
				Content: content,
			})
			count++
		}
	}

	result := copyChartTemplates(chart)
	result.ExternalFiles = newFiles
	return result, count
}
