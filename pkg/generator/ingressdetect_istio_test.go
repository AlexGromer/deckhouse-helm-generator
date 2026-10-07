package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestIngressDetect_IstioFromClass(t *testing.T) {
	byAnnotation := makeIngressResource("Ingress", "web", nil, map[string]string{"kubernetes.io/ingress.class": "istio"})
	if got := DetectIngressController([]*types.ProcessedResource{byAnnotation}); got != ControllerIstio {
		t.Errorf("annotation istio: got %q, want istio", got)
	}

	byClassName := makeIngressResource("Ingress", "web", nil, nil)
	byClassName.Original.Object.Object["spec"] = map[string]interface{}{"ingressClassName": "istio"}
	if got := DetectIngressController([]*types.ProcessedResource{byClassName}); got != ControllerIstio {
		t.Errorf("ingressClassName istio: got %q, want istio", got)
	}

	nginx := makeIngressResource("Ingress", "web", nil, nil)
	nginx.Original.Object.Object["spec"] = map[string]interface{}{"ingressClassName": "nginx"}
	if got := DetectIngressController([]*types.ProcessedResource{nginx}); got != ControllerNginx {
		t.Errorf("ingressClassName nginx: got %q, want nginx", got)
	}

	// The annotation wins over spec.ingressClassName.
	both := makeIngressResource("Ingress", "web", nil, map[string]string{"kubernetes.io/ingress.class": "traefik"})
	both.Original.Object.Object["spec"] = map[string]interface{}{"ingressClassName": "istio"}
	if got := DetectIngressController([]*types.ProcessedResource{both}); got != ControllerTraefik {
		t.Errorf("annotation and class: got %q, want traefik", got)
	}
}

func TestIngressDetect_IstioAddsNoAnnotations(t *testing.T) {
	got := GenerateIngressAnnotations(ControllerIstio, []IngressFeature{IngressSSLRedirect, IngressRewrite})
	if len(got) != 0 {
		t.Errorf("Istio must get no Ingress annotations, got %v", got)
	}
	for _, want := range []string{"ingressClassName: istio", "--with istio-ingress", "istio-system"} {
		if !strings.Contains(IstioIngressNote, want) {
			t.Errorf("IstioIngressNote lacks %q: %s", want, IstioIngressNote)
		}
	}
}
