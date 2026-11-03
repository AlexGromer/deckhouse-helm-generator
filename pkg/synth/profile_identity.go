package synth

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
)

// ProfileNaming overrides the names dhg gives to the objects of an
// application. Each value is a template (.Name, .Vars); empty keeps the DNS
// name of the application. ConfigMaps, Secrets and claims built from the
// application keep their names, and so does the label app.kubernetes.io/name.
type ProfileNaming struct {
	Deployment string `json:"deployment"`
	Container  string `json:"container"`
	Service    string `json:"service"`
}

// ProfileService sets the type of the Service and adds ports to it.
type ProfileService struct {
	// Type is ClusterIP, NodePort or LoadBalancer; empty leaves the cluster's
	// default. nodePort is never set.
	Type string `json:"type"`
	// ExtraPorts follow the application's own ports in the Service; they do
	// not become container ports.
	ExtraPorts []ProfileServicePort `json:"extraPorts"`
}

// ProfileServicePort is one extra Service port.
type ProfileServicePort struct {
	Name string `json:"name"`
	Port int64  `json:"port"`
	// TargetPort is a number or the name of a container port; Port when unset.
	TargetPort *intstr.IntOrString `json:"targetPort"`
	// Protocol is TCP (default), UDP or SCTP.
	Protocol string `json:"protocol"`
}

// ProfileStrategy is the Deployment strategy.
type ProfileStrategy struct {
	// Type is RollingUpdate (also when only rollingUpdate is set) or Recreate.
	Type          string                `json:"type"`
	RollingUpdate *ProfileRollingUpdate `json:"rollingUpdate"`
}

// ProfileRollingUpdate holds the rolling update limits: a non-negative
// number or a percentage ("25%").
type ProfileRollingUpdate struct {
	MaxSurge       *intstr.IntOrString `json:"maxSurge"`
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable"`
}

var percentValue = regexp.MustCompile(`^[0-9]+%$`)

// validateIdentity checks naming, selector, service and strategy. Strings
// that are templates are checked after they are rendered (applyIdentity).
func (p *Profile) validateIdentity() error {
	for _, n := range []struct {
		path, value string
		check       func(string) []string
	}{
		{"naming.deployment", p.Naming.Deployment, validation.IsDNS1123Subdomain},
		{"naming.container", p.Naming.Container, validation.IsDNS1123Label},
		{"naming.service", p.Naming.Service, validation.IsDNS1035Label},
	} {
		if n.value == "" || strings.Contains(n.value, "{{") {
			continue
		}
		if errs := n.check(n.value); len(errs) > 0 {
			return fmt.Errorf("%s: %q: %s", n.path, n.value, errs[0])
		}
	}
	for _, k := range sortedKeys(p.Selector) {
		if errs := validation.IsQualifiedName(k); len(errs) > 0 {
			return fmt.Errorf("selector: label key %q: %s", k, errs[0])
		}
		if v := p.Selector[k]; !strings.Contains(v, "{{") {
			if err := checkSelectorValue("selector."+k, v); err != nil {
				return err
			}
		}
	}
	if err := p.Service.validate(); err != nil {
		return err
	}
	return p.Strategy.validate()
}

func checkSelectorValue(path, v string) error {
	if v == "" {
		return fmt.Errorf("%s: the value must not be empty", path)
	}
	if errs := validation.IsValidLabelValue(v); len(errs) > 0 {
		return fmt.Errorf("%s: %q: %s", path, v, errs[0])
	}
	return nil
}

