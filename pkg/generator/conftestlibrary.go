package generator

import (
	"fmt"
	"strings"
)

// ConftestLibraryOptions configures conftest policy library generation.
type ConftestLibraryOptions struct {
	// Categories is the list of policy categories to generate.
	// Valid values: "security", "resources", "labels", "networking", "storage".
	// Empty slice defaults to all categories.
	Categories []string
	// OutputDir is the directory prefix for generated policy files.
	OutputDir string
}

// ConftestLibraryResult contains generated conftest policy files.
type ConftestLibraryResult struct {
	// Policies maps file path to Rego policy content.
	Policies map[string]string
	// TotalRules is the total number of deny rules generated.
	TotalRules int
	// NOTESTxt contains usage instructions.
	NOTESTxt string
}

// allCategories is the list of all built-in categories.
var allConftestCategories = []string{"security", "resources", "labels", "networking", "storage"}

// GenerateConftestLibrary generates a conftest Rego policy library for the given categories.
// If Categories is empty, all 5 built-in categories are generated.
func GenerateConftestLibrary(opts ConftestLibraryOptions) *ConftestLibraryResult {
	categories := opts.Categories
	if len(categories) == 0 {
		categories = allConftestCategories
	}

	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = "policy"
	}

	policies := make(map[string]string)
	totalRules := 0

	for _, cat := range categories {
		switch cat {
		case "security":
			secPolicies := generateSecurityPolicies(outputDir)
			for k, v := range secPolicies {
				policies[k] = v
				totalRules++
			}
		case "resources":
			resPolicies := generateConftestResourcesPolicies(outputDir)
			for k, v := range resPolicies {
				policies[k] = v
				totalRules++
			}
		case "labels":
			lblPolicies := generateConftestLabelsPolicies(outputDir)
			for k, v := range lblPolicies {
				policies[k] = v
				totalRules++
			}
		case "networking":
			netPolicies := generateNetworkingPolicies(outputDir)
			for k, v := range netPolicies {
				policies[k] = v
				totalRules++
			}
		case "storage":
			stgPolicies := generateStoragePolicies(outputDir)
			for k, v := range stgPolicies {
				policies[k] = v
				totalRules++
			}
		}
	}

	categoryList := strings.Join(categories, ", ")
	notesTxt := fmt.Sprintf(
		"Conftest policy library\nCategories: %s\nTotal policies: %d\nTotal rules: %d\n\n"+
			"Run conftest:\n  conftest test --policy %s templates/\n\n"+
			"Install conftest:\n  https://www.conftest.dev/install/\n",
		categoryList, len(policies), totalRules, outputDir)

	return &ConftestLibraryResult{
		Policies:   policies,
		TotalRules: totalRules,
		NOTESTxt:   notesTxt,
	}
}

func generateSecurityPolicies(dir string) map[string]string {
	return map[string]string{
		dir + "/deny-privileged.rego": `package main

# Deny privileged containers
deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  c.securityContext.privileged == true
  msg := sprintf("Container '%s' must not run as privileged", [c.name])
}
`,
		dir + "/deny-root.rego": `package main

# Deny containers running as root
deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  not c.securityContext.runAsNonRoot
  msg := sprintf("Container '%s' must set runAsNonRoot=true", [c.name])
}
`,
		dir + "/deny-hostpath.rego": `package main

# Deny hostPath volume mounts
deny[msg] {
  input.kind == "Deployment"
  v := input.spec.template.spec.volumes[_]
  v.hostPath
  msg := sprintf("Volume '%s' uses hostPath which is not allowed", [v.name])
}
`,
		dir + "/deny-hostnetwork.rego": `package main

# Deny hostNetwork usage
deny[msg] {
  input.kind == "Deployment"
  input.spec.template.spec.hostNetwork == true
  msg := "Pods must not use hostNetwork"
}
`,
	}
}

func generateConftestResourcesPolicies(dir string) map[string]string {
	return map[string]string{
		dir + "/require-requests.rego": `package main

# Require resource requests
deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  not c.resources.requests
  msg := sprintf("Container '%s' must specify resource requests", [c.name])
}
`,
		dir + "/require-limits.rego": `package main

# Require resource limits
deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  not c.resources.limits
  msg := sprintf("Container '%s' must specify resource limits", [c.name])
}
`,
		dir + "/deny-unbounded.rego": `package main

# Deny unbounded CPU or memory
deny[msg] {
  input.kind == "Deployment"
  c := input.spec.template.spec.containers[_]
  not c.resources.limits.memory
  msg := sprintf("Container '%s' must have a memory limit", [c.name])
}
`,
	}
}

func generateConftestLabelsPolicies(dir string) map[string]string {
	return map[string]string{
		dir + "/require-standard-labels.rego": `package main

# Require standard Kubernetes recommended labels
required_labels := {"app.kubernetes.io/name", "app.kubernetes.io/version"}

deny[msg] {
  provided := {label | input.metadata.labels[label]}
  missing := required_labels - provided
  count(missing) > 0
  msg := sprintf("Resource is missing required labels: %v", [missing])
}
`,
	}
}

func generateNetworkingPolicies(dir string) map[string]string {
	return map[string]string{
		dir + "/deny-loadbalancer.rego": `package main

# Deny LoadBalancer service type unless explicitly needed
deny[msg] {
  input.kind == "Service"
  input.spec.type == "LoadBalancer"
  not input.metadata.annotations["networking.allow-loadbalancer"]
  msg := "Service type LoadBalancer is not allowed without annotation networking.allow-loadbalancer"
}
`,
		dir + "/require-networkpolicy.rego": `package main

# Warn if namespace has no NetworkPolicy
deny[msg] {
  input.kind == "Namespace"
  not input.metadata.annotations["networking.networkpolicy-enforced"]
  msg := sprintf("Namespace '%s' should have a NetworkPolicy", [input.metadata.name])
}
`,
	}
}

func generateStoragePolicies(dir string) map[string]string {
	return map[string]string{
		dir + "/deny-default-storageclass.rego": `package main

# Deny PVC without explicit storageClassName
deny[msg] {
  input.kind == "PersistentVolumeClaim"
  not input.spec.storageClassName
  msg := sprintf("PVC '%s' must specify an explicit storageClassName", [input.metadata.name])
}
`,
	}
}
