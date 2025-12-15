package translator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

func validateYAML(schemaJSON []byte, yamlData []byte) error {
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
