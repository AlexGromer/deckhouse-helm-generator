package pss

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Profile represents a Pod Security Standard profile.
type Profile string

const (
	ProfilePrivileged Profile = "privileged"
	ProfileBaseline   Profile = "baseline"
	ProfileRestricted Profile = "restricted"
)

// ParseProfile parses a string into a Profile, ignoring case.
func ParseProfile(s string) (Profile, error) {
	switch strings.ToLower(s) {
	case "privileged":
		return ProfilePrivileged, nil
	case "baseline":
		return ProfileBaseline, nil
	case "restricted":
		return ProfileRestricted, nil
	default:
		return "", fmt.Errorf("unknown PSS profile %q", s)
	}
}

// Finding represents a violation of a PSS control.
type Finding struct {
	Profile Profile
	Control string
	Field   string
	Value   string
}

// WorkloadReport represents the evaluation result for a single workload.
type WorkloadReport struct {
	Profile  Profile
	Findings []Finding
}

// Report represents the evaluation result for multiple resources/chart.
type Report struct {
	Workloads map[types.ResourceKey]WorkloadReport
}

func getNestedSlice(obj map[string]interface{}, fields ...string) []interface{} {
	val, found, _ := unstructured.NestedSlice(obj, fields...)
	if !found {
		return nil
	}
	return val
}

func getNestedMap(obj map[string]interface{}, fields ...string) map[string]interface{} {
	val, found, _ := unstructured.NestedMap(obj, fields...)
	if !found {
		return nil
	}
	return val
}

func getNestedString(obj map[string]interface{}, fields ...string) string {
	val, found, _ := unstructured.NestedString(obj, fields...)
	if !found {
		return ""
	}
	return val
}

func getNestedBool(obj map[string]interface{}, fields ...string) (bool, bool) {
	val, found, _ := unstructured.NestedBool(obj, fields...)
	return val, found
}

// EvaluateWorkload evaluates a workload against the target profile.
func EvaluateWorkload(obj *unstructured.Unstructured, target Profile) WorkloadReport {
	spec, annotations, _, ok := PodSpec(obj)
	if !ok {
		return WorkloadReport{Profile: ProfilePrivileged} // Return empty report for non-workloads
	}

	findings := evaluateSpec(spec, annotations)

	baselineFindings := 0
	restrictedFindings := 0

	for _, f := range findings {
		switch f.Profile {
		case ProfileBaseline:
			baselineFindings++
		case ProfileRestricted:
			restrictedFindings++
		}
	}

	profile := ProfileRestricted
	if baselineFindings > 0 {
		profile = ProfilePrivileged
	} else if restrictedFindings > 0 {
		profile = ProfileBaseline
	}

	filteredFindings := []Finding{}
	for _, f := range findings {
		if f.Profile == ProfileBaseline || (target == ProfileRestricted && f.Profile == ProfileRestricted) {
			filteredFindings = append(filteredFindings, f)
		}
	}

	return WorkloadReport{
		Profile:  profile,
		Findings: filteredFindings,
	}
}

// EvaluateResources evaluates a list of processed resources.
func EvaluateResources(resources []*types.ProcessedResource, target Profile) *Report {
	report := &Report{
		Workloads: make(map[types.ResourceKey]WorkloadReport),
	}

	for _, res := range resources {
		if res.Original == nil || res.Original.Object == nil {
			continue
		}

		_, _, _, isWorkload := PodSpec(res.Original.Object)
		if !isWorkload {
			continue
		}

		report.Workloads[res.Original.ResourceKey()] = EvaluateWorkload(res.Original.Object, target)
	}

	return report
}

// EvaluateChart evaluates an entire generated chart.
func EvaluateChart(chart *types.GeneratedChart, graph *types.ResourceGraph, target Profile) *Report {
	// Not needed to implement right now, features pass resources directly
	return &Report{Workloads: make(map[types.ResourceKey]WorkloadReport)}
}

