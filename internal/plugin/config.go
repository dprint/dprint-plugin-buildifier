package plugin

import (
	"fmt"
	"strings"

	"github.com/buger/jsonparser"
)

// Configuration is the resolved configuration of the plugin. buildifier has no
// style options, so the only option is dprint's own `newLineKind`.
type Configuration struct {
	// NewLineKind is the kind of newline to output: `lf` (buildifier's),
	// `crlf` or `auto` (the last newline in the file).
	NewLineKind string
}

type configDiagnostic struct {
	PropertyName string
	Message      string
}

// newLineKinds are the values of dprint's `newLineKind`
var newLineKinds = []string{"auto", "lf", "crlf"}

type configEntry struct {
	key       string
	value     []byte
	valueType jsonparser.ValueType
}

// resolveConfig resolves the plugin's configuration from its JSON object, the
// JSON object of an override for a single format (which may be nil), and the
// global configuration's JSON object.
func resolveConfig(pluginConfig, overrideConfig, globalConfig []byte) (Configuration, []configDiagnostic) {
	var diagnostics []configDiagnostic
	config := Configuration{NewLineKind: "lf"}

	// the global configuration has already been validated by the cli
	if value, valueType, _, err := jsonparser.Get(globalConfig, "newLineKind"); err == nil {
		if kind, ok := parseEnum(value, valueType, newLineKinds); ok {
			config.NewLineKind = kind
		}
	}

	for _, entry := range mergeEntries(objectEntries(pluginConfig), objectEntries(overrideConfig)) {
		if entry.valueType == jsonparser.Null {
			continue
		}
		switch entry.key {
		case "newLineKind":
			if kind, ok := parseEnum(entry.value, entry.valueType, newLineKinds); ok {
				config.NewLineKind = kind
			} else {
				diagnostics = append(diagnostics, configDiagnostic{
					PropertyName: entry.key,
					Message:      fmt.Sprintf("Found invalid value '%s'. Expected one of: %s", entry.value, strings.Join(newLineKinds, ", ")),
				})
			}
		default:
			diagnostics = append(diagnostics, configDiagnostic{
				PropertyName: entry.key,
				Message:      "Unknown property in configuration",
			})
		}
	}

	return config, diagnostics
}

// objectEntries gets the entries of a JSON object in order.
func objectEntries(data []byte) []configEntry {
	var entries []configEntry
	if len(data) == 0 {
		return entries
	}
	_ = jsonparser.ObjectEach(data, func(key, value []byte, valueType jsonparser.ValueType, _ int) error {
		entries = append(entries, configEntry{key: string(key), value: value, valueType: valueType})
		return nil
	})
	return entries
}

// mergeEntries overwrites the entries with the overrides, the same way
// dprint-core merges an override configuration into the plugin configuration.
func mergeEntries(entries, overrides []configEntry) []configEntry {
	for _, override := range overrides {
		replaced := false
		for i := range entries {
			if entries[i].key == override.key {
				entries[i] = override
				replaced = true
				break
			}
		}
		if !replaced {
			entries = append(entries, override)
		}
	}
	return entries
}

func parseEnum(value []byte, valueType jsonparser.ValueType, options []string) (string, bool) {
	if valueType != jsonparser.String {
		return "", false
	}
	text := string(value)
	for _, option := range options {
		if text == option {
			return text, true
		}
	}
	return "", false
}

func marshalConfig(config Configuration) []byte {
	var data []byte
	data = append(data, `{"newLineKind":`...)
	data = appendJSONString(data, config.NewLineKind)
	return append(data, '}')
}

func marshalConfigDiagnostics(diagnostics []configDiagnostic) []byte {
	data := []byte{'['}
	for i, diagnostic := range diagnostics {
		if i != 0 {
			data = append(data, ',')
		}
		data = append(data, `{"propertyName":`...)
		data = appendJSONString(data, diagnostic.PropertyName)
		data = append(data, `,"message":`...)
		data = appendJSONString(data, diagnostic.Message)
		data = append(data, '}')
	}
	return append(data, ']')
}
