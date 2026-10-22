package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

// write mimics the host writing data into the plugin's shared bytes
func write(p *Plugin, data string) {
	p.ClearSharedBytes(uint32(len(data)))
	copy(p.sharedBytes, data)
}

func TestInfo(t *testing.T) {
	var p Plugin
	if size := p.Info("1.2.3"); size != uint32(len(p.sharedBytes)) {
		t.Fatalf("Info() = %d, want %d", size, len(p.sharedBytes))
	}
	var info struct {
		Name            string `json:"name"`
		Version         string `json:"version"`
		ConfigKey       string `json:"configKey"`
		HelpURL         string `json:"helpUrl"`
		ConfigSchemaURL string `json:"configSchemaUrl"`
		UpdateURL       string `json:"updateUrl"`
	}
	if err := json.Unmarshal(p.takeFromSharedBytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Name != "dprint-plugin-buildifier" ||
		info.Version != "1.2.3" ||
		info.ConfigKey != "buildifier" ||
		info.HelpURL != "https://dprint.dev/plugins/buildifier" ||
		info.ConfigSchemaURL != "https://plugins.dprint.dev/dprint/dprint-plugin-buildifier/1.2.3/schema.json" ||
		info.UpdateURL != "https://plugins.dprint.dev/dprint/dprint-plugin-buildifier/latest.json" {
		t.Fatalf("unexpected plugin info: %#v", info)
	}
}

func TestConfigFileMatching(t *testing.T) {
	var p Plugin
	p.ConfigFileMatching(1)
	var matching struct {
		FileExtensions []string `json:"fileExtensions"`
		FileNames      []string `json:"fileNames"`
		Additive       bool     `json:"additive"`
	}
	if err := json.Unmarshal(p.takeFromSharedBytes(), &matching); err != nil {
		t.Fatal(err)
	}
	if strings.Join(matching.FileExtensions, ",") != "bzl,bazel,star,sky" ||
		strings.Join(matching.FileNames, ",") != "BUILD,WORKSPACE,WORKSPACE.bzlmod" ||
		matching.Additive {
		t.Fatalf("unexpected file matching: %#v", matching)
	}
}

func TestConfigsByID(t *testing.T) {
	var p Plugin
	write(&p, `{"plugin":{},"global":{"indentWidth":4,"newLineKind":"crlf"}}`)
	p.RegisterConfig(1)
	write(&p, `{"plugin":{"indentWidth":4,"unknown":null},"global":{}}`)
	p.RegisterConfig(2)

	p.ResolvedConfig(1)
	if resolved := string(p.takeFromSharedBytes()); resolved != `{"newLineKind":"crlf"}` {
		t.Fatalf("unexpected resolved config: %s", resolved)
	}

	// the other global options don't apply, so they get no diagnostics
	p.ConfigDiagnostics(1)
	if diagnostics := string(p.takeFromSharedBytes()); diagnostics != "[]" {
		t.Fatalf("unexpected diagnostics for config 1: %s", diagnostics)
	}
	p.ConfigDiagnostics(2)
	// a null value is the same as leaving the property out
	expected := `[{"propertyName":"indentWidth","message":"Unknown property in configuration"}]`
	if diagnostics := string(p.takeFromSharedBytes()); diagnostics != expected {
		t.Fatalf("unexpected diagnostics for config 2: %s", diagnostics)
	}

	p.ReleaseConfig(1)
	if _, ok := p.configs[1]; ok {
		t.Fatal("config 1 was not released")
	}
}

func TestFormat(t *testing.T) {
	var p Plugin
	write(&p, `{"plugin":{},"global":{}}`)
	p.RegisterConfig(1)

	// windows paths are normalized, so the file name is found
	write(&p, `C:\repo\pkg\BUILD`)
	p.SetFilePath()
	write(&p, "cc_library(deps = [\":b\", \":a\"], name = \"x\")\n")
	if result := p.Format(1); result != formatChanged {
		t.Fatalf("Format() = %d, want %d", result, formatChanged)
	}
	p.FormattedText()
	if formatted := string(p.takeFromSharedBytes()); formatted != "cc_library(\n    name = \"x\",\n    deps = [\n        \":a\",\n        \":b\",\n    ],\n)\n" {
		t.Fatalf("unexpected formatted text: %q", formatted)
	}
	p.FormattedText()
	if formatted := p.takeFromSharedBytes(); formatted != nil {
		t.Fatalf("FormattedText() did not clear the result: %q", formatted)
	}

	write(&p, "/repo/BUILD")
	p.SetFilePath()
	write(&p, "cc_library(name = \"x\")\n")
	if result := p.Format(1); result != formatNoChange {
		t.Fatalf("Format() = %d, want %d", result, formatNoChange)
	}

	write(&p, "/repo/defs.bzl")
	p.SetFilePath()
	write(&p, "def f(:\n")
	if result := p.Format(1); result != formatError {
		t.Fatalf("Format() = %d, want %d", result, formatError)
	}
	p.ErrorText()
	if errorText := string(p.takeFromSharedBytes()); errorText != "1:8: syntax error near :" {
		t.Fatalf("unexpected error text: %q", errorText)
	}
}

func TestFormatWithOverrideConfig(t *testing.T) {
	var p Plugin
	write(&p, `{"plugin":{},"global":{}}`)
	p.RegisterConfig(1)

	write(&p, `{"newLineKind":"crlf"}`)
	p.SetOverrideConfig()
	write(&p, "/repo/BUILD")
	p.SetFilePath()
	write(&p, "x=1\n")
	if result := p.Format(1); result != formatChanged {
		t.Fatalf("Format() = %d, want %d", result, formatChanged)
	}
	p.FormattedText()
	if formatted := string(p.takeFromSharedBytes()); formatted != "x = 1\r\n" {
		t.Fatalf("override not applied: %q", formatted)
	}

	// the override only applies to the one format
	write(&p, "/repo/BUILD")
	p.SetFilePath()
	write(&p, "x=1\n")
	p.Format(1)
	p.FormattedText()
	if formatted := string(p.takeFromSharedBytes()); formatted != "x = 1\n" {
		t.Fatalf("override was kept: %q", formatted)
	}
}

func TestFormatRange(t *testing.T) {
	var p Plugin
	write(&p, `{"plugin":{},"global":{}}`)
	p.RegisterConfig(1)

	write(&p, `{"some":"override"}`)
	p.SetOverrideConfig()
	write(&p, "/repo/BUILD")
	p.SetFilePath()
	write(&p, "x=1\n")
	if result := p.FormatRange(1, 0, 2); result != formatNoChange {
		t.Fatalf("FormatRange() = %d, want %d", result, formatNoChange)
	}
	if p.sharedBytes != nil || p.filePath != "" || p.overrideConfig != nil {
		t.Fatal("FormatRange() did not clear the state of the format")
	}
}

func TestCheckConfigUpdates(t *testing.T) {
	var p Plugin
	write(&p, `{"oldVersion":"0.0.1","config":{}}`)
	p.CheckConfigUpdates()
	if result := string(p.takeFromSharedBytes()); result != `{"kind":"ok","data":[]}` {
		t.Fatalf("unexpected result: %s", result)
	}
}
