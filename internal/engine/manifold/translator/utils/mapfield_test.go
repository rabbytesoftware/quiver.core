package utils

import (
	"reflect"
	"testing"
)

func TestGetSliceField(t *testing.T) {
	testCases := []struct {
		name     string
		m        map[string]interface{}
		key      string
		expected []interface{}
	}{
		{
			name: "valid slice field",
			m: map[string]interface{}{
				"items": []interface{}{"a", "b", "c"},
			},
			key:      "items",
			expected: []interface{}{"a", "b", "c"},
		},
		{
			name: "empty slice",
			m: map[string]interface{}{
				"items": []interface{}{},
			},
			key:      "items",
			expected: []interface{}{},
		},
		{
			name:     "non-existent key",
			m:        map[string]interface{}{},
			key:      "missing",
			expected: []interface{}{},
		},
		{
			name: "wrong type - string",
			m: map[string]interface{}{
				"items": "not a slice",
			},
			key:      "items",
			expected: []interface{}{},
		},
		{
			name: "wrong type - int",
			m: map[string]interface{}{
				"items": 123,
			},
			key:      "items",
			expected: []interface{}{},
		},
		{
			name: "slice with mixed types",
			m: map[string]interface{}{
				"items": []interface{}{1, "two", 3.0, true},
			},
			key:      "items",
			expected: []interface{}{1, "two", 3.0, true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetSliceField(tc.m, tc.key)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestGetStringField(t *testing.T) {
	testCases := []struct {
		name     string
		m        map[string]interface{}
		key      string
		expected string
	}{
		{
			name: "valid string field",
			m: map[string]interface{}{
				"name": "test",
			},
			key:      "name",
			expected: "test",
		},
		{
			name: "empty string",
			m: map[string]interface{}{
				"name": "",
			},
			key:      "name",
			expected: "",
		},
		{
			name:     "non-existent key",
			m:        map[string]interface{}{},
			key:      "missing",
			expected: "",
		},
		{
			name: "wrong type - int",
			m: map[string]interface{}{
				"name": 123,
			},
			key:      "name",
			expected: "",
		},
		{
			name: "wrong type - slice",
			m: map[string]interface{}{
				"name": []interface{}{"a", "b"},
			},
			key:      "name",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetStringField(tc.m, tc.key)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestGetIntField(t *testing.T) {
	testCases := []struct {
		name     string
		m        map[string]interface{}
		key      string
		expected int
	}{
		{
			name: "valid int field",
			m: map[string]interface{}{
				"count": 42,
			},
			key:      "count",
			expected: 42,
		},
		{
			name: "float64 field converted to int",
			m: map[string]interface{}{
				"count": 42.0,
			},
			key:      "count",
			expected: 42,
		},
		{
			name: "float64 with decimals truncated",
			m: map[string]interface{}{
				"count": 42.9,
			},
			key:      "count",
			expected: 42,
		},
		{
			name: "zero value",
			m: map[string]interface{}{
				"count": 0,
			},
			key:      "count",
			expected: 0,
		},
		{
			name:     "non-existent key",
			m:        map[string]interface{}{},
			key:      "missing",
			expected: 0,
		},
		{
			name: "wrong type - string",
			m: map[string]interface{}{
				"count": "123",
			},
			key:      "count",
			expected: 0,
		},
		{
			name: "negative int",
			m: map[string]interface{}{
				"count": -10,
			},
			key:      "count",
			expected: -10,
		},
		{
			name: "negative float64",
			m: map[string]interface{}{
				"count": -10.5,
			},
			key:      "count",
			expected: -10,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetIntField(tc.m, tc.key)
			if result != tc.expected {
				t.Errorf("Expected %d, got %d", tc.expected, result)
			}
		})
	}
}

func TestGetBoolField(t *testing.T) {
	testCases := []struct {
		name     string
		m        map[string]interface{}
		key      string
		expected bool
	}{
		{
			name: "true value",
			m: map[string]interface{}{
				"enabled": true,
			},
			key:      "enabled",
			expected: true,
		},
		{
			name: "false value",
			m: map[string]interface{}{
				"enabled": false,
			},
			key:      "enabled",
			expected: false,
		},
		{
			name:     "non-existent key",
			m:        map[string]interface{}{},
			key:      "missing",
			expected: false,
		},
		{
			name: "wrong type - string",
			m: map[string]interface{}{
				"enabled": "true",
			},
			key:      "enabled",
			expected: false,
		},
		{
			name: "wrong type - int",
			m: map[string]interface{}{
				"enabled": 1,
			},
			key:      "enabled",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetBoolField(tc.m, tc.key)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestToStringSlice(t *testing.T) {
	testCases := []struct {
		name     string
		data     []interface{}
		expected []string
	}{
		{
			name:     "all strings",
			data:     []interface{}{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty slice",
			data:     []interface{}{},
			expected: []string{},
		},
		{
			name:     "mixed types - filters non-strings",
			data:     []interface{}{"a", 123, "b", true, "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "no strings",
			data:     []interface{}{123, true, 45.6},
			expected: []string{},
		},
		{
			name:     "empty strings included",
			data:     []interface{}{"a", "", "b"},
			expected: []string{"a", "", "b"},
		},
		{
			name:     "single string",
			data:     []interface{}{"single"},
			expected: []string{"single"},
		},
		{
			name:     "strings with spaces",
			data:     []interface{}{"hello world", "test  value"},
			expected: []string{"hello world", "test  value"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ToStringSlice(tc.data)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}
