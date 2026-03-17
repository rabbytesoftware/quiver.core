package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestArrow_Structure(t *testing.T) {
	// Create a test arrow with all fields populated
	testID := uuid.New()
	arrow := Arrow{
		ID:            testID,
		Namespace:     Namespace("test:namespace"),
		ArrowVersion:  []string{"1.0", "1.1", "2.0"},
		Name:          "Test Arrow",
		Description:   "A test arrow for unit testing",
		Version:       "1.0.0",
		License:       "MIT",
		Maintainers:   []string{"test@example.com"},
		Credits:       []string{"John Doe", "Jane Smith"},
		URL:           URL("https://example.com/arrow"),
		Documentation: "https://docs.example.com",
		Requirements: Requirement{
			OS:       OS("linux/amd64"),
			Memory:   1024,
			Disk:     512,
			CpuCores: 2,
		},
		Dependencies: []Namespace{Namespace("dep:namespace")},
		Netbridge: []PortRule{
			{
				StartPort: 8080,
				EndPort:   8080,
				Protocol:  Protocol("tcp"),
			},
		},
		Variables: []Variable{
			{
				Name:    "TEST_VAR",
				Type:    VariableType("string"),
				Default: "default_value",
			},
		},
		Methods: []Method{
			{
				Platforms:  []string{"linux/amd64"},
				MethodName: "install",
				Actions: []Action{
					{
						Type:  ActionTypeRun,
						Value: "echo hello",
					},
				},
			},
		},
	}

	// Test basic fields
	if arrow.ID != testID {
		t.Errorf("Expected ID %v, got %v", testID, arrow.ID)
	}

	if arrow.Namespace != Namespace("test:namespace") {
		t.Errorf("Expected Namespace 'test.namespace', got %q", arrow.Namespace)
	}

	if arrow.Name != "Test Arrow" {
		t.Errorf("Expected Name 'Test Arrow', got %q", arrow.Name)
	}

	if arrow.Description != "A test arrow for unit testing" {
		t.Errorf("Expected Description 'A test arrow for unit testing', got %q", arrow.Description)
	}

	if arrow.Version != "1.0.0" {
		t.Errorf("Expected Version '1.0.0', got %q", arrow.Version)
	}

	if arrow.License != "MIT" {
		t.Errorf("Expected License 'MIT', got %q", arrow.License)
	}

	if arrow.Documentation != "https://docs.example.com" {
		t.Errorf("Expected Documentation 'https://docs.example.com', got %q", arrow.Documentation)
	}

	// Test array fields
	if len(arrow.ArrowVersion) != 3 {
		t.Errorf("Expected 3 arrow versions, got %d", len(arrow.ArrowVersion))
	}

	if len(arrow.Maintainers) != 1 || arrow.Maintainers[0] != "test@example.com" {
		t.Errorf("Expected Maintainers ['test@example.com'], got %v", arrow.Maintainers)
	}

	if len(arrow.Credits) != 2 {
		t.Errorf("Expected 2 credits, got %d", len(arrow.Credits))
	}

	if len(arrow.Dependencies) != 1 {
		t.Errorf("Expected 1 dependency, got %d", len(arrow.Dependencies))
	}

	if len(arrow.Netbridge) != 1 {
		t.Errorf("Expected 1 netbridge rule, got %d", len(arrow.Netbridge))
	}

	if len(arrow.Variables) != 1 {
		t.Errorf("Expected 1 variable, got %d", len(arrow.Variables))
	}

	if len(arrow.Methods) != 1 {
		t.Errorf("Expected 1 method, got %d", len(arrow.Methods))
	}
}

func TestArrow_EmptyArrow(t *testing.T) {
	// Test empty arrow
	arrow := Arrow{}

	if arrow.ID != uuid.Nil {
		t.Errorf("Expected nil UUID, got %v", arrow.ID)
	}

	if arrow.Namespace != "" {
		t.Errorf("Expected empty Namespace, got %q", arrow.Namespace)
	}

	if arrow.Name != "" {
		t.Errorf("Expected empty Name, got %q", arrow.Name)
	}

	if arrow.ArrowVersion != nil {
		t.Errorf("Expected nil ArrowVersion, got %v", arrow.ArrowVersion)
	}

	if arrow.Maintainers != nil {
		t.Errorf("Expected nil Maintainers, got %v", arrow.Maintainers)
	}

	if arrow.Dependencies != nil {
		t.Errorf("Expected nil Dependencies, got %v", arrow.Dependencies)
	}

	if arrow.Netbridge != nil {
		t.Errorf("Expected nil Netbridge, got %v", arrow.Netbridge)
	}

	if arrow.Variables != nil {
		t.Errorf("Expected nil Variables, got %v", arrow.Variables)
	}

	if arrow.Methods != nil {
		t.Errorf("Expected nil Methods, got %v", arrow.Methods)
	}
}

