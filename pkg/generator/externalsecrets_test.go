package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestExternalSecrets_DetectHardcoded(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "test",
		Templates: map[string]string{
			"templates/secret.yaml": `apiVersion: v1
kind: Secret
metadata:
  name: db-secret
type: Opaque
stringData:
  password: "supersecret"
  username: "admin"
`,
		},
		ValuesYAML: `database:
  password: "hardcoded123"
  host: "localhost"
  token: "abc-xyz"
`,
	}

	findings := DetectHardcodedSecrets(chart)

	if len(findings) == 0 {
		t.Fatal("expected to detect hardcoded secrets")
	}

	// Should detect Secret with stringData
	foundSecret := false
	foundPassword := false
	foundToken := false
	for _, f := range findings {
		if f.Kind == "Secret" {
			foundSecret = true
		}
		if strings.Contains(f.Key, "password") {
			foundPassword = true
		}
		if strings.Contains(f.Key, "token") {
			foundToken = true
		}
	}
	if !foundSecret {
		t.Error("should detect kind: Secret with stringData")
	}
	if !foundPassword {
		t.Error("should detect 'password' key in values")
	}
	if !foundToken {
		t.Error("should detect 'token' key in values")
	}
}

func TestExternalSecrets_DetectHardcoded_NoSecrets(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "clean",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: app\n",
		},
		ValuesYAML: `replicas: 3
image: nginx:latest
`,
	}

	findings := DetectHardcodedSecrets(chart)
	if len(findings) != 0 {
		t.Errorf("expected no findings for clean chart, got %d", len(findings))
	}
}

func TestExternalSecrets_ConvertVault(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "test",
		Templates: map[string]string{
			"templates/secret.yaml": `apiVersion: v1
kind: Secret
metadata:
  name: db-secret
type: Opaque
stringData:
  password: "supersecret"
`,
		},
		ValuesYAML: "{}",
	}

	result := ConvertToExternalSecrets(chart, "vault")

	// Original secret template should be replaced
	if _, ok := result.Templates["templates/secret.yaml"]; ok {
		t.Error("original Secret template should be removed after conversion")
	}

	// ExternalSecret should be created
	es, ok := result.Templates["templates/db-secret-externalsecret.yaml"]
	if !ok {
		t.Fatal("expected ExternalSecret template to be generated")
	}

	if !strings.Contains(es, "kind: ExternalSecret") {
		t.Error("should contain ExternalSecret kind")
	}
	if !strings.Contains(es, "vault") {
		t.Error("should reference vault provider")
	}
	if !strings.Contains(es, "db-secret") {
		t.Error("should reference original secret name")
	}
}

func TestExternalSecrets_ConvertAWS(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "test",
		Templates: map[string]string{
			"templates/secret.yaml": `apiVersion: v1
kind: Secret
metadata:
  name: api-keys
type: Opaque
data:
  api-key: dGVzdA==
`,
		},
		ValuesYAML: "{}",
	}

	result := ConvertToExternalSecrets(chart, "aws")

	es, ok := result.Templates["templates/api-keys-externalsecret.yaml"]
	if !ok {
		t.Fatal("expected ExternalSecret template for AWS")
	}

	if !strings.Contains(es, "kind: ExternalSecret") {
		t.Error("should contain ExternalSecret kind")
	}
	if !strings.Contains(es, "aws") {
		t.Error("should reference AWS provider")
	}
}

func TestExternalSecrets_ConvertGCP(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "test",
		Templates: map[string]string{
			"templates/secret.yaml": `apiVersion: v1
kind: Secret
metadata:
  name: gcp-creds
type: Opaque
stringData:
  key: "value"
`,
		},
		ValuesYAML: "{}",
	}

	result := ConvertToExternalSecrets(chart, "gcp")

	es, ok := result.Templates["templates/gcp-creds-externalsecret.yaml"]
	if !ok {
		t.Fatal("expected ExternalSecret template for GCP")
	}

	if !strings.Contains(es, "gcp") {
		t.Error("should reference GCP provider")
	}
}

func TestExternalSecrets_ConvertAzure(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "test",
		Templates: map[string]string{
			"templates/secret.yaml": `apiVersion: v1
kind: Secret
metadata:
  name: azure-creds
type: Opaque
stringData:
  secret: "value"
`,
		},
		ValuesYAML: "{}",
	}

	result := ConvertToExternalSecrets(chart, "azure")

	es, ok := result.Templates["templates/azure-creds-externalsecret.yaml"]
	if !ok {
		t.Fatal("expected ExternalSecret template for Azure")
	}

	if !strings.Contains(es, "azure") {
		t.Error("should reference Azure provider")
	}
}

func TestExternalSecrets_PreservesNonSecretTemplates(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "test",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: app\n",
			"templates/secret.yaml": `apiVersion: v1
kind: Secret
metadata:
  name: my-secret
type: Opaque
stringData:
  key: value
`,
		},
		ValuesYAML: "{}",
	}

	result := ConvertToExternalSecrets(chart, "vault")

	if _, ok := result.Templates["templates/deployment.yaml"]; !ok {
		t.Error("non-Secret templates should be preserved")
	}
}