func (s ProfileService) validate() error {
	switch s.Type {
	case "", "ClusterIP", "NodePort", "LoadBalancer":
	default:
		return fmt.Errorf("service.type: %q, want ClusterIP, NodePort or LoadBalancer", s.Type)
	}
	names := map[string]bool{}
	ports := map[string]bool{}
	for i, p := range s.ExtraPorts {
		path := fmt.Sprintf("service.extraPorts[%d]", i)
		if p.Name == "" {
			return fmt.Errorf("%s.name is required", path)
		}
		if errs := validation.IsDNS1123Label(p.Name); len(errs) > 0 {
			return fmt.Errorf("%s.name: %q: %s", path, p.Name, errs[0])
		}
		if names[p.Name] {
			return fmt.Errorf("%s.name: %q is used twice", path, p.Name)
		}
		names[p.Name] = true
		if p.Port < 1 || p.Port > 65535 {
			return fmt.Errorf("%s.port: %d is not a port (1-65535)", path, p.Port)
		}
		proto, err := servicePortProtocol(path, p.Protocol)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%d/%s", p.Port, proto)
		if ports[key] {
			return fmt.Errorf("%s.port: %d/%s is used twice", path, p.Port, proto)
		}
		ports[key] = true
		if t := p.TargetPort; t != nil {
			switch t.Type {
			case intstr.Int:
				if t.IntVal < 1 || t.IntVal > 65535 {
					return fmt.Errorf("%s.targetPort: %d is not a port (1-65535)", path, t.IntVal)
				}
			default:
				if errs := validation.IsValidPortName(t.StrVal); len(errs) > 0 {
					return fmt.Errorf("%s.targetPort: %q: %s", path, t.StrVal, errs[0])
				}
			}
		}
	}
	return nil
}

func servicePortProtocol(path, protocol string) (string, error) {
	switch protocol {
	case "":
		return "TCP", nil
	case "TCP", "UDP", "SCTP":
		return protocol, nil
	}
	return "", fmt.Errorf("%s.protocol: %q, want TCP, UDP or SCTP", path, protocol)
}

func (s *ProfileStrategy) validate() error {
	if s == nil {
		return nil
	}
	switch s.Type {
	case "", "RollingUpdate":
	case "Recreate":
		if s.RollingUpdate != nil {
			return fmt.Errorf("strategy.rollingUpdate: not allowed with type Recreate")
		}
	default:
		return fmt.Errorf("strategy.type: %q, want RollingUpdate or Recreate", s.Type)
	}
	r := s.RollingUpdate
	if r == nil {
		return nil
	}
	if err := checkRollingValue("strategy.rollingUpdate.maxSurge", r.MaxSurge, false); err != nil {
		return err
	}
	if err := checkRollingValue("strategy.rollingUpdate.maxUnavailable", r.MaxUnavailable, true); err != nil {
		return err
	}
	if r.MaxSurge != nil && r.MaxUnavailable != nil && rollingIsZero(*r.MaxSurge) && rollingIsZero(*r.MaxUnavailable) {
		return fmt.Errorf("strategy.rollingUpdate: maxSurge and maxUnavailable cannot both be 0")
	}
	return nil
}

// checkRollingValue accepts a non-negative number or a percentage;
// maxUnavailable is at most 100%.
func checkRollingValue(path string, v *intstr.IntOrString, atMost100 bool) error {
	if v == nil {
		return nil
	}
	if v.Type == intstr.Int {
		if v.IntVal < 0 {
			return fmt.Errorf("%s: %d is negative", path, v.IntVal)
		}
		return nil
	}
	if !percentValue.MatchString(v.StrVal) {
		return fmt.Errorf("%s: %q, want a number or a percentage like \"25%%\"", path, v.StrVal)
	}
	if atMost100 {
		if n, err := strconv.Atoi(strings.TrimSuffix(v.StrVal, "%")); err != nil || n > 100 {
			return fmt.Errorf("%s: %q is more than 100%%", path, v.StrVal)
		}
	}
	return nil
}

func rollingIsZero(v intstr.IntOrString) bool {
	if v.Type == intstr.Int {
		return v.IntVal == 0
	}
	return strings.TrimLeft(strings.TrimSuffix(v.StrVal, "%"), "0") == ""
}

// checkIdentityTemplates parses the templates of naming and selector.
func (p *Profile) checkIdentityTemplates(check func(path string, v interface{}) error) error {
	for _, n := range []struct{ path, value string }{
		{"naming.deployment", p.Naming.Deployment},
		{"naming.container", p.Naming.Container},
		{"naming.service", p.Naming.Service},
	} {
		if err := check(n.path, n.value); err != nil {
			return err
		}
	}
	for _, k := range sortedKeys(p.Selector) {
		if err := check("selector."+k, p.Selector[k]); err != nil {
			return err
		}
	}
	return nil
}

