package domain

import (
	"testing"
)

func TestVariable_Structure(t *testing.T) {
	// Test Variable struct with all fields
	variable := Variable{
		Name:        "TEST_VAR",
		Description: "A test variable",
		Default:     "default_value",
		Values:      []string{"value1", "value2", "value3"},
		Min:         1,
		Max:         100,
		Sensitive:   true,
		Type:        VariableType("string"),
	}

	// Test field access
	if variable.Name != "TEST_VAR" {
		t.Errorf("Expected Name 'TEST_VAR', got %q", variable.Name)
	}

	if variable.Description != "A test variable" {
		t.Errorf("Expected Description 'A test variable', got %q", variable.Description)
	}

	if variable.Default != "default_value" {
		t.Errorf("Expected Default 'default_value', got %q", variable.Default)
	}

	if len(variable.Values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(variable.Values))
	}

	if variable.Values[0] != "value1" {
		t.Errorf("Expected first value 'value1', got %q", variable.Values[0])
	}

	if variable.Min != 1 {
		t.Errorf("Expected Min 1, got %d", variable.Min)
	}

	if variable.Max != 100 {
		t.Errorf("Expected Max 100, got %d", variable.Max)
	}

	if !variable.Sensitive {
		t.Error("Expected Sensitive to be true")
	}

	if !variable.Type.IsString() {
		t.Error("Expected Type to be string")
	}
}

func TestVariable_EmptyVariable(t *testing.T) {
	// Test empty variable
	variable := Variable{}

	if variable.Name != "" {
		t.Errorf("Expected empty Name, got %q", variable.Name)
	}

	if variable.Default != "" {
		t.Errorf("Expected empty Default, got %q", variable.Default)
	}

	if variable.Values != nil {
		t.Errorf("Expected nil Values, got %v", variable.Values)
	}

	if variable.Min != 0 {
		t.Errorf("Expected Min 0, got %d", variable.Min)
	}

	if variable.Max != 0 {
		t.Errorf("Expected Max 0, got %d", variable.Max)
	}

	if variable.Sensitive {
		t.Error("Expected Sensitive to be false")
	}

	if variable.Type != "" {
		t.Errorf("Expected empty Type, got %q", variable.Type)
	}
}

func TestVariable_Types(t *testing.T) {
	testCases := []struct {
		name         string
		variable     Variable
		expectedType string
	}{
		{
			name: "string variable",
			variable: Variable{
				Name: "STRING_VAR",
				Type: VariableType("string"),
			},
			expectedType: "string",
		},
		{
			name: "number variable",
			variable: Variable{
				Name: "NUMBER_VAR",
				Type: VariableType("number"),
				Min:  1,
				Max:  10,
			},
			expectedType: "number",
		},
		{
			name: "boolean variable",
			variable: Variable{
				Name:    "BOOLEAN_VAR",
				Type:    VariableType("boolean"),
				Default: "false",
			},
			expectedType: "boolean",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.variable.Type.String() != tc.expectedType {
				t.Errorf("Expected Type %q, got %q", tc.expectedType, tc.variable.Type.String())
			}

			if !tc.variable.Type.IsValid() {
				t.Error("Expected Type to be valid")
			}

			// Test type-specific methods
			switch tc.expectedType {
			case "string":
				if !tc.variable.Type.IsString() {
					t.Error("Expected Type to be string")
				}
			case "number":
				if !tc.variable.Type.IsNumber() {
					t.Error("Expected Type to be number")
				}
			case "boolean":
				if !tc.variable.Type.IsBoolean() {
					t.Error("Expected Type to be boolean")
				}
			}
		})
	}
}

func TestVariable_SensitiveHandling(t *testing.T) {
	// Test sensitive variable
	sensitiveVar := Variable{
		Name:      "API_KEY",
		Default:   "secret_key",
		Sensitive: true,
		Type:      VariableType("string"),
	}

	if !sensitiveVar.Sensitive {
		t.Error("Expected variable to be sensitive")
	}

	// Test non-sensitive variable
	normalVar := Variable{
		Name:      "PUBLIC_URL",
		Default:   "https://example.com",
		Sensitive: false,
		Type:      VariableType("string"),
	}

	if normalVar.Sensitive {
		t.Error("Expected variable to not be sensitive")
	}
}

func TestVariable_ValueConstraints(t *testing.T) {
	// Test variable with value constraints
	variable := Variable{
		Name:   "ENUM_VAR",
		Values: []string{"option1", "option2", "option3"},
		Type:   VariableType("string"),
	}

	if len(variable.Values) != 3 {
		t.Errorf("Expected 3 allowed values, got %d", len(variable.Values))
	}

	expectedValues := []string{"option1", "option2", "option3"}
	for i, value := range variable.Values {
		if value != expectedValues[i] {
			t.Errorf("Expected value %d to be %q, got %q", i, expectedValues[i], value)
		}
	}
}

