package translator

import (
	"testing"
)

func TestValidateYAML(t *testing.T) {
	tests := []struct {
		name       string
		schemaJSON []byte
		yamlData   []byte
		wantErr    bool
	}{
		{
			name: "valid data with simple schema",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"name": {"type": "string"}
				},
				"required": ["name"]
			}`),
			yamlData: []byte("name: test"),
			wantErr:  false,
		},
		{
			name: "missing required field",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"name": {"type": "string"}
				},
				"required": ["name"]
			}`),
			yamlData: []byte("other: value"),
			wantErr:  true,
		},
		{
			name: "wrong type",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"age": {"type": "number"}
				},
				"required": ["age"]
			}`),
			yamlData: []byte("age: not-a-number"),
			wantErr:  true,
		},
		{
			name:       "invalid YAML data",
			schemaJSON: []byte(`{"type": "object"}`),
			yamlData:   []byte("invalid: yaml: [[["),
			wantErr:    true,
		},
		{
			name:       "invalid schema JSON",
			schemaJSON: []byte(`{invalid json`),
			yamlData:   []byte("name: test"),
			wantErr:    true,
		},
		{
			name: "empty YAML data",
			schemaJSON: []byte(`{
				"type": "object",
				"required": ["name"]
			}`),
			yamlData: []byte(""),
			wantErr:  true,
		},
		{
			name: "additional properties not allowed",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"name": {"type": "string"}
				},
				"additionalProperties": false
			}`),
			yamlData: []byte("name: test\nextra: field"),
			wantErr:  true,
		},
		{
			name: "nested object validation",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"metadata": {
						"type": "object",
						"properties": {
							"name": {"type": "string"}
						},
						"required": ["name"]
					}
				},
				"required": ["metadata"]
			}`),
			yamlData: []byte("metadata:\n  name: test"),
			wantErr:  false,
		},
		{
			name: "nested object missing required",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"metadata": {
						"type": "object",
						"properties": {
							"name": {"type": "string"}
						},
						"required": ["name"]
					}
				},
				"required": ["metadata"]
			}`),
			yamlData: []byte("metadata:\n  other: value"),
			wantErr:  true,
		},
		{
			name: "array validation",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"items": {
						"type": "array",
						"items": {"type": "string"}
					}
				}
			}`),
			yamlData: []byte("items:\n  - item1\n  - item2"),
			wantErr:  false,
		},
		{
			name: "array with wrong item type",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"items": {
						"type": "array",
						"items": {"type": "string"}
					}
				}
			}`),
			yamlData: []byte("items:\n  - 123\n  - 456"),
			wantErr:  true,
		},
		{
			name: "enum validation success",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"status": {
						"type": "string",
						"enum": ["active", "inactive"]
					}
				}
			}`),
			yamlData: []byte("status: active"),
			wantErr:  false,
		},
		{
			name: "enum validation failure",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"status": {
						"type": "string",
						"enum": ["active", "inactive"]
					}
				}
			}`),
			yamlData: []byte("status: unknown"),
			wantErr:  true,
		},
		{
			name: "pattern validation success",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"version": {
						"type": "string",
						"pattern": "^v[0-9]+$"
					}
				}
			}`),
			yamlData: []byte("version: v1"),
			wantErr:  false,
		},
		{
			name: "pattern validation failure",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"version": {
						"type": "string",
						"pattern": "^v[0-9]+$"
					}
				}
			}`),
			yamlData: []byte("version: invalid"),
			wantErr:  true,
		},
		{
			name: "minimum value validation",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"count": {
						"type": "number",
						"minimum": 1
					}
				}
			}`),
			yamlData: []byte("count: 0"),
			wantErr:  true,
		},
		{
			name: "maximum value validation",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"count": {
						"type": "number",
						"maximum": 10
					}
				}
			}`),
			yamlData: []byte("count: 11"),
			wantErr:  true,
		},
		{
			name: "complex nested structure success",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"data": {
						"type": "object",
						"properties": {
							"items": {
								"type": "array",
								"items": {
									"type": "object",
									"properties": {
										"id": {"type": "integer"},
										"name": {"type": "string"}
									},
									"required": ["id", "name"]
								}
							}
						}
					}
				}
			}`),
			yamlData: []byte(`data:
  items:
    - id: 1
      name: Item1
    - id: 2
      name: Item2
`),
			wantErr: false,
		},
		{
			name: "multiple validation errors",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"age": {"type": "number"},
					"email": {"type": "string", "format": "email"}
				},
				"required": ["name", "age", "email"]
			}`),
			yamlData: []byte(`name: 123
age: not-a-number
`),
			wantErr: true,
		},
		{
			name: "string length validation",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"name": {
						"type": "string",
						"minLength": 3,
						"maxLength": 10
					}
				}
			}`),
			yamlData: []byte("name: ab"),
			wantErr:  true,
		},
		{
			name: "boolean type validation",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"active": {"type": "boolean"}
				}
			}`),
			yamlData: []byte("active: true"),
			wantErr:  false,
		},
		{
			name: "null value",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"value": {"type": ["string", "null"]}
				}
			}`),
			yamlData: []byte("value: null"),
			wantErr:  false,
		},
		{
			name: "deeply nested structure",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"level1": {
						"type": "object",
						"properties": {
							"level2": {
								"type": "object",
								"properties": {
									"level3": {
										"type": "object",
										"properties": {
											"value": {"type": "string"}
										}
									}
								}
							}
						}
					}
				}
			}`),
			yamlData: []byte(`level1:
  level2:
    level3:
      value: deep
`),
			wantErr: false,
		},
		{
			name: "empty object validation",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object"
			}`),
			yamlData: []byte("{}"),
			wantErr:  false,
		},
		{
			name: "array of mixed types",
			schemaJSON: []byte(`{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {
					"mixed": {
						"type": "array"
					}
				}
			}`),
			yamlData: []byte(`mixed:
  - string
  - 123
  - true
  - null
`),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateYAML(tt.schemaJSON, tt.yamlData)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateYAML() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