// applyIdentity renders the names and the selector, validates the result and
// sets what the profile says on app. It runs after the port defaults, so
// extra Service ports are checked against the application's final ports.
func applyIdentity(app *App, prof *Profile, data profileData) error {
	for _, n := range []struct {
		path, tpl string
		check     func(string) []string
		target    *string
	}{
		{"naming.deployment", prof.Naming.Deployment, validation.IsDNS1123Subdomain, &app.DeploymentName},
		{"naming.container", prof.Naming.Container, validation.IsDNS1123Label, &app.ContainerName},
		{"naming.service", prof.Naming.Service, validation.IsDNS1035Label, &app.ServiceName},
	} {
		if n.tpl == "" {
			continue
		}
		name, err := render(n.path, n.tpl, data)
		if err != nil {
			return err
		}
		if name == "" {
			return fmt.Errorf("%s: the template %q renders to an empty name", n.path, n.tpl)
		}
		if errs := n.check(name); len(errs) > 0 {
			return fmt.Errorf("%s: %q: %s", n.path, name, errs[0])
		}
		*n.target = name
	}

	if len(prof.Selector) > 0 {
		sel := make(map[string]string, len(prof.Selector))
		for _, k := range sortedKeys(prof.Selector) {
			path := "selector." + k
			v, err := render(path, prof.Selector[k], data)
			if err != nil {
				return err
			}
			if err := checkSelectorValue(path, v); err != nil {
				return err
			}
			sel[k] = v
		}
		app.Selector = sel
	}

	if prof.Service.Type != "" {
		app.ServiceType = prof.Service.Type
	}
	if err := applyExtraPorts(app, prof.Service.ExtraPorts); err != nil {
		return err
	}

	if s := prof.Strategy; s != nil {
		app.Strategy = s.object()
	}
	return nil
}

// applyExtraPorts adds the extra Service ports and checks them against the
// application's ports: names and port/protocol pairs must stay unique.
func applyExtraPorts(app *App, extra []ProfileServicePort) error {
	names := map[string]bool{}
	numbers := map[string]bool{}
	for _, p := range app.Ports {
		names[p.Name] = true
		numbers[fmt.Sprintf("%d/%s", p.Port, p.Protocol)] = true
	}
	for _, p := range app.ExtraServicePorts {
		names[p.Name] = true
		numbers[fmt.Sprintf("%d/%s", p.Port, p.Protocol)] = true
	}
	for i, p := range extra {
		path := fmt.Sprintf("service.extraPorts[%d]", i)
		proto, err := servicePortProtocol(path, p.Protocol)
		if err != nil {
			return err
		}
		if names[p.Name] {
			return fmt.Errorf("%s.name: %q is already a port of %s", path, p.Name, app.Name)
		}
		key := fmt.Sprintf("%d/%s", p.Port, proto)
		if numbers[key] {
			return fmt.Errorf("%s.port: %s is already a port of %s", path, key, app.Name)
		}
		names[p.Name], numbers[key] = true, true
		sp := ServicePort{Name: p.Name, Port: p.Port, Protocol: proto}
		if t := p.TargetPort; t != nil {
			if t.Type == intstr.Int {
				sp.TargetPort = int64(t.IntVal)
			} else {
				sp.TargetPortName = t.StrVal
			}
		}
		app.ExtraServicePorts = append(app.ExtraServicePorts, sp)
	}
	return nil
}

// object is the strategy as a JSON-typed map (numbers are int64).
func (s *ProfileStrategy) object() map[string]interface{} {
	typ := s.Type
	if typ == "" && s.RollingUpdate != nil {
		typ = "RollingUpdate"
	}
	if typ == "" {
		return nil
	}
	out := map[string]interface{}{"type": typ}
	if r := s.RollingUpdate; r != nil {
		ru := map[string]interface{}{}
		for key, v := range map[string]*intstr.IntOrString{"maxSurge": r.MaxSurge, "maxUnavailable": r.MaxUnavailable} {
			if v == nil {
				continue
			}
			if v.Type == intstr.Int {
				ru[key] = int64(v.IntVal)
			} else {
				ru[key] = v.StrVal
			}
		}
		if len(ru) > 0 {
			out["rollingUpdate"] = ru
		}
	}
	return out
}
