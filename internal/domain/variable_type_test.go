package domain

import "testing"

func TestVariableType_String(t *testing.T) {
	testCases := []struct {
		name     string
		varType  VariableType
		expected string
	}{
		{
			name:     "string type",
			varType:  VariableTypeString,
			expected: "string",
		},
		{
			name:     "number type",
			varType:  VariableTypeNumber,
			expected: "number",
		},
		{
			name:     "boolean type",
			varType:  VariableTypeBoolean,
			expected: "boolean",
		},
		{
			name:     "custom type",
			varType:  VariableType("custom"),
			expected: "custom",
		},
		{
			name:     "empty type",
			varType:  VariableType(""),
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.varType.String()
			if result != tc.expected {
				t.Errorf("Expected String() to return %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestVariableType_IsValid(t *testing.T) {
	testCases := []struct {
		name     string
		varType  VariableType
		expected bool
	}{
		{
			name:     "string type",
			varType:  VariableTypeString,
			expected: true,
		},
		{
			name:     "number type",
			varType:  VariableTypeNumber,
			expected: true,
		},
		{
			name:     "boolean type",
			varType:  VariableTypeBoolean,
			expected: true,
		},
		{
			name:     "select type",
			varType:  VariableTypeSelect,
			expected: true,
		},
		{
			name:     "custom type",
			varType:  VariableType("custom"),
			expected: false,
		},
		{
			name:     "empty type",
			varType:  VariableType(""),
			expected: false,
		},
		{
			name:     "uppercase string",
			varType:  VariableType("STRING"),
			expected: false,
		},
		{
			name:     "mixed case",
			varType:  VariableType("String"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.varType.IsValid()
			if result != tc.expected {
				t.Errorf("Expected IsValid() to return %v for %q, got %v", tc.expected, tc.varType, result)
			}
		})
	}
}

func TestVariableType_IsString(t *testing.T) {
	testCases := []struct {
		name     string
		varType  VariableType
		expected bool
	}{
		{
			name:     "string type",
			varType:  VariableTypeString,
			expected: true,
		},
		{
			name:     "number type",
			varType:  VariableTypeNumber,
			expected: false,
		},
		{
			name:     "boolean type",
			varType:  VariableTypeBoolean,
			expected: false,
		},
		{
			name:     "custom type",
			varType:  VariableType("custom"),
			expected: false,
		},
		{
			name:     "empty type",
			varType:  VariableType(""),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.varType.IsString()
			if result != tc.expected {
				t.Errorf("Expected IsString() to return %v for %q, got %v", tc.expected, tc.varType, result)
			}
		})
	}
}

func TestVariableType_IsNumber(t *testing.T) {
	testCases := []struct {
		name     string
		varType  VariableType
		expected bool
	}{
		{
			name:     "string type",
			varType:  VariableTypeString,
			expected: false,
		},
		{
			name:     "number type",
			varType:  VariableTypeNumber,
			expected: true,
		},
		{
			name:     "boolean type",
			varType:  VariableTypeBoolean,
			expected: false,
		},
		{
			name:     "custom type",
			varType:  VariableType("custom"),
			expected: false,
		},
		{
			name:     "empty type",
			varType:  VariableType(""),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.varType.IsNumber()
			if result != tc.expected {
				t.Errorf("Expected IsNumber() to return %v for %q, got %v", tc.expected, tc.varType, result)
			}
		})
	}
}

func TestVariableType_IsBoolean(t *testing.T) {
	testCases := []struct {
		name     string
		varType  VariableType
		expected bool
	}{
		{
			name:     "string type",
			varType:  VariableTypeString,
			expected: false,
		},
		{
			name:     "number type",
			varType:  VariableTypeNumber,
			expected: false,
		},
		{
			name:     "boolean type",
			varType:  VariableTypeBoolean,
			expected: true,
		},
		{
			name:     "custom type",
			varType:  VariableType("custom"),
			expected: false,
		},
		{
			name:     "empty type",
			varType:  VariableType(""),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.varType.IsBoolean()
			if result != tc.expected {
				t.Errorf("Expected IsBoolean() to return %v for %q, got %v", tc.expected, tc.varType, result)
			}
		})
	}
}

func TestVariableType_Constants(t *testing.T) {
	// Test that constants have expected values
	if VariableTypeString != "string" {
		t.Errorf("Expected VariableTypeString to be 'string', got %q", VariableTypeString)
	}

	if VariableTypeNumber != "number" {
		t.Errorf("Expected VariableTypeNumber to be 'number', got %q", VariableTypeNumber)
	}

	if VariableTypeBoolean != "boolean" {
		t.Errorf("Expected VariableTypeBoolean to be 'boolean', got %q", VariableTypeBoolean)
	}

	if VariableTypeSelect != "select" {
		t.Errorf("Expected VariableTypeSelect to be 'select', got %q", VariableTypeSelect)
	}
}

func TestVariableType_AllMethods(t *testing.T) {
	// Test all methods on each constant
	varTypes := []VariableType{
		VariableTypeString,
		VariableTypeNumber,
		VariableTypeBoolean,
		VariableTypeSelect,
	}

	for _, varType := range varTypes {
		// All defined constants should be valid
		if !varType.IsValid() {
			t.Errorf("Expected variable type %q to be valid", varType)
		}

		// Each type should have exactly one specific method return true
		trueCount := 0
		if varType.IsString() {
			trueCount++
		}
		if varType.IsNumber() {
			trueCount++
		}
		if varType.IsBoolean() {
			trueCount++
		}
		if varType.IsSelect() {
			trueCount++
		}

		if trueCount != 1 {
			t.Errorf("Expected exactly one specific method to return true for type %q, got %d", varType, trueCount)
		}

		// String method should return the expected value
		if varType.String() != string(varType) {
			t.Errorf("Expected String() to return %q, got %q", string(varType), varType.String())
		}
	}
}

func TestVariableType_PointerMethods(t *testing.T) {
	// Test that pointer methods work correctly
	varType := VariableTypeString

	if !varType.IsValid() {
		t.Error("Expected VariableTypeString to be valid")
	}

	if !varType.IsString() {
		t.Error("Expected VariableTypeString.IsString() to return true")
	}

	if varType.IsNumber() {
		t.Error("Expected VariableTypeString.IsNumber() to return false")
	}

	if varType.IsBoolean() {
		t.Error("Expected VariableTypeString.IsBoolean() to return false")
	}
}

func TestVariableType_InvalidTypes(t *testing.T) {
	invalidTypes := []VariableType{
		VariableType("int"),
		VariableType("float"),
		VariableType("array"),
		VariableType("object"),
		VariableType("null"),
		VariableType("undefined"),
	}

	for _, invalidType := range invalidTypes {
		if invalidType.IsValid() {
			t.Errorf("Expected type %q to be invalid", invalidType)
		}

		// All specific type methods should return false for invalid types
		if invalidType.IsString() {
			t.Errorf("Expected IsString() to return false for invalid type %q", invalidType)
		}

		if invalidType.IsNumber() {
			t.Errorf("Expected IsNumber() to return false for invalid type %q", invalidType)
		}

		if invalidType.IsBoolean() {
			t.Errorf("Expected IsBoolean() to return false for invalid type %q", invalidType)
		}
	}
}