func TestVariable_NumericConstraints(t *testing.T) {
	// Test variable with numeric constraints
	variable := Variable{
		Name:    "PORT_NUMBER",
		Min:     1024,
		Max:     65535,
		Default: "8080",
		Type:    VariableType("number"),
	}

	if variable.Min != 1024 {
		t.Errorf("Expected Min 1024, got %d", variable.Min)
	}

	if variable.Max != 65535 {
		t.Errorf("Expected Max 65535, got %d", variable.Max)
	}

	if variable.Default != "8080" {
		t.Errorf("Expected Default '8080', got %q", variable.Default)
	}

	if !variable.Type.IsNumber() {
		t.Error("Expected Type to be number")
	}
}

func TestVariable_ComplexExamples(t *testing.T) {
	testCases := []struct {
		name     string
		variable Variable
	}{
		{
			name: "API configuration",
			variable: Variable{
				Name:      "API_ENDPOINT",
				Default:   "https://api.example.com",
				Type:      VariableType("string"),
				Sensitive: false,
			},
		},
		{
			name: "Database password",
			variable: Variable{
				Name:      "DB_PASSWORD",
				Default:   "",
				Type:      VariableType("string"),
				Sensitive: true,
			},
		},
		{
			name: "Port with range",
			variable: Variable{
				Name:    "HTTP_PORT",
				Default: "8080",
				Min:     1024,
				Max:     65535,
				Type:    VariableType("number"),
			},
		},
		{
			name: "Environment selection",
			variable: Variable{
				Name:    "ENVIRONMENT",
				Default: "development",
				Values:  []string{"development", "staging", "production"},
				Type:    VariableType("string"),
			},
		},
		{
			name: "Debug flag",
			variable: Variable{
				Name:    "DEBUG_MODE",
				Default: "false",
				Values:  []string{"true", "false"},
				Type:    VariableType("boolean"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify the variable is well-formed
			if tc.variable.Name == "" {
				t.Error("Variable name should not be empty")
			}

			if !tc.variable.Type.IsValid() {
				t.Errorf("Variable type %q should be valid", tc.variable.Type)
			}

			// Type-specific validations
			if tc.variable.Type.IsNumber() {
				if tc.variable.Min < 0 || tc.variable.Max < 0 {
					// Negative constraints might be valid in some cases, just check they're set
				}
				if tc.variable.Min > 0 && tc.variable.Max > 0 && tc.variable.Min > tc.variable.Max {
					t.Error("Min should not be greater than Max for number type")
				}
			}

			// If values are constrained, default should be valid (if not empty)
			if len(tc.variable.Values) > 0 && tc.variable.Default != "" {
				found := false
				for _, value := range tc.variable.Values {
					if value == tc.variable.Default {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Default value %q should be in allowed values %v", tc.variable.Default, tc.variable.Values)
				}
			}
		})
	}
}

func TestVariable_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		variable    Variable
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid variable",
			variable: Variable{
				Name:    "TEST_VAR",
				Default: "value1",
				Values:  []string{"value1", "value2", "value3"},
				Type:    VariableType("string"),
			},
			expectError: false,
		},
		{
			name: "empty name",
			variable: Variable{
				Name:    "",
				Default: "value",
				Type:    VariableType("string"),
			},
			expectError: true,
			errorMsg:    "variable name cannot be empty",
		},
		{
			name: "name too long",
			variable: Variable{
				Name:    string(make([]byte, MaxVariableNameLength+1)),
				Default: "value",
				Type:    VariableType("string"),
			},
			expectError: true,
			errorMsg:    "variable name exceeds max length",
		},
		{
			name: "min greater than max",
			variable: Variable{
				Name: "TEST_VAR",
				Min:  100,
				Max:  10,
				Type: VariableType("number"),
			},
			expectError: true,
			errorMsg:    "min (100) cannot be greater than max (10)",
		},
		{
			name: "default not in values",
			variable: Variable{
				Name:    "TEST_VAR",
				Default: "invalid",
				Values:  []string{"value1", "value2", "value3"},
				Type:    VariableType("string"),
			},
			expectError: true,
			errorMsg:    "default value 'invalid' not found in allowed values",
		},
		{
			name: "empty default with values is valid",
			variable: Variable{
				Name:    "TEST_VAR",
				Default: "",
				Values:  []string{"value1", "value2", "value3"},
				Type:    VariableType("string"),
			},
			expectError: false,
		},
		{
			name: "valid min max",
			variable: Variable{
				Name: "PORT",
				Min:  1024,
				Max:  65535,
				Type: VariableType("number"),
			},
			expectError: false,
		},
		{
			name: "zero max with positive min is valid",
			variable: Variable{
				Name: "TEST_VAR",
				Min:  10,
				Max:  0,
				Type: VariableType("number"),
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.variable.Validate()
			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tc.errorMsg != "" && !containsStr(err.Error(), tc.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
