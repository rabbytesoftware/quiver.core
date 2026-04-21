package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVariable_Structure(t *testing.T) {
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

	assert.Equal(t, "TEST_VAR", variable.Name)
	assert.Equal(t, "A test variable", variable.Description)
	assert.Equal(t, "default_value", variable.Default)
	assert.Len(t, variable.Values, 3)
	assert.Equal(t, "value1", variable.Values[0])
	assert.Equal(t, 1, variable.Min)
	assert.Equal(t, 100, variable.Max)
	assert.True(t, variable.Sensitive, "Expected Sensitive to be true")
	assert.True(t, variable.Type.IsString(), "Expected Type to be string")
}

func TestVariable_EmptyVariable(t *testing.T) {
	variable := Variable{}

	assert.Equal(t, "", variable.Name)
	assert.Equal(t, "", variable.Default)
	assert.Nil(t, variable.Values)
	assert.Equal(t, 0, variable.Min)
	assert.Equal(t, 0, variable.Max)
	assert.False(t, variable.Sensitive, "Expected Sensitive to be false")
	assert.Equal(t, VariableType(""), variable.Type)
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
			assert.Equal(t, tc.expectedType, tc.variable.Type.String())
			assert.True(t, tc.variable.Type.IsValid(), "Expected Type to be valid")

			switch tc.expectedType {
			case "string":
				assert.True(t, tc.variable.Type.IsString(), "Expected Type to be string")
			case "number":
				assert.True(t, tc.variable.Type.IsNumber(), "Expected Type to be number")
			case "boolean":
				assert.True(t, tc.variable.Type.IsBoolean(), "Expected Type to be boolean")
			}
		})
	}
}

func TestVariable_SensitiveHandling(t *testing.T) {
	sensitiveVar := Variable{
		Name:      "API_KEY",
		Default:   "secret_key",
		Sensitive: true,
		Type:      VariableType("string"),
	}
	assert.True(t, sensitiveVar.Sensitive, "Expected variable to be sensitive")

	normalVar := Variable{
		Name:      "PUBLIC_URL",
		Default:   "https://example.com",
		Sensitive: false,
		Type:      VariableType("string"),
	}
	assert.False(t, normalVar.Sensitive, "Expected variable to not be sensitive")
}

func TestVariable_ValueConstraints(t *testing.T) {
	variable := Variable{
		Name:   "ENUM_VAR",
		Values: []string{"option1", "option2", "option3"},
		Type:   VariableType("string"),
	}

	require.Len(t, variable.Values, 3)

	expectedValues := []string{"option1", "option2", "option3"}
	for i, value := range variable.Values {
		assert.Equal(t, expectedValues[i], value)
	}
}

func TestVariable_NumericConstraints(t *testing.T) {
	variable := Variable{
		Name:    "PORT_NUMBER",
		Min:     1024,
		Max:     65535,
		Default: "8080",
		Type:    VariableType("number"),
	}

	assert.Equal(t, 1024, variable.Min)
	assert.Equal(t, 65535, variable.Max)
	assert.Equal(t, "8080", variable.Default)
	assert.True(t, variable.Type.IsNumber(), "Expected Type to be number")
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
			assert.NotEmpty(t, tc.variable.Name, "Variable name should not be empty")
			assert.True(t, tc.variable.Type.IsValid(), "Variable type %q should be valid", tc.variable.Type)

			if tc.variable.Type.IsNumber() {
				if tc.variable.Min > 0 && tc.variable.Max > 0 && tc.variable.Min > tc.variable.Max {
					assert.Fail(t, "Min should not be greater than Max for number type")
				}
			}

			if len(tc.variable.Values) > 0 && tc.variable.Default != "" {
				found := false
				for _, value := range tc.variable.Values {
					if value == tc.variable.Default {
						found = true
						break
					}
				}
				assert.True(t, found, "Default value %q should be in allowed values %v", tc.variable.Default, tc.variable.Values)
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
				require.Error(t, err)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
