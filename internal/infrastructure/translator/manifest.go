package translator

import (
	"fmt"
	"strings"

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
