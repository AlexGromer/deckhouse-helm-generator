package synth

import "testing"

func TestIsSensitiveWholeWordParts(t *testing.T) {
	secret := []string{
		"DB_PASSWORD", "db_password", "SVC_PASSWD", "KEYCLOAK_CLIENT_SECRET", "AUTH_TOKEN", "TOKEN",
		"SERVICE_CREDENTIALS", "TLS_PRIVATE_KEY", "PRIVATEKEY", "STRIPE_API_KEY", "API_KEY", "APIKEY",
		"S3_ACCESS_KEY", "SPRING_DATASOURCE_PASSWORD", "DBPASSWORD", "spring.security.oauth2.client-secret",
		"HIKARI_PASSWORDS",
	}
	for _, name := range secret {
		if !IsSensitive(name) {
			t.Errorf("%s must go to a Secret", name)
		}
	}
	plain := []string{
		"KEYCLOAK_URL", "TOKEN_URL", "TOKEN_URI", "AUTH_TOKEN_HOST", "PASSWORD_PORT", "SECRET_ENDPOINT",
		"LOGGING_LEVEL", "KAFKA_HOST", "SECRETARY_NAME", "KEYSTORE_TYPE", "PRIMARY_KEY", "DB_PASSWORD_FILE",
		"TOKEN_ENABLED", "MONKEY_COUNT", "SUPPORT",
		// Limits, durations and sizes of a credential are not the credential.
		"MAX_TOKENS", "OPENAI_MAX_TOKENS", "TOKEN_TTL", "TOKEN_EXPIRY", "TOKEN_EXPIRATION", "PASSWORD_MIN_LENGTH",
		"SECRET_MAX_AGE_COUNT", "PASSWORD_LIMIT", "TOKEN_TIMEOUT", "API_KEY_MAX", "PASSWORD_MIN",
	}
	for _, name := range plain {
		if IsSensitive(name) {
			t.Errorf("%s must stay in the ConfigMap", name)
		}
	}
}

func TestSecretRulesFromProfileReplaceTheDefaults(t *testing.T) {
	rules, err := NewSecretRules([]string{`^X_.*_PIN$`, `(?i)vault_`})
	if err != nil {
		t.Fatal(err)
	}
	if !rules.Match("X_CARD_PIN") || !rules.Match("MY_VAULT_ADDR") {
		t.Error("profile patterns must match (case-insensitive)")
	}
	if rules.Match("DB_PASSWORD") {
		t.Error("profile patterns replace the built-in heuristic")
	}
	if _, err := NewSecretRules([]string{"("}); err == nil {
		t.Error("an invalid pattern must be an error")
	}

	app := App{Name: "x", secrets: rules}
	app.SetEnv("DB_PASSWORD", "")
	app.SetEnv("X_CARD_PIN", "")
	if len(app.Env) != 1 || app.Env[0].Name != "DB_PASSWORD" || len(app.SecretEnv) != 1 || app.SecretEnv[0].Name != "X_CARD_PIN" {
		t.Errorf("env = %+v secret = %+v", app.Env, app.SecretEnv)
	}
}
