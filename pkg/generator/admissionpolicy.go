package generator

import (
	"fmt"
	"strings"
)

// GenerateAdmissionPolicies generates admission control policies for the specified engine.
// Supported engines: "kyverno", "opa".
// Returns a map of filename → YAML content.
func GenerateAdmissionPolicies(engine string) map[string]string {
	switch engine {
	case "kyverno":
		return generateKyvernoPolicies()
	case "opa":
		return generateOPAPolicies()
	default:
		return map[string]string{}
	}
}

func generateKyvernoPolicies() map[string]string {
	result := make(map[string]string, 4)

	result["require-labels.yaml"] = kyvernoPolicy("require-labels",
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

	result["deny-privileged.yaml"] = kyvernoPolicy("deny-privileged",
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

	result["require-resources.yaml"] = kyvernoPolicy("require-resources",
		"Require resource requests and limits on all containers.",
		`    rules:
      - name: check-resources
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "CPU and memory requests and limits are required."
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

	result["require-probes.yaml"] = kyvernoPolicy("require-probes",
		"Require readiness and liveness probes on all containers.",
		`    rules:
      - name: check-probes
        match:
          any:
            - resources:
                kinds:
                  - Pod
        validate:
          message: "Readiness and liveness probes are required."
          pattern:
            spec:
              containers:
                - readinessProbe: "?*"
                  livenessProbe: "?*"`)

	return result
}

func kyvernoPolicy(name, description, rulesBlock string) string {
	var b strings.Builder
	b.WriteString("apiVersion: kyverno.io/v1\n")
	b.WriteString("kind: ClusterPolicy\n")
	b.WriteString("metadata:\n")
	fmt.Fprintf(&b, "  name: %s\n", name)
	b.WriteString("  annotations:\n")
	b.WriteString("    policies.kyverno.io/title: " + name + "\n")
	fmt.Fprintf(&b, "    policies.kyverno.io/description: %s\n", description)
	b.WriteString("spec:\n")
	b.WriteString("  validationFailureAction: audit\n")
	b.WriteString(rulesBlock + "\n")
	return b.String()
}

func generateOPAPolicies() map[string]string {
	result := make(map[string]string, 4)

	result["require-labels.yaml"] = opaPolicy("require-labels",
		"Require app.kubernetes.io/name and app.kubernetes.io/version labels on all Pods.",
		"Pod",
		`      violation[{"msg": msg}] {
        provided := {label | input.review.object.metadata.labels[label]}
        required := {"app.kubernetes.io/name", "app.kubernetes.io/version"}
        missing := required - provided
        count(missing) > 0
        msg := sprintf("Missing required labels: %v", [missing])
      }`,
		"")

	result["deny-privileged.yaml"] = opaPolicy("deny-privileged",
		"Deny containers running in privileged mode.",
		"Pod",
		`      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        container.securityContext.privileged == true
        msg := sprintf("Privileged container not allowed: %v", [container.name])
      }`,
		"")

	result["require-resources.yaml"] = opaPolicy("require-resources",
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
      }`,
		"")

	result["require-probes.yaml"] = opaPolicy("require-probes",
		"Require readiness and liveness probes on all containers.",
		"Pod",
		`      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not container.readinessProbe
        msg := sprintf("Readiness probe required for container: %v", [container.name])
      }
      violation[{"msg": msg}] {
        container := input.review.object.spec.containers[_]
        not container.livenessProbe
        msg := sprintf("Liveness probe required for container: %v", [container.name])
      }`,
		"")

	return result
}

func opaPolicy(name, description, kind, regoBody, constraintParams string) string {
	var b strings.Builder

	// ConstraintTemplate
	b.WriteString("apiVersion: templates.gatekeeper.sh/v1\n")
	b.WriteString("kind: ConstraintTemplate\n")
	b.WriteString("metadata:\n")
	fmt.Fprintf(&b, "  name: %s\n", name)
	b.WriteString("spec:\n")
	b.WriteString("  crd:\n")
	b.WriteString("    spec:\n")
	b.WriteString("      names:\n")
	fmt.Fprintf(&b, "        kind: %s\n", opaKindName(name))
	b.WriteString("  targets:\n")
	b.WriteString("    - target: admission.k8s.gatekeeper.sh\n")
	b.WriteString("      rego: |\n")
	fmt.Fprintf(&b, "        package %s\n", strings.ReplaceAll(name, "-", ""))
	b.WriteString(regoBody + "\n")

	// Separator
	b.WriteString("---\n")

	// Constraint
	fmt.Fprintf(&b, "apiVersion: constraints.gatekeeper.sh/%s\n", opaKindName(name))
	b.WriteString("kind: " + opaKindName(name) + "\n")
	b.WriteString("metadata:\n")
	fmt.Fprintf(&b, "  name: %s\n", name)
	b.WriteString("spec:\n")
	b.WriteString("  enforcementAction: dryrun\n")
	b.WriteString("  match:\n")
	b.WriteString("    kinds:\n")
	b.WriteString("      - apiGroups: [\"\"]\n")
	fmt.Fprintf(&b, "        kinds: [\"%s\"]\n", kind)

	return b.String()
}

// opaKindName converts a kebab-case name to a CamelCase OPA kind name.
func opaKindName(name string) string {
	parts := strings.Split(name, "-")
	var result strings.Builder
	for _, p := range parts {
		if len(p) > 0 {
			result.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
	}
	return result.String()
}
