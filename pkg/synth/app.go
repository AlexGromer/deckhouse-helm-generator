// Package synth describes an application from facts found outside Kubernetes
// (an image's configuration, a docker-compose file, a project's sources) and
// builds the Kubernetes manifests that run it. The manifests then go through
// the regular dhg pipeline like any input read from files.
//
// Only facts present in the input end up in a manifest. What cannot be
// derived (resource requests, secrets, Ingress hosts, volume sizes) is left
// out and reported through Notes. See docs/SPEC_SYNTHESIS.md.
package synth

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// App is one service to run.
type App struct {
	// Name is the DNS-1123 name of the Deployment, Service and label
	// app.kubernetes.io/name.
	Name string
	// Image is the container image reference.
	Image string
	// Command and Args override the image's entrypoint and command; set only
	// when the input overrides them.
	Command []string
	Args    []string
	// WorkingDir overrides the image's working directory.
	WorkingDir string
	// Ports are the container ports; the Service exposes the same numbers.
	Ports []Port
	// Env holds plain variables (ConfigMap <name>-env), SecretEnv sensitive
	// ones (Secret <name>-env).
	Env       []EnvVar
	SecretEnv []EnvVar
	// RunAsUser and RunAsGroup come from the image, Dockerfile or compose user.
	RunAsUser  *int64
	RunAsGroup *int64
	// Volumes are mounted into the container.
	Volumes []Volume
	// Liveness and Readiness are the container probes.
	Liveness  *Probe
	Readiness *Probe
	// Replicas is set only when the input gives it.
	Replicas *int64
	// Resources is set only when the input gives it.
	Resources *Resources
	// Labels are extra pod labels.
	Labels map[string]string
}

// Port is a container port.
type Port struct {
	Name     string
	Port     int64
	Protocol string // TCP or UDP
}

// EnvVar is an environment variable.
type EnvVar struct {
	Name  string
	Value string
}

// VolumeKind says how a volume is backed.
type VolumeKind string

const (
	// VolumeEmptyDir is scratch space that lives as long as the pod.
	VolumeEmptyDir VolumeKind = "emptyDir"
	// VolumePVC is a PersistentVolumeClaim named <app>-<volume>.
	VolumePVC VolumeKind = "pvc"
	// VolumeConfigFile is a file from ConfigMap <app>-files, mounted with subPath.
	VolumeConfigFile VolumeKind = "configFile"
)

// Volume is a volume mounted into the container.
type Volume struct {
	Name      string
	MountPath string
	Kind      VolumeKind
	// Size is the PVC request (default 1Gi).
	Size string
	// FileName and Content describe a VolumeConfigFile.
	FileName string
	Content  string
	ReadOnly bool
}

// Probe is an HTTP GET or exec probe.
type Probe struct {
	HTTPPath string
	Port     int64
	Exec     []string

	PeriodSeconds       int64
	TimeoutSeconds      int64
	FailureThreshold    int64
	InitialDelaySeconds int64
}

// Resources are container requests and limits ("cpu", "memory").
type Resources struct {
	Requests map[string]string
	Limits   map[string]string
}

// Notes collects what the synthesis decided or could not decide.
type Notes struct {
	items []string
}

// Addf records a note.
func (n *Notes) Addf(format string, args ...interface{}) {
	n.items = append(n.items, fmt.Sprintf(format, args...))
}

// Items returns the notes in the order they were recorded.
func (n *Notes) Items() []string {
	if n == nil {
		return nil
	}
	return append([]string(nil), n.items...)
}

var (
	nonDNS        = regexp.MustCompile(`[^a-z0-9-]+`)
	repeatedDash  = regexp.MustCompile(`-+`)
	sensitiveName = regexp.MustCompile(`(?i)(PASSWORD|PASSWD|SECRET|TOKEN|PRIVATE_?KEY|API_?KEY|ACCESS_?KEY|CREDENTIAL)`)
)

// DNSName turns s into a DNS-1123 label: lower case, [a-z0-9-], at most 63
// characters, starting and ending with an alphanumeric character.
func DNSName(s string) string {
	s = strings.ToLower(s)
	s = nonDNS.ReplaceAllString(s, "-")
	s = repeatedDash.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = strings.TrimRight(s[:63], "-")
	}
	if s == "" {
		return "app"
	}
	return s
}

// IsSensitive reports whether an environment variable name suggests a
// secret value (password, token, key, credential).
func IsSensitive(name string) bool {
	return sensitiveName.MatchString(name)
}

// AddPort adds a port unless the same number and protocol is already
// there, named "<protocol>-<port>": the application protocol is unknown, and
// names like "http" drive protocol detection in meshes.
func (a *App) AddPort(port int64, protocol string) {
	protocol = strings.ToUpper(protocol)
	if protocol == "" {
		protocol = "TCP"
	}
	a.AddNamedPort(fmt.Sprintf("%s-%d", strings.ToLower(protocol), port), port, protocol)
}

// AddNamedPort adds a port with a known name (e.g. "http" for a web
// framework's port). An existing port with the same number and protocol is
// renamed instead.
func (a *App) AddNamedPort(name string, port int64, protocol string) {
	for i, p := range a.Ports {
		if p.Port == port && p.Protocol == protocol {
			if !strings.HasPrefix(name, strings.ToLower(protocol)+"-") && !a.hasPortName(name) {
				a.Ports[i].Name = name
			}
			return
		}
	}
	if a.hasPortName(name) {
		name = fmt.Sprintf("%s-%d", strings.ToLower(protocol), port)
	}
	a.Ports = append(a.Ports, Port{Name: name, Port: port, Protocol: protocol})
}

func (a *App) hasPortName(name string) bool {
	for _, p := range a.Ports {
		if p.Name == name {
			return true
		}
	}
	return false
}

// SetEnv sets a variable, routing names that look sensitive to SecretEnv.
// A later value for the same name replaces the earlier one.
func (a *App) SetEnv(name, value string) {
	target := &a.Env
	other := &a.SecretEnv
	if IsSensitive(name) {
		target, other = other, target
	}
	*other = removeEnv(*other, name)
	for i := range *target {
		if (*target)[i].Name == name {
			(*target)[i].Value = value
			return
		}
	}
	*target = append(*target, EnvVar{Name: name, Value: value})
}

func removeEnv(env []EnvVar, name string) []EnvVar {
	out := env[:0]
	for _, e := range env {
		if e.Name != name {
			out = append(out, e)
		}
	}
	return out
}

// SetUser parses "uid[:gid]" and sets RunAsUser/RunAsGroup. It returns false
// when the user is a name rather than a number: kubelet cannot verify
// runAsNonRoot for a named user, so such users are only reported.
func (a *App) SetUser(user string) bool {
	user = strings.TrimSpace(user)
	if user == "" {
		return true
	}
	uidPart, gidPart, _ := strings.Cut(user, ":")
	var uid, gid int64
	if _, err := fmt.Sscanf(uidPart, "%d", &uid); err != nil || fmt.Sprint(uid) != uidPart {
		return false
	}
	a.RunAsUser = &uid
	if gidPart != "" {
		if _, err := fmt.Sscanf(gidPart, "%d", &gid); err == nil && fmt.Sprint(gid) == gidPart {
			a.RunAsGroup = &gid
		}
	}
	return true
}

// sortedKeys returns the keys of m in order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
