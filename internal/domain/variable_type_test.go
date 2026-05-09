package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
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
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestVariableType_Constants(t *testing.T) {
	assert.Equal(t, VariableType("string"), VariableTypeString)
	assert.Equal(t, VariableType("number"), VariableTypeNumber)
	assert.Equal(t, VariableType("boolean"), VariableTypeBoolean)
	assert.Equal(t, VariableType("select"), VariableTypeSelect)
}

func TestVariableType_AllMethods(t *testing.T) {
	varTypes := []VariableType{
		VariableTypeString,
		VariableTypeNumber,
		VariableTypeBoolean,
		VariableTypeSelect,
	}

	for _, varType := range varTypes {
		assert.True(t, varType.IsValid(), "Expected variable type %q to be valid", varType)

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
		assert.Equal(t, 1, trueCount, "Expected exactly one specific method to return true for type %q", varType)

		assert.Equal(t, string(varType), varType.String())
	}
}

func TestVariableType_PointerMethods(t *testing.T) {
	varType := VariableTypeString

	assert.True(t, varType.IsValid())
	assert.True(t, varType.IsString())
	assert.False(t, varType.IsNumber())
	assert.False(t, varType.IsBoolean())
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
		assert.False(t, invalidType.IsValid(), "Expected type %q to be invalid", invalidType)
		assert.False(t, invalidType.IsString(), "Expected IsString() to return false for invalid type %q", invalidType)
		assert.False(t, invalidType.IsNumber(), "Expected IsNumber() to return false for invalid type %q", invalidType)
		assert.False(t, invalidType.IsBoolean(), "Expected IsBoolean() to return false for invalid type %q", invalidType)
	}
}