func evaluateSpec(spec map[string]interface{}, annotations map[string]string) []Finding {
	// We will implement all 11 checks here as requested.
	var findings []Finding

	// Check OS
	isWindows := getNestedString(spec, "os", "name") == "windows"

	// Baseline controls

	// 1. Host Namespaces
	for _, field := range []string{"hostNetwork", "hostPID", "hostIPC"} {
		if val, found := getNestedBool(spec, field); found && val {
			findings = append(findings, Finding{Profile: ProfileBaseline, Control: "Host Namespaces", Field: "spec." + field, Value: "true"})
		}
	}

	// 2. Privileged Containers
	evaluateContainers(spec, "privileged", func(c map[string]interface{}, pathPrefix string) {
		secCtx := getNestedMap(c, "securityContext")
		if secCtx == nil {
			return
		}
		// Also handle invalid types, if not boolean it's a violation. We use raw get
		val, ok := secCtx["privileged"]
		if ok {
			if b, isBool := val.(bool); isBool {
				if b {
					findings = append(findings, Finding{Profile: ProfileBaseline, Control: "Privileged Containers", Field: pathPrefix + ".securityContext.privileged", Value: "true"})
				}
			} else {
				findings = append(findings, Finding{Profile: ProfileBaseline, Control: "Privileged Containers", Field: pathPrefix + ".securityContext.privileged", Value: fmt.Sprintf("%v", val)})
			}
		}
	})

	// 3. Capabilities Baseline
	baselineAllowedCaps := map[string]bool{
		"AUDIT_WRITE": true, "CHOWN": true, "DAC_OVERRIDE": true, "FOWNER": true,
		"FSETID": true, "KILL": true, "MKNOD": true, "NET_BIND_SERVICE": true,
		"SETFCAP": true, "SETGID": true, "SETPCAP": true, "SETUID": true, "SYS_CHROOT": true,
	}

	evaluateContainers(spec, "capabilities", func(c map[string]interface{}, pathPrefix string) {
		adds := getNestedSlice(c, "securityContext", "capabilities", "add")
		for i, add := range adds {
			addStr, _ := add.(string)
			if !baselineAllowedCaps[addStr] {
				findings = append(findings, Finding{Profile: ProfileBaseline, Control: "Capabilities", Field: fmt.Sprintf("%s.securityContext.capabilities.add[%d]", pathPrefix, i), Value: fmt.Sprintf("%v", add)})
			}
		}
	})

	// 4. HostPath Volumes
	volumes := getNestedSlice(spec, "volumes")
	for i, v := range volumes {
		vol, _ := v.(map[string]interface{})
		for k := range vol {
			if k == "hostPath" {
				findings = append(findings, Finding{Profile: ProfileBaseline, Control: "HostPath Volumes", Field: fmt.Sprintf("spec.volumes[%d].hostPath", i), Value: fmt.Sprintf("%v", vol["hostPath"])})
			}
		}
	}

	// 5. Host Ports
	evaluateContainers(spec, "hostPort", func(c map[string]interface{}, pathPrefix string) {
		ports := getNestedSlice(c, "ports")
		for i, p := range ports {
			port, _ := p.(map[string]interface{})
			if port != nil {
				if hostPort, ok := port["hostPort"]; ok {
					// hostPort 0 is allowed. Anything else, or invalid type is forbidden
					valStr := fmt.Sprintf("%v", hostPort)
					if valStr != "0" {
						findings = append(findings, Finding{Profile: ProfileBaseline, Control: "Host Ports", Field: fmt.Sprintf("%s.ports[%d].hostPort", pathPrefix, i), Value: valStr})
					}
				}
			}
		}
	})

	// 6. AppArmor
	evaluateContainers(spec, "appArmor", func(c map[string]interface{}, pathPrefix string) {
		// AppArmor in securityContext (v1.30+)
		if aa := getNestedMap(c, "securityContext", "appArmorProfile"); aa != nil {
			if typ, _ := aa["type"].(string); typ == "Unconfined" {
				findings = append(findings, Finding{Profile: ProfileBaseline, Control: "AppArmor", Field: pathPrefix + ".securityContext.appArmorProfile.type", Value: "Unconfined"})
			}
		}

		// Annotations
		name, _ := c["name"].(string)
		if name != "" {
			annKey := "container.apparmor.security.beta.kubernetes.io/" + name
			if val, ok := annotations[annKey]; ok && val == "unconfined" {
				findings = append(findings, Finding{Profile: ProfileBaseline, Control: "AppArmor", Field: "metadata.annotations[" + annKey + "]", Value: "unconfined"})
			}
		}
	})

	// 7. SELinux
	allowedSELinuxTypes := map[string]bool{
		"": true, "container_t": true, "container_init_t": true, "container_kvm_t": true, "container_engine_t": true,
	}

	checkSELinux := func(secCtx map[string]interface{}, path string) {
		if se := getNestedMap(secCtx, "seLinuxOptions"); se != nil {
			if typ, ok := se["type"]; ok {
				typStr := fmt.Sprintf("%v", typ)
				if !allowedSELinuxTypes[typStr] {
					findings = append(findings, Finding{Profile: ProfileBaseline, Control: "SELinux", Field: path + ".seLinuxOptions.type", Value: typStr})
				}
			}
			if user, ok := se["user"]; ok && fmt.Sprintf("%v", user) != "" {
				findings = append(findings, Finding{Profile: ProfileBaseline, Control: "SELinux", Field: path + ".seLinuxOptions.user", Value: fmt.Sprintf("%v", user)})
			}
			if role, ok := se["role"]; ok && fmt.Sprintf("%v", role) != "" {
				findings = append(findings, Finding{Profile: ProfileBaseline, Control: "SELinux", Field: path + ".seLinuxOptions.role", Value: fmt.Sprintf("%v", role)})
			}
		}
	}

	if secCtx := getNestedMap(spec, "securityContext"); secCtx != nil {
		checkSELinux(secCtx, "spec.securityContext")
	}
	evaluateContainers(spec, "seLinux", func(c map[string]interface{}, pathPrefix string) {
		if secCtx := getNestedMap(c, "securityContext"); secCtx != nil {
			checkSELinux(secCtx, pathPrefix+".securityContext")
		}
	})

	// 8. /proc Mount Type
	evaluateContainers(spec, "procMount", func(c map[string]interface{}, pathPrefix string) {
		if secCtx := getNestedMap(c, "securityContext"); secCtx != nil {
			if pm, ok := secCtx["procMount"]; ok && fmt.Sprintf("%v", pm) == "Unmasked" {
				findings = append(findings, Finding{Profile: ProfileBaseline, Control: "/proc Mount Type", Field: pathPrefix + ".securityContext.procMount", Value: "Unmasked"})
			}
		}
	})

	// 9. Seccomp
	checkSeccompBaseline := func(secCtx map[string]interface{}, path string) {
		if sc := getNestedMap(secCtx, "seccompProfile"); sc != nil {
			if typ, ok := sc["type"]; ok && fmt.Sprintf("%v", typ) == "Unconfined" {
				findings = append(findings, Finding{Profile: ProfileBaseline, Control: "Seccomp", Field: path + ".seccompProfile.type", Value: "Unconfined"})
			}
		}
	}

	if secCtx := getNestedMap(spec, "securityContext"); secCtx != nil {
		checkSeccompBaseline(secCtx, "spec.securityContext")
	}
	evaluateContainers(spec, "seccomp", func(c map[string]interface{}, pathPrefix string) {
		if secCtx := getNestedMap(c, "securityContext"); secCtx != nil {
			checkSeccompBaseline(secCtx, pathPrefix+".securityContext")
		}
	})

	// 10. Sysctls
	allowedSysctls := map[string]bool{
		"kernel.shm_rmid_forced": true, "net.ipv4.ip_local_port_range": true,
		"net.ipv4.ip_unprivileged_port_start": true, "net.ipv4.tcp_syncookies": true,
		"net.ipv4.ping_group_range": true, "net.ipv4.ip_local_reserved_ports": true,
		"net.ipv4.tcp_keepalive_time": true, "net.ipv4.tcp_fin_timeout": true,
		"net.ipv4.tcp_keepalive_intvl": true, "net.ipv4.tcp_keepalive_probes": true,
	}

	if secCtx := getNestedMap(spec, "securityContext"); secCtx != nil {
		sysctls := getNestedSlice(secCtx, "sysctls")
		for i, sysctlIntf := range sysctls {
			if sysctl, ok := sysctlIntf.(map[string]interface{}); ok {
				name := fmt.Sprintf("%v", sysctl["name"])
				if !allowedSysctls[name] {
					findings = append(findings, Finding{Profile: ProfileBaseline, Control: "Sysctls", Field: fmt.Sprintf("spec.securityContext.sysctls[%d].name", i), Value: name})
				}
			}
		}
	}

	// 11. Host Probes / Lifecycle Hooks
	checkProbeHost := func(probe map[string]interface{}, probeName, path string) {
		if probe == nil {
			return
		}

		if hg := getNestedMap(probe, "httpGet"); hg != nil {
			if host, ok := hg["host"]; ok && fmt.Sprintf("%v", host) != "" {
				findings = append(findings, Finding{Profile: ProfileBaseline, Control: "Host Probes / Lifecycle Hooks", Field: path + "." + probeName + ".httpGet.host", Value: fmt.Sprintf("%v", host)})
			}
		}
		if ts := getNestedMap(probe, "tcpSocket"); ts != nil {
			if host, ok := ts["host"]; ok && fmt.Sprintf("%v", host) != "" {
				findings = append(findings, Finding{Profile: ProfileBaseline, Control: "Host Probes / Lifecycle Hooks", Field: path + "." + probeName + ".tcpSocket.host", Value: fmt.Sprintf("%v", host)})
			}
		}
	}

	evaluateContainers(spec, "probes", func(c map[string]interface{}, pathPrefix string) {
		checkProbeHost(getNestedMap(c, "livenessProbe"), "livenessProbe", pathPrefix)
		checkProbeHost(getNestedMap(c, "readinessProbe"), "readinessProbe", pathPrefix)
		checkProbeHost(getNestedMap(c, "startupProbe"), "startupProbe", pathPrefix)

		if lc := getNestedMap(c, "lifecycle"); lc != nil {
			checkProbeHost(getNestedMap(lc, "postStart"), "lifecycle.postStart", pathPrefix)
			checkProbeHost(getNestedMap(lc, "preStop"), "lifecycle.preStop", pathPrefix)
		}
	})

	// Windows options
	if secCtx := getNestedMap(spec, "securityContext"); secCtx != nil {
		if wo := getNestedMap(secCtx, "windowsOptions"); wo != nil {
			if val, ok := wo["hostProcess"]; ok {
				if b, isBool := val.(bool); (isBool && b) || (!isBool && fmt.Sprintf("%v", val) == "true") {
					findings = append(findings, Finding{Profile: ProfileBaseline, Control: "HostProcess", Field: "spec.securityContext.windowsOptions.hostProcess", Value: "true"})
				}
			}
		}
	}
	evaluateContainers(spec, "windowsOptions", func(c map[string]interface{}, pathPrefix string) {
		if secCtx := getNestedMap(c, "securityContext"); secCtx != nil {
			if wo := getNestedMap(secCtx, "windowsOptions"); wo != nil {
				if val, ok := wo["hostProcess"]; ok {
					if b, isBool := val.(bool); (isBool && b) || (!isBool && fmt.Sprintf("%v", val) == "true") {
						findings = append(findings, Finding{Profile: ProfileBaseline, Control: "HostProcess", Field: pathPrefix + ".securityContext.windowsOptions.hostProcess", Value: "true"})
					}
				}
			}
		}
	})

	// Restricted controls

	// 1. Volume Types
	allowedVolTypes := map[string]bool{
		"configMap": true, "csi": true, "downwardAPI": true, "emptyDir": true,
		"ephemeral": true, "persistentVolumeClaim": true, "projected": true, "secret": true,
	}
	for i, v := range volumes {
		vol, _ := v.(map[string]interface{})
		for k := range vol {
			if k != "name" && !allowedVolTypes[k] {
				findings = append(findings, Finding{Profile: ProfileRestricted, Control: "Volume Types", Field: fmt.Sprintf("spec.volumes[%d].%s", i, k), Value: k})
			}
		}
	}

	if !isWindows {
		// 2. Privilege Escalation
		evaluateContainers(spec, "allowPrivilegeEscalation", func(c map[string]interface{}, pathPrefix string) {
			secCtx := getNestedMap(c, "securityContext")
			val, ok := secCtx["allowPrivilegeEscalation"]
			if !ok {
				findings = append(findings, Finding{Profile: ProfileRestricted, Control: "Privilege Escalation", Field: pathPrefix + ".securityContext.allowPrivilegeEscalation", Value: "<unset>"})
			} else {
				b, isBool := val.(bool)
				if !isBool || b {
					findings = append(findings, Finding{Profile: ProfileRestricted, Control: "Privilege Escalation", Field: pathPrefix + ".securityContext.allowPrivilegeEscalation", Value: fmt.Sprintf("%v", val)})
				}
			}
		})
	}

	// 3. Running as Non-root
	podRunAsNonRoot := false
	podRunAsNonRootSet := false
	if secCtx := getNestedMap(spec, "securityContext"); secCtx != nil {
		if val, ok := secCtx["runAsNonRoot"]; ok {
			if b, isBool := val.(bool); isBool {
				podRunAsNonRoot = b
				podRunAsNonRootSet = true
			}
		}
	}

	evaluateContainers(spec, "runAsNonRoot", func(c map[string]interface{}, pathPrefix string) {
		secCtx := getNestedMap(c, "securityContext")
		val, ok := secCtx["runAsNonRoot"]
		if !ok {
			if !podRunAsNonRootSet || !podRunAsNonRoot {
				findings = append(findings, Finding{Profile: ProfileRestricted, Control: "Running as Non-root", Field: pathPrefix + ".securityContext.runAsNonRoot", Value: "<unset>"})
			}
		} else {
			b, isBool := val.(bool)
			if !isBool || !b {
				findings = append(findings, Finding{Profile: ProfileRestricted, Control: "Running as Non-root", Field: pathPrefix + ".securityContext.runAsNonRoot", Value: fmt.Sprintf("%v", val)})
			}
		}
	})

	// Running as Non-root user (runAsUser != 0)
	checkRunAsUser := func(secCtx map[string]interface{}, path string) {
		if secCtx != nil {
			if val, ok := secCtx["runAsUser"]; ok {
				if fmt.Sprintf("%v", val) == "0" {
					findings = append(findings, Finding{Profile: ProfileRestricted, Control: "Running as Non-root user", Field: path + ".runAsUser", Value: "0"})
				}
			}
		}
	}

	checkRunAsUser(getNestedMap(spec, "securityContext"), "spec.securityContext")
	evaluateContainers(spec, "runAsUser", func(c map[string]interface{}, pathPrefix string) {
		checkRunAsUser(getNestedMap(c, "securityContext"), pathPrefix+".securityContext")
	})

	if !isWindows {
		// 4. Seccomp Restricted
		podSeccompDefault := false
		if secCtx := getNestedMap(spec, "securityContext"); secCtx != nil {
			if sc := getNestedMap(secCtx, "seccompProfile"); sc != nil {
				if typ, ok := sc["type"]; ok {
					typStr := fmt.Sprintf("%v", typ)
					if typStr == "RuntimeDefault" || typStr == "Localhost" {
						podSeccompDefault = true
					}
				}
			}
		}

		evaluateContainers(spec, "seccompRestricted", func(c map[string]interface{}, pathPrefix string) {
			secCtx := getNestedMap(c, "securityContext")
			hasDefaultSeccomp := false
			val := "<unset>"

			if sc := getNestedMap(secCtx, "seccompProfile"); sc != nil {
				if typ, ok := sc["type"]; ok {
					typStr := fmt.Sprintf("%v", typ)
					val = typStr
					if typStr == "RuntimeDefault" || typStr == "Localhost" {
						hasDefaultSeccomp = true
					}
				}
			}

			if !hasDefaultSeccomp && !podSeccompDefault {
				findings = append(findings, Finding{Profile: ProfileRestricted, Control: "Seccomp", Field: pathPrefix + ".securityContext.seccompProfile.type", Value: val})
			}
		})

		// 5. Capabilities Restricted
		evaluateContainers(spec, "capabilitiesRestricted", func(c map[string]interface{}, pathPrefix string) {
			secCtx := getNestedMap(c, "securityContext")

			// Check drops ALL
			drops := getNestedSlice(secCtx, "capabilities", "drop")
			hasAll := false
			for _, drop := range drops {
				if fmt.Sprintf("%v", drop) == "ALL" {
					hasAll = true
					break
				}
			}
			if !hasAll {
				findings = append(findings, Finding{Profile: ProfileRestricted, Control: "Capabilities", Field: pathPrefix + ".securityContext.capabilities.drop", Value: "missing ALL"})
			}

			// Check adds
			adds := getNestedSlice(secCtx, "capabilities", "add")
			for i, add := range adds {
				addStr := fmt.Sprintf("%v", add)
				if addStr != "NET_BIND_SERVICE" {
					findings = append(findings, Finding{Profile: ProfileRestricted, Control: "Capabilities", Field: fmt.Sprintf("%s.securityContext.capabilities.add[%d]", pathPrefix, i), Value: addStr})
				}
			}
		})
	}

	// Sort findings by Field, then Control
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Field != findings[j].Field {
			return findings[i].Field < findings[j].Field
		}
		return findings[i].Control < findings[j].Control
	})

	return findings
}