func TestArrow_SystemTypes(t *testing.T) {
	// Test system type fields
	arrow := Arrow{
		URL: URL("https://example.com/arrow"),
		Requirements: Requirement{
			OS:          OS("linux/amd64"),
			Memory:      1024,
			Disk:        512,
			CpuCores:    2,
			NetworkMbps: 100,
		},
	}

	// Test URL
	if !arrow.URL.IsValid() {
		t.Error("Expected URL to be valid")
	}

	// Test Requirements
	if !arrow.Requirements.IsValid() {
		t.Error("Expected Requirements to be valid")
	}

	// Test that OS requirement is set
	if arrow.Requirements.OS == "" {
		t.Error("Expected OS requirement to be set")
	}

	if !arrow.Requirements.OS.IsLinux() {
		t.Error("Expected OS requirement to be Linux")
	}
}

func TestArrow_PortRules(t *testing.T) {
	// Test port rules
	arrow := Arrow{
		Netbridge: []PortRule{
			{
				StartPort: 80,
				EndPort:   80,
				Protocol:  Protocol("tcp"),
			},
			{
				StartPort: 443,
				EndPort:   443,
				Protocol:  Protocol("tcp"),
			},
			{
				StartPort: 8000,
				EndPort:   8999,
				Protocol:  Protocol("tcp"),
			},
		},
	}

	if len(arrow.Netbridge) != 3 {
		t.Errorf("Expected 3 netbridge rules, got %d", len(arrow.Netbridge))
	}

	// Test first rule
	if !arrow.Netbridge[0].IsStartPortValid() {
		t.Error("Expected first rule start port to be valid")
	}

	if !arrow.Netbridge[0].IsEndPortValid() {
		t.Error("Expected first rule end port to be valid")
	}

	if !arrow.Netbridge[0].Protocol.IsTCP() {
		t.Error("Expected first rule protocol to be TCP")
	}

	// Test port range rule
	if arrow.Netbridge[2].StartPort != 8000 {
		t.Errorf("Expected third rule start port 8000, got %d", arrow.Netbridge[2].StartPort)
	}

	if arrow.Netbridge[2].EndPort != 8999 {
		t.Errorf("Expected third rule end port 8999, got %d", arrow.Netbridge[2].EndPort)
	}
}

func TestArrow_Variables(t *testing.T) {
	// Test variables
	arrow := Arrow{
		Variables: []Variable{
			{
				Name:    "API_KEY",
				Type:    VariableType("string"),
				Default: "",
			},
			{
				Name:    "PORT",
				Type:    VariableType("number"),
				Default: "8080",
			},
			{
				Name:    "DEBUG",
				Type:    VariableType("boolean"),
				Default: "false",
			},
		},
	}

	if len(arrow.Variables) != 3 {
		t.Errorf("Expected 3 variables, got %d", len(arrow.Variables))
	}

	// Test string variable
	if !arrow.Variables[0].Type.IsString() {
		t.Error("Expected first variable to be string type")
	}

	// Test number variable
	if !arrow.Variables[1].Type.IsNumber() {
		t.Error("Expected second variable to be number type")
	}

	// Test boolean variable
	if !arrow.Variables[2].Type.IsBoolean() {
		t.Error("Expected third variable to be boolean type")
	}
}

