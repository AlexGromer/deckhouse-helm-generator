package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ConftestOptions configures conftest policy generation.
type ConftestOptions struct {
	// PolicyDir is the directory where policy files are written (e.g., "policy").
	PolicyDir string
	// Namespace is the OPA/Rego namespace (e.g., "main").
	Namespace string
	// Policies is the list of policy names to generate.
	// Empty defaults to all 5 built-in policies.
	Policies []string
}

// ConftestResult contains generated conftest policy files and commands.
type ConftestResult struct {
	// PolicyFiles maps file path to Rego policy content.
	PolicyFiles map[string]string
	// Commands is a list of conftest commands to run.
	Commands []string
	// NOTESTxt contains usage instructions.
	NOTESTxt string
}

// allBuiltinPolicies is the list of all built-in conftest policy names.
var allBuiltinPolicies = []string{
	"deny-privileged",
	"require-labels",
	"require-resources",
	"require-probes",
	"deny-latest-tag",
}

// GenerateConftestPolicies generates conftest Rego policy files for the given resource graph.
// Returns nil if graph is nil.
func GenerateConftestPolicies(graph *types.ResourceGraph, opts ConftestOptions) *ConftestResult {
	if graph == nil {
		return nil
	}

	policyDir := opts.PolicyDir
	if policyDir == "" {
		policyDir = "policy"
	}

	ns := opts.Namespace
	if ns == "" {
		ns = "main"
	}

	policies := opts.Policies
	if len(policies) == 0 {
		policies = allBuiltinPolicies
	}

	policyFiles := make(map[string]string, len(policies))

	for _, name := range policies {
		path := fmt.Sprintf("%s/%s.rego", policyDir, name)
		content := generateConftestPolicy(name, ns)
		policyFiles[path] = content
	}

	commands := []string{
		fmt.Sprintf("conftest test --policy %s templates/", policyDir),
	}

	notesTxt := fmt.Sprintf(
		"Conftest policy validation\nPolicies: %s\nPolicy directory: %s\n\nRun:\n  %s\n\nInstall conftest:\n  https://www.conftest.dev/install/\n",
		strings.Join(policies, ", "),
		policyDir,
		strings.Join(commands, "\n  "),
	)

	return &ConftestResult{
		PolicyFiles: policyFiles,
		Commands:    commands,
		NOTESTxt:    notesTxt,
	}
}

// generateConftestPolicy generates a Rego policy for the given policy name and namespace.
func generateConftestPolicy(name, ns string) string {
	switch name {
	case "deny-privileged":
		return fmt.Sprintf(`package %s

# Deny privileged containers
deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  c.securityContext.privileged == true
  msg := sprintf("Container '%%s' must not run as privileged", [c.name])
}
`, ns)
	case "require-labels":
		return fmt.Sprintf(`package %s

# Require standard labels
required_labels := {"app.kubernetes.io/name"}

deny[msg] {
  provided := {label | input.metadata.labels[label]}
  missing := required_labels - provided
  count(missing) > 0
  msg := sprintf("Resource is missing required labels: %%v", [missing])
}
`, ns)
	case "require-resources":
		return fmt.Sprintf(`package %s

# Require resource limits and requests
deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  not c.resources.limits
  msg := sprintf("Container '%%s' must specify resource limits", [c.name])
}

deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  not c.resources.requests
  msg := sprintf("Container '%%s' must specify resource requests", [c.name])
}
`, ns)
	case "require-probes":
		return fmt.Sprintf(`package %s

# Require readiness and liveness probes
deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  not c.readinessProbe
  msg := sprintf("Container '%%s' must define a readinessProbe", [c.name])
}

deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  not c.livenessProbe
  msg := sprintf("Container '%%s' must define a livenessProbe", [c.name])
}
`, ns)
	case "deny-latest-tag":
		return fmt.Sprintf(`package %s

# Deny containers using the :latest image tag
deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  endswith(c.image, ":latest")
  msg := sprintf("Container '%%s' must not use :latest image tag", [c.name])
}

deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  not contains(c.image, ":")
  msg := sprintf("Container '%%s' image must specify a version tag (not :latest)", [c.name])
}
`, ns)
	default:
		return fmt.Sprintf(`package %s

# Policy: %s
deny[msg] {
  false
  msg := "policy not implemented"
}
`, ns, name)
	}
}

// InjectConftestPolicies injects conftest policy files into the chart using
// copy-on-write semantics. Returns nil, 0 if chart is nil.
func InjectConftestPolicies(chart *types.GeneratedChart, result *ConftestResult) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}
	if result == nil {
		return chart, 0
	}

	newChart := copyChartTemplates(chart)

	// Copy ExternalFiles
	newExtFiles := make([]types.ExternalFileInfo, len(chart.ExternalFiles))
	copy(newExtFiles, chart.ExternalFiles)

	count := 0
	for path, content := range result.PolicyFiles {
		newExtFiles = append(newExtFiles, types.ExternalFileInfo{
			Path:    path,
			Content: content,
		})
		count++
	}

	newChart.ExternalFiles = newExtFiles
	return newChart, count
}
