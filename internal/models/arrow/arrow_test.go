package arrow

import (
	"testing"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/models/networking"
	"github.com/rabbytesoftware/quiver/internal/models/requirement"
	"github.com/rabbytesoftware/quiver/internal/models/runtime"
	"github.com/rabbytesoftware/quiver/internal/models/system"
	"github.com/rabbytesoftware/quiver/internal/models/variable"
)

func TestArrow_Structure(t *testing.T) {
	// Create a test arrow with all fields populated
	testID := uuid.New()
	arrow := Arrow{
		ID:            testID,
		Namespace:     ArrowNamespace("test.namespace"),
		ArrowVersion:  []string{"1.0", "1.1", "2.0"},
		Name:          "Test Arrow",
		Description:   "A test arrow for unit testing",
		Version:       "1.0.0",
		License:       "MIT",
		Maintainers:   []string{"test@example.com"},
		Credits:       []string{"John Doe", "Jane Smith"},
		URL:           system.URL("https://example.com/arrow"),
		Documentation: "https://docs.example.com",
		Requirements: requirement.Requirement{
			OS:       system.OS("linux/amd64"),
			Memory:   1024,
			Disk:     512,
			CpuCores: 2,
		},
		Dependencies: []ArrowNamespace{ArrowNamespace("dep.namespace")},
		Netbridge: []networking.Port{
			{
				StartPort: 8080,
				EndPort:   8080,
				Protocol:  networking.Protocol("tcp"),
			},
		},
		Variables: []variable.Variable{
			{
				Name:    "TEST_VAR",
				Type:    variable.VariableType("string"),
				Default: "default_value",
			},
		},
		Methods: []runtime.Method{
			{
				OS:      system.OS("linux/amd64"),
				Command: []string{"echo", "hello"},
			},
		},
	}

	// Test basic fields
	if arrow.ID != testID {
		t.Errorf("Expected ID %v, got %v", testID, arrow.ID)
	}

	if arrow.Namespace != ArrowNamespace("test.namespace") {
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
		URL: system.URL("https://example.com/arrow"),
		Requirements: requirement.Requirement{
			OS:       system.OS("linux/amd64"),
			Memory:   1024,
			Disk:     512,
			CpuCores: 2,
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
		Netbridge: []networking.Port{
			{
				StartPort: 80,
				EndPort:   80,
				Protocol:  networking.Protocol("tcp"),
			},
			{
				StartPort: 443,
				EndPort:   443,
				Protocol:  networking.Protocol("tcp"),
			},
			{
				StartPort: 8000,
				EndPort:   8999,
				Protocol:  networking.Protocol("tcp"),
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
		Variables: []variable.Variable{
			{
				Name:    "API_KEY",
				Type:    variable.VariableType("string"),
				Default: "",
			},
			{
				Name:    "PORT",
				Type:    variable.VariableType("number"),
				Default: "8080",
			},
			{
				Name:    "DEBUG",
				Type:    variable.VariableType("boolean"),
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

func TestArrow_RuntimeMethods(t *testing.T) {
	// Test runtime methods
	arrow := Arrow{
		Methods: []runtime.Method{
			{
				OS:      system.OS("linux/amd64"),
				Command: []string{"./start.sh"},
			},
			{
				OS:      system.OS("windows/amd64"),
				Command: []string{"start.bat"},
			},
			{
				OS:      system.OS("darwin/arm64"),
				Command: []string{"./start.sh"},
			},
		},
	}

	if len(arrow.Methods) != 3 {
		t.Errorf("Expected 3 methods, got %d", len(arrow.Methods))
	}

	// Test Linux method
	if !arrow.Methods[0].OS.IsLinux() {
		t.Error("Expected first method OS to be Linux")
	}

	// Test Windows method
	if !arrow.Methods[1].OS.IsWindows() {
		t.Error("Expected second method OS to be Windows")
	}

	// Test Darwin method
	if !arrow.Methods[2].OS.IsDarwin() {
		t.Error("Expected third method OS to be Darwin")
	}

	if !arrow.Methods[2].OS.IsARM64() {
		t.Error("Expected third method OS to be ARM64")
	}
}
