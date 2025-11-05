package validator

import (
	"encoding/json"
	"fmt"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

type ValidatorImplementation struct{}

func NewValidator() Validator {
	return &ValidatorImplementation{}
}

func (v *ValidatorImplementation) Validate(schemaJSON []byte, yamlData []byte) (*ValidationResult, error) {
	var yamlMap map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &yamlMap); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	jsonData, err := json.Marshal(yamlMap)
	if err != nil {
		return nil, fmt.Errorf("failed to convert YAML to JSON: %w", err)
	}

	schemaLoader := gojsonschema.NewBytesLoader(schemaJSON)
	documentLoader := gojsonschema.NewBytesLoader(jsonData)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return nil, fmt.Errorf("validation error: %w", err)
	}

	validationResult := &ValidationResult{
		Valid:  result.Valid(),
		Errors: []ValidationError{},
	}

	if !result.Valid() {
		for _, desc := range result.Errors() {
			validationResult.Errors = append(validationResult.Errors, ValidationError{
				Field:   desc.Field(),
				Message: desc.Description(),
				Value:   desc.Value(),
			})
		}
	}

	return validationResult, nil
}
