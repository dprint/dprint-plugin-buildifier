package plugin

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestResolveConfigDefaults(t *testing.T) {
	config, diagnostics := resolveConfig(nil, nil, nil)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if config != (Configuration{NewLineKind: "lf"}) {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestResolveConfigGlobal(t *testing.T) {
	// the global newLineKind applies, and the other global options don't
	config, diagnostics := resolveConfig(nil, nil, []byte(`{"newLineKind":"crlf","indentWidth":2,"lineWidth":80}`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if config.NewLineKind != "crlf" {
		t.Fatalf("unexpected config: %#v", config)
	}

	config, _ = resolveConfig([]byte(`{"newLineKind":"auto"}`), nil, []byte(`{"newLineKind":"crlf"}`))
	if config.NewLineKind != "auto" {
		t.Fatalf("the plugin config didn't take precedence: %#v", config)
	}
}

func TestResolveConfigOverride(t *testing.T) {
	config, _ := resolveConfig([]byte(`{"newLineKind":"crlf"}`), []byte(`{"newLineKind":"auto"}`), nil)
	if config.NewLineKind != "auto" {
		t.Fatalf("the override config didn't take precedence: %#v", config)
	}
}

func TestResolveConfigDiagnostics(t *testing.T) {
	config, diagnostics := resolveConfig([]byte(`{"newLineKind":"cr","indentWidth":4,"unknown":null}`), nil, nil)
	expected := []configDiagnostic{
		{PropertyName: "newLineKind", Message: "Found invalid value 'cr'. Expected one of: auto, lf, crlf"},
		{PropertyName: "indentWidth", Message: "Unknown property in configuration"},
	}
	if !reflect.DeepEqual(diagnostics, expected) {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if config.NewLineKind != "lf" {
		t.Fatalf("an invalid value changed the config: %#v", config)
	}
}

// the schema documents the defaults, so it needs to stay in sync with them
func TestSchemaMatchesDefaults(t *testing.T) {
	data, err := os.ReadFile("../../deployment/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		ID         string `json:"$id"`
		Properties map[string]struct {
			Default any `json:"default"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.ID != "https://plugins.dprint.dev/dprint/dprint-plugin-buildifier/0.0.0/schema.json" {
		t.Fatalf("unexpected $id: %s", schema.ID)
	}
	config, _ := resolveConfig(nil, nil, nil)
	var defaults map[string]any
	if err := json.Unmarshal(marshalConfig(config), &defaults); err != nil {
		t.Fatal(err)
	}
	if len(schema.Properties) != len(defaults) {
		t.Fatalf("schema has %d properties, but the config has %d", len(schema.Properties), len(defaults))
	}
	for key, value := range defaults {
		property, ok := schema.Properties[key]
		if !ok {
			t.Errorf("schema is missing %q", key)
		} else if !reflect.DeepEqual(property.Default, value) {
			t.Errorf("default of %q is %v in the schema, but %v in the config", key, property.Default, value)
		}
	}
}
