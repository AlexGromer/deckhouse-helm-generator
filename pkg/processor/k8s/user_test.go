package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

func makeUserObj(name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1",
			"kind":       "User",
			"metadata":   map[string]interface{}{"name": name},
			"spec":       spec,
		},
	}
}

func TestUserProcessor_Name(t *testing.T) {
	proc := NewUserProcessor()
	testutil.AssertEqual(t, "user", proc.Name(), "processor name")
}

func TestUserProcessor_Supports(t *testing.T) {
	proc := NewUserProcessor()
	gvks := proc.Supports()
	if len(gvks) != 1 {
		t.Fatalf("Expected 1 GVK, got %d", len(gvks))
	}
	expected := schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "User"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

func TestUserProcessor_Email(t *testing.T) {
	proc := NewUserProcessor()
	ctx := newTestProcessorContext()

	obj := makeUserObj("admin", map[string]interface{}{
		"email":    "admin@example.com",
		"password": "hashedpassword123",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "admin@example.com", result.Values["email"], "email")
}

func TestUserProcessor_Groups(t *testing.T) {
	proc := NewUserProcessor()
	ctx := newTestProcessorContext()

	obj := makeUserObj("admin", map[string]interface{}{
		"email":  "admin@example.com",
		"groups": []interface{}{"admins", "developers"},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	groups, ok := result.Values["groups"].([]interface{})
	if !ok {
		t.Fatal("Expected groups as slice")
	}
	if len(groups) != 2 {
		t.Fatalf("Expected 2 groups, got %d", len(groups))
	}
}

func TestUserProcessor_TTL(t *testing.T) {
	proc := NewUserProcessor()
	ctx := newTestProcessorContext()

	obj := makeUserObj("temp-user", map[string]interface{}{
		"email": "temp@example.com",
		"ttl":   "24h",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "24h", result.Values["ttl"], "ttl")
}

func TestUserProcessor_Password_NotInValues(t *testing.T) {
	proc := NewUserProcessor()
	ctx := newTestProcessorContext()

	obj := makeUserObj("admin", map[string]interface{}{
		"email":    "admin@example.com",
		"password": "supersecret",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// Password must NOT be in values (sensitive)
	if _, ok := result.Values["password"]; ok {
		t.Error("Password should NOT be in values (sensitive field)")
	}
}

func TestUserProcessor_Password_InMetadata(t *testing.T) {
	proc := NewUserProcessor()
	ctx := newTestProcessorContext()

	obj := makeUserObj("admin", map[string]interface{}{
		"email":    "admin@example.com",
		"password": "supersecret",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// Password should be flagged in metadata as sensitive
	sensitive, ok := result.Metadata["sensitive_fields"]
	if !ok {
		t.Fatal("Expected sensitive_fields in metadata")
	}

	fields, ok := sensitive.([]string)
	if !ok {
		t.Fatal("Expected sensitive_fields to be []string")
	}

	found := false
	for _, f := range fields {
		if f == "password" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'password' in sensitive_fields")
	}
}

func TestUserProcessor_ServiceName(t *testing.T) {
	proc := NewUserProcessor()
	ctx := newTestProcessorContext()

	obj := makeUserObj("john-doe", map[string]interface{}{
		"email": "john@example.com",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "john-doe", result.ServiceName, "ServiceName")
}
