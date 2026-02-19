package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

func makeDexAuthenticatorObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1",
			"kind":       "DexAuthenticator",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
	if spec != nil {
		obj.Object["spec"] = spec
	}
	return obj
}

func TestDexAuthenticatorProcessor_Name(t *testing.T) {
	proc := NewDexAuthenticatorProcessor()
	testutil.AssertEqual(t, "dexauthenticator", proc.Name(), "processor name")
}

func TestDexAuthenticatorProcessor_Supports(t *testing.T) {
	proc := NewDexAuthenticatorProcessor()
	gvks := proc.Supports()
	if len(gvks) != 1 {
		t.Fatalf("Expected 1 GVK, got %d", len(gvks))
	}
	expected := schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "DexAuthenticator"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

func TestDexAuthenticatorProcessor_ApplicationDomain(t *testing.T) {
	proc := NewDexAuthenticatorProcessor()
	ctx := newTestProcessorContext()

	obj := makeDexAuthenticatorObj("app-dex", "default", map[string]interface{}{
		"applicationDomain": "app.example.com",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "app.example.com", result.Values["applicationDomain"], "applicationDomain")
}

func TestDexAuthenticatorProcessor_SendAuthorizationHeader(t *testing.T) {
	proc := NewDexAuthenticatorProcessor()
	ctx := newTestProcessorContext()

	obj := makeDexAuthenticatorObj("app-dex", "default", map[string]interface{}{
		"applicationDomain":        "app.example.com",
		"sendAuthorizationHeader":  true,
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Values["sendAuthorizationHeader"], "sendAuthorizationHeader")
}

func TestDexAuthenticatorProcessor_IngressClassName(t *testing.T) {
	proc := NewDexAuthenticatorProcessor()
	ctx := newTestProcessorContext()

	obj := makeDexAuthenticatorObj("app-dex", "default", map[string]interface{}{
		"applicationDomain":           "app.example.com",
		"applicationIngressClassName": "nginx",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "nginx", result.Values["applicationIngressClassName"], "applicationIngressClassName")
}

func TestDexAuthenticatorProcessor_AllowedGroups(t *testing.T) {
	proc := NewDexAuthenticatorProcessor()
	ctx := newTestProcessorContext()

	obj := makeDexAuthenticatorObj("app-dex", "default", map[string]interface{}{
		"applicationDomain": "app.example.com",
		"allowedGroups":     []interface{}{"admins", "developers"},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	groups, ok := result.Values["allowedGroups"].([]interface{})
	if !ok {
		t.Fatal("Expected allowedGroups as slice")
	}
	if len(groups) != 2 {
		t.Fatalf("Expected 2 groups, got %d", len(groups))
	}
}

func TestDexAuthenticatorProcessor_Template(t *testing.T) {
	proc := NewDexAuthenticatorProcessor()
	ctx := newTestProcessorContext()

	obj := makeDexAuthenticatorObj("app-dex", "default", map[string]interface{}{
		"applicationDomain": "app.example.com",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: deckhouse.io/v1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: DexAuthenticator", "kind")
	testutil.AssertContains(t, tpl, ".applicationDomain", "applicationDomain ref")
}

func TestDexAuthenticatorProcessor_ServiceName(t *testing.T) {
	proc := NewDexAuthenticatorProcessor()
	ctx := newTestProcessorContext()

	obj := makeDexAuthenticatorObj("grafana-dex", "monitoring", map[string]interface{}{
		"applicationDomain": "grafana.example.com",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "grafana-dex", result.ServiceName, "ServiceName")
}
