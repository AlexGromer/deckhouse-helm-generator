package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// PolicyAsCodeOptions configures policy-as-code generation.
type PolicyAsCodeOptions struct {
	// OutputFormats controls which engines to generate policies for: "kyverno", "opa".
	OutputFormats []string
	// PolicyTypes lists policy types to generate.
	// Supported: "require-labels", "require-resources", "disallow-privileged",
	//            "restrict-registries", "require-probes".
	PolicyTypes []string
}

// PolicyAsCodeResult holds generated policies for each engine.
type PolicyAsCodeResult struct {
	// KyvernoPolicies maps filename → YAML content for Kyverno.
	KyvernoPolicies map[string]string
	// OPAPolicies maps filename → YAML content for OPA/Gatekeeper.
	OPAPolicies map[string]string
	// Summary is a human-readable summary of generated policies.
	Summary string
}

// GeneratePolicyAsCode generates policy-as-code artifacts for the given graph and options.
// Returns a non-nil PolicyAsCodeResult even for empty graphs.
func GeneratePolicyAsCode(graph *types.ResourceGraph, opts PolicyAsCodeOptions) *PolicyAsCodeResult {
	result := &PolicyAsCodeResult{
		KyvernoPolicies: make(map[string]string),
		OPAPolicies:     make(map[string]string),
	}

	wantKyverno := false
	wantOPA := false
	for _, f := range opts.OutputFormats {
		switch f {
		case "kyverno":
			wantKyverno = true
		case "opa":
			wantOPA = true
		}
	}

	for _, pt := range opts.PolicyTypes {
		if wantKyverno {
			if content, name := kyvernoPolicyForType(pt); content != "" {
				result.KyvernoPolicies[name] = content
			}
		}
		if wantOPA {
			if content, name := opaPolicyForType(pt); content != "" {
				result.OPAPolicies[name] = content
			}
		}
	}

	// _ suppress unused graph warning — future: graph-aware policies
	_ = graph

	result.Summary = buildPolicyAsCodeSummary(result)
	return result
}

func kyvernoPolicyForType(policyType string) (content, name string) {
	switch policyType {
	case "require-labels":
		n := "require-labels"
		c := kyvernoPolicy(n,
			"Require app.kubernetes.io/name and app.kubernetes.io/version labels on all Pods.",
			`    rules:
      - name: check-labels
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "Labels 'app.kubernetes.io/name' and 'app.kubernetes.io/version' are required."
          pattern:
            metadata:
              labels:
                app.kubernetes.io/name: "?*"
                app.kubernetes.io/version: "?*"`)
		return c, n + ".yaml"

	case "require-resources":
		n := "require-resources"
		c := kyvernoPolicy(n,
			"Require CPU and memory resource requests and limits on all containers.",
			`    rules:
      - name: check-resources
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "CPU and memory resource requests and limits are required."
          pattern:
            spec:
              containers:
                - resources:
                    requests:
                      memory: "?*"
                      cpu: "?*"
                    limits:
                      memory: "?*"
                      cpu: "?*"`)
		return c, n + ".yaml"

	case "disallow-privileged":
		n := "disallow-privileged"
		c := kyvernoPolicy(n,
			"Deny containers running in privileged mode.",
			`    rules:
      - name: deny-privileged-containers
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "Privileged containers are not allowed."
          pattern:
            spec:
              containers:
                - securityContext:
                    privileged: "false"`)
		return c, n + ".yaml"

	case "restrict-registries":
		n := "restrict-registries"
		c := kyvernoPolicy(n,
			"Restrict container image registries to approved registries.",
			`    rules:
      - name: restrict-image-registries
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "Only approved image registries are allowed."
          pattern:
            spec:
              containers:
                - image: "registry.example.com/*"`)
		return c, n + ".yaml"

	case "require-probes":
		n := "require-probes"
		c := kyvernoPolicy(n,
			"Require readiness and liveness probes on all containers.",
			`    rules:
      - name: check-probes
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "Readiness and liveness Probes are required for containers."
          pattern:
            spec:
              containers:
                - readinessProbe: "?*"
                  livenessProbe: "?*"`)
		return c, n + ".yaml"
	}
	return "", ""
}

func opaPolicyForType(policyType string) (content, name string) {
	switch policyType {
	case "require-labels":
		n := "require-labels"
		c := opaPolicy(n,
			"Require app.kubernetes.io/name and app.kubernetes.io/version labels on all Pods.",
			"Pod",
			`      violation[{"msg": msg}] {
        provided := {label | input.review.object.metadata.labels[label]}
        required := {"app.kubernetes.io/name", "app.kubernetes.io/version"}
        missing := required - provided
        count(missing) > 0
        msg := sprintf("Missing required labels: %v", [missing])
      }`, "")
		return c, n + ".yaml"

	case "require-resources":
		n := "require-resources"
		c := opaPolicy(n,
			"Require resource requests and limits on all containers.",
			"Pod",
			`      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not container.resources.requests
        msg := sprintf("Resource requests required for container: %v", [container.name])
      }
      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not container.resources.limits
        msg := sprintf("Resource limits required for container: %v", [container.name])
      }`, "")
		return c, n + ".yaml"

	case "disallow-privileged":
		n := "disallow-privileged"
		c := opaPolicy(n,
			"Deny containers running in privileged mode.",
			"Pod",
			`      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        container.securityContext.privileged == true
        msg := sprintf("Privileged container not allowed: %v", [container.name])
      }`, "")
		return c, n + ".yaml"

	case "restrict-registries":
		n := "restrict-registries"
		c := opaPolicy(n,
			"Restrict container image registries to approved registries.",
			"Pod",
			`      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not startswith(container.image, "registry.example.com/")
        msg := sprintf("Unapproved image registry for container: %v, image: %v", [container.name, container.image])
      }`, "")
		return c, n + ".yaml"

	case "require-probes":
		n := "require-probes"
		c := opaPolicy(n,
			"Require readiness and liveness probes on all containers.",
			"Pod",
			`      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not container.readinessProbe
        msg := sprintf("readinessProbe required for container: %v", [container.name])
      }
      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not container.livenessProbe
        msg := sprintf("livenessProbe required for container: %v", [container.name])
      }`, "")
		return c, n + ".yaml"
	}
	return "", ""
}

func buildPolicyAsCodeSummary(result *PolicyAsCodeResult) string {
	var sb strings.Builder
	sb.WriteString("Policy-as-Code Summary\n")
	sb.WriteString("======================\n")
	kyvernoCount := len(result.KyvernoPolicies)
	opaCount := len(result.OPAPolicies)
	if kyvernoCount > 0 {
		sb.WriteString(fmt.Sprintf("Kyverno policies: %d\n", kyvernoCount))
	}
	if opaCount > 0 {
		sb.WriteString(fmt.Sprintf("OPA/Gatekeeper policies: %d\n", opaCount))
	}
	return sb.String()
}
