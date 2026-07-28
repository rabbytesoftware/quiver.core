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

// WriteYAML writes YAML. It routes through JSON first so the output honors the
// value's json tags — the API DTOs carry only json tags, and marshalling them
// with yaml directly would emit lowercased Go field names.
func WriteYAML(w io.Writer, v any) error {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	var generic any
	if err := json.Unmarshal(jsonBytes, &generic); err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	raw, err := yaml.Marshal(generic)
	if err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("write yaml: %w", err)
	}
	return nil
}
