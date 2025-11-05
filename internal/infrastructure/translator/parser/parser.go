package parser

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type ParserImplementation struct{}

func NewParser() Parser {
	return &ParserImplementation{}
}

func (p *ParserImplementation) Parse(manifestStr string) (*ManifestInfo, error) {
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

func (p *ParserImplementation) ParseFromYAML(yamlData []byte) (*ManifestInfo, error) {
	manifestStr, err := extractManifestFromYAML(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract manifest from YAML: %w", err)
	}
	if manifestStr == "" {
		return nil, fmt.Errorf("manifest field not found in YAML")
	}
	return p.Parse(manifestStr)
}

func extractManifestFromYAML(yamlData []byte) (string, error) {
	var temp temporaryManifest
	if err := yaml.Unmarshal(yamlData, &temp); err != nil {
		return "", fmt.Errorf("failed to parse YAML: %w", err)
	}
	return temp.Manifest, nil
}
