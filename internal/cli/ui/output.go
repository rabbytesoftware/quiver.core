package ui

import (
	"encoding/json"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Output formats accepted by the --output flag.
const (
	FormatTable = "table"
	FormatJSON  = "json"
	FormatYAML  = "yaml"
)

// ResolveFormat picks the effective output format: an explicit flag wins,
// otherwise table on a TTY and json when piped.
func ResolveFormat(flag string, isTTY bool) (string, error) {
	switch flag {
	case "":
		if isTTY {
			return FormatTable, nil
		}
		return FormatJSON, nil
	case FormatTable, FormatJSON, FormatYAML:
		return flag, nil
	default:
		return "", fmt.Errorf("unknown output format %q (table|json|yaml)", flag)
	}
}

// WriteJSON writes indented JSON.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

// WriteYAML writes YAML.
func WriteYAML(w io.Writer, v any) error {
	raw, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("write yaml: %w", err)
	}
	return nil
}