func TestArrow_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		arrow       Arrow
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid arrow",
			arrow: Arrow{
				Name:    "test-arrow",
				Version: "1.0.0",
				Requirements: Requirement{
					CpuCores:    1,
					Memory:      256,
					Disk:        100,
					NetworkMbps: 10,
					OS:          OSLinuxAMD64,
				},
			},
			expectError: false,
		},
		{
			name: "empty name",
			arrow: Arrow{
				Name:    "",
				Version: "1.0.0",
			},
			expectError: true,
			errorMsg:    "arrow name cannot be empty",
		},
		{
			name: "name too long",
			arrow: Arrow{
				Name:    string(make([]byte, MaxNameLength+1)),
				Version: "1.0.0",
			},
			expectError: true,
			errorMsg:    "arrow name exceeds max length",
		},
		{
			name: "empty version",
			arrow: Arrow{
				Name:    "test-arrow",
				Version: "",
			},
			expectError: true,
			errorMsg:    "arrow version cannot be empty",
		},
		{
			name: "description too long",
			arrow: Arrow{
				Name:        "test-arrow",
				Version:     "1.0.0",
				Description: string(make([]byte, MaxDescriptionLength+1)),
			},
			expectError: true,
			errorMsg:    "description exceeds max length",
		},
		{
			name: "invalid requirements",
			arrow: Arrow{
				Name:    "test-arrow",
				Version: "1.0.0",
				Requirements: Requirement{
					CpuCores: -1,
				},
			},
			expectError: true,
			errorMsg:    "requirements validation failed",
		},
		{
			name: "invalid icon URL",
			arrow: Arrow{
				Name:    "test-arrow",
				Version: "1.0.0",
				IconURL: URL("not a valid url"),
				Requirements: Requirement{
					CpuCores:    1,
					Memory:      256,
					Disk:        100,
					NetworkMbps: 10,
					OS:          OSLinuxAMD64,
				},
			},
			expectError: true,
			errorMsg:    "invalid icon URL",
		},
		{
			name: "invalid banner URL",
			arrow: Arrow{
				Name:      "test-arrow",
				Version:   "1.0.0",
				BannerURL: URL("not a valid url"),
				Requirements: Requirement{
					CpuCores:    1,
					Memory:      256,
					Disk:        100,
					NetworkMbps: 10,
					OS:          OSLinuxAMD64,
				},
			},
			expectError: true,
			errorMsg:    "invalid banner URL",
		},
		{
			name: "invalid namespace",
			arrow: Arrow{
				Name:      "test-arrow",
				Version:   "1.0.0",
				Namespace: Namespace("invalid"),
				Requirements: Requirement{
					CpuCores:    1,
					Memory:      256,
					Disk:        100,
					NetworkMbps: 10,
					OS:          OSLinuxAMD64,
				},
			},
			expectError: true,
			errorMsg:    "namespace validation failed",
		},
		{
			name: "invalid variable",
			arrow: Arrow{
				Name:    "test-arrow",
				Version: "1.0.0",
				Variables: []Variable{
					{Name: ""},
				},
				Requirements: Requirement{
					CpuCores:    1,
					Memory:      256,
					Disk:        100,
					NetworkMbps: 10,
					OS:          OSLinuxAMD64,
				},
			},
			expectError: true,
			errorMsg:    "variable[0] validation failed",
		},
		{
			name: "invalid method",
			arrow: Arrow{
				Name:    "test-arrow",
				Version: "1.0.0",
				Methods: []Method{
					{MethodName: ""},
				},
				Requirements: Requirement{
					CpuCores:    1,
					Memory:      256,
					Disk:        100,
					NetworkMbps: 10,
					OS:          OSLinuxAMD64,
				},
			},
			expectError: true,
			errorMsg:    "method[0] validation failed",
		},
		{
			name: "invalid netbridge port",
			arrow: Arrow{
				Name:    "test-arrow",
				Version: "1.0.0",
				Netbridge: []PortRule{
					{ID: "", StartPort: 8080, EndPort: 8080, Protocol: ProtocolTCP},
				},
				Requirements: Requirement{
					CpuCores:    1,
					Memory:      256,
					Disk:        100,
					NetworkMbps: 10,
					OS:          OSLinuxAMD64,
				},
			},
			expectError: true,
			errorMsg:    "port[0] validation failed",
		},
		{
			name: "valid with all fields",
			arrow: Arrow{
				Name:          "test-arrow",
				Version:       "1.0.0",
				Description:   "A test arrow",
				IconURL:       URL("https://example.com/icon.png"),
				BannerURL:     URL("https://example.com/banner.png"),
				Documentation: "https://docs.example.com",
				Variables: []Variable{
					{
						Name:    "TEST_VAR",
						Type:    VariableType("string"),
						Default: "value",
					},
				},
				Requirements: Requirement{
					CpuCores:    1,
					Memory:      256,
					Disk:        100,
					NetworkMbps: 10,
					OS:          OSLinuxAMD64,
				},
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.arrow.Validate()
			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tc.errorMsg != "" && !containsSubstring(err.Error(), tc.errorMsg) {
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

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
