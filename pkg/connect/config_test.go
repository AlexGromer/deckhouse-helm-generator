package connect

import (
	"testing"
)

func TestParseConfigFile(t *testing.T) {
	// properties
	props := "key1=val1\n#comment\nkey2=val2\\" + "\ncont"
	kvs := ParseConfigFile("test.properties", props)
	if len(kvs) != 2 || kvs[0].Key != "key1" || kvs[1].Key != "key2" || kvs[1].Value != "val2cont" {
		t.Errorf("properties parse failed: %v", kvs)
	}

	// yaml
	yml := "---\napp:\n  name: ${APP_NAME:default}\n  port: 8080"
	kvs = ParseConfigFile("test.yaml", yml)

	foundName := false
	foundPort := false
	for _, kv := range kvs {
		if kv.Key == "app.name" && kv.Value == "default" {
			foundName = true
		}
		if kv.Key == "app.port" && kv.Value == "8080" {
			foundPort = true
		}
	}
	if !foundName || !foundPort {
		t.Errorf("yaml parse failed: %v", kvs)
	}

	// ini
	ini := "[section]\nkey=val"
	kvs = ParseConfigFile("test.ini", ini)
	if len(kvs) != 1 || kvs[0].Key != "section.key" || kvs[0].Value != "val" {
		t.Errorf("ini parse failed: %v", kvs)
	}

	// json
	jsn := `{"a": {"b": "c"}}`
	kvs = ParseConfigFile("test.json", jsn)
	if len(kvs) != 1 || kvs[0].Key != "a.b" || kvs[0].Value != "c" {
		t.Errorf("json parse failed: %v", kvs)
	}
}

func TestJavaOpts(t *testing.T) {
	kvs := JavaOpts("-Dserver.port=9090 -Ddebug")
	if len(kvs) != 2 || kvs[0].Key != "server.port" || kvs[0].Value != "9090" || kvs[1].Key != "debug" || kvs[1].Value != "true" {
		t.Errorf("JavaOpts failed: %v", kvs)
	}
}

func TestCommandLineProperties(t *testing.T) {
	kvs := CommandLineProperties([]string{"--port=8080", "--spring.application.json={\"a\":\"b\"}"})
	foundPort := false
	foundA := false
	for _, kv := range kvs {
		if kv.Key == "port" && kv.Value == "8080" {
			foundPort = true
		}
		if kv.Key == "a" && kv.Value == "b" {
			foundA = true
		}
	}
	if !foundPort || !foundA {
		t.Errorf("CommandLineProperties failed: %v", kvs)
	}
}

func TestApplicationJSON(t *testing.T) {
	kvs := ApplicationJSON(`{"x": "y"}`)
	if len(kvs) != 1 || kvs[0].Key != "x" || kvs[0].Value != "y" {
		t.Errorf("ApplicationJSON failed: %v", kvs)
	}
}