func evaluateContainers(spec map[string]interface{}, id string, fn func(c map[string]interface{}, pathPrefix string)) {
	for _, listName := range []string{"initContainers", "containers", "ephemeralContainers"} {
		containers := getNestedSlice(spec, listName)
		for i, cIntf := range containers {
			if c, ok := cIntf.(map[string]interface{}); ok {
				fn(c, fmt.Sprintf("spec.%s[%d]", listName, i))
			}
		}
	}
}

// PodSpec extracts the pod spec map from a workload object.
func PodSpec(obj *unstructured.Unstructured) (spec map[string]interface{}, annotations map[string]string, path string, ok bool) {
	kind := obj.GetKind()
	switch kind {
	case "Pod":
		spec = getNestedMap(obj.Object, "spec")
		annotations = obj.GetAnnotations()
		path = "spec"
		ok = spec != nil
	case "CronJob":
		spec = getNestedMap(obj.Object, "spec", "jobTemplate", "spec", "template", "spec")
		annotations = getNestedMapString(obj.Object, "spec", "jobTemplate", "spec", "template", "metadata", "annotations")
		path = "spec.jobTemplate.spec.template.spec"
		ok = spec != nil
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		spec = getNestedMap(obj.Object, "spec", "template", "spec")
		annotations = getNestedMapString(obj.Object, "spec", "template", "metadata", "annotations")
		path = "spec.template.spec"
		ok = spec != nil
	case "Rollout":
		// argoproj.io/v1alpha1
		spec = getNestedMap(obj.Object, "spec", "template", "spec")
		annotations = getNestedMapString(obj.Object, "spec", "template", "metadata", "annotations")
		path = "spec.template.spec"
		ok = spec != nil
	default:
		ok = false
	}

	if annotations == nil {
		annotations = make(map[string]string)
	}

	return spec, annotations, path, ok
}

func getNestedMapString(obj map[string]interface{}, fields ...string) map[string]string {
	val, found, _ := unstructured.NestedStringMap(obj, fields...)
	if !found {
		return nil
	}
	return val
}
