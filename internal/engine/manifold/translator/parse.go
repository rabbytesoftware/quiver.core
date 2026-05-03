package translator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

type ManifestInfo struct {
	SchemaType  string
	Version     string
	ManifestKey string
}

type temporaryManifest struct {
	Schema   string `yaml:"schema"`
	Manifest string `yaml:"manifest"`
}

func parseManifestString(manifestStr string) (*ManifestInfo, error) {
	manifestStr = strings.TrimSpace(manifestStr)
	manifestStr = strings.Trim(manifestStr, "\"'")

	parts := strings.Split(manifestStr, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid manifest format: %s (expected: schema@version)", manifestStr)
	}

	schemaType := strings.TrimSpace(parts[0])
	version := strings.TrimSpace(parts[1])

	if schemaType == "" || version == "" {
		return nil, fmt.Errorf("invalid manifest: schema or version is empty")
	}

	return &ManifestInfo{
		SchemaType:  schemaType,
		Version:     version,
		ManifestKey: schemaType + "@" + version,
	}, nil
}

func extractManifestFromYAML(yamlData []byte) (*ManifestInfo, error) {
	manifestStr, err := extractSchemaField(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract manifest from YAML: %w", err)
	}

	if manifestStr == "" {
		return nil, fmt.Errorf("manifest field not found in YAML")
	}

	return parseManifestString(manifestStr)
}

func extractSchemaField(yamlData []byte) (string, error) {
	var temp temporaryManifest
	if err := yaml.Unmarshal(yamlData, &temp); err != nil {
		return "", fmt.Errorf("failed to parse YAML: %w", err)
	}
	if temp.Schema != "" {
		return temp.Schema, nil
	}
	if temp.Manifest != "" {
		return temp.Manifest, nil
	}
	return "", nil
}

// ─── YAML validation ──────────────────────────────────────────────────────────

func validateYAML(schemaJSON, yamlData []byte) error {
	var yamlMap map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &yamlMap); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	jsonData, err := json.Marshal(yamlMap)
	if err != nil {
		return fmt.Errorf("failed to convert YAML to JSON: %w", err)
	}

	schemaLoader := gojsonschema.NewBytesLoader(schemaJSON)
	documentLoader := gojsonschema.NewBytesLoader(jsonData)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	if !result.Valid() {
		var errorMessages []string
		for _, desc := range result.Errors() {
			errorMessages = append(errorMessages, fmt.Sprintf("%s: %s", desc.Field(), desc.Description()))
		}
		return fmt.Errorf("validation failed: %s", strings.Join(errorMessages, "; "))
	}

	return nil
}
