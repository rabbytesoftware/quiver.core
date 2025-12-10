package requirements

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/shared"
)

func TestNewRequirements(t *testing.T) {
	req := NewRequirements()
	if req == nil {
		t.Fatal("NewRequirements() returned nil")
	}
}

func TestRequirements_Validate(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	testReq := &arrow.Requirement{}
	valid, err := req.Validate(ctx, testReq)
	if err != nil {
		t.Errorf("Validate() returned error: %v", err)
	}
	if valid {
		t.Error("Validate() should return false for unimplemented method")
	}
}

func TestRequirements_ValidateOS(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	valid, err := req.ValidateOS(ctx, shared.OSLinuxAMD64)
	if err != nil {
		t.Errorf("ValidateOS() returned error: %v", err)
	}
	if valid {
		t.Error("ValidateOS() should return false for unimplemented method")
	}
}

func TestRequirements_ValidateOSVersion(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	valid, err := req.ValidateOSVersion(ctx, "1.0.0")
	if err != nil {
		t.Errorf("ValidateOSVersion() returned error: %v", err)
	}
	if valid {
		t.Error("ValidateOSVersion() should return false for unimplemented method")
	}
}

func TestRequirements_ValidateArch(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	valid, err := req.ValidateArch(ctx, "amd64")
	if err != nil {
		t.Errorf("ValidateArch() returned error: %v", err)
	}
	if valid {
		t.Error("ValidateArch() should return false for unimplemented method")
	}
}

func TestRequirements_ValidateCPU(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	valid, err := req.ValidateCPU(ctx, 4)
	if err != nil {
		t.Errorf("ValidateCPU() returned error: %v", err)
	}
	if valid {
		t.Error("ValidateCPU() should return false for unimplemented method")
	}
}

func TestRequirements_ValidateMemory(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	valid, err := req.ValidateMemory(ctx, 8192)
	if err != nil {
		t.Errorf("ValidateMemory() returned error: %v", err)
	}
	if valid {
		t.Error("ValidateMemory() should return false for unimplemented method")
	}
}

func TestRequirements_ValidateDisk(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	valid, err := req.ValidateDisk(ctx, 1000000)
	if err != nil {
		t.Errorf("ValidateDisk() returned error: %v", err)
	}
	if valid {
		t.Error("ValidateDisk() should return false for unimplemented method")
	}
}

func TestRequirements_ValidateNetwork(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	valid, err := req.ValidateNetwork(ctx, 1000)
	if err != nil {
		t.Errorf("ValidateNetwork() returned error: %v", err)
	}
	if valid {
		t.Error("ValidateNetwork() should return false for unimplemented method")
	}
}

func TestRequirements_InterfaceCompliance(t *testing.T) {
	// Test that Requirements implements SRVInterface
	var _ SRVInterface = &Requirements{}
}

func TestRequirements_MultipleInstances(t *testing.T) {
	req1 := NewRequirements()
	req2 := NewRequirements()

	// Both should be valid
	if req1 == nil || req2 == nil {
		t.Error("NewRequirements() returned nil instance")
	}

	// Test that both instances work correctly
	ctx := context.Background()
	valid1, _ := req1.Validate(ctx, &arrow.Requirement{})
	valid2, _ := req2.Validate(ctx, &arrow.Requirement{})

	if valid1 != valid2 {
		t.Error("Both instances should have same Validate behavior")
	}
}

func TestRequirements_AllValidationMethods(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	// Test all validation methods with various inputs
	testCases := []struct {
		name string
		fn   func() (bool, error)
	}{
		{"Validate", func() (bool, error) {
			return req.Validate(ctx, &arrow.Requirement{})
		}},
		{"ValidateOS", func() (bool, error) {
			return req.ValidateOS(ctx, shared.OSLinuxAMD64)
		}},
		{"ValidateOSVersion", func() (bool, error) {
			return req.ValidateOSVersion(ctx, "1.0.0")
		}},
		{"ValidateArch", func() (bool, error) {
			return req.ValidateArch(ctx, "amd64")
		}},
		{"ValidateCPU", func() (bool, error) {
			return req.ValidateCPU(ctx, 4)
		}},
		{"ValidateMemory", func() (bool, error) {
			return req.ValidateMemory(ctx, 8192)
		}},
		{"ValidateDisk", func() (bool, error) {
			return req.ValidateDisk(ctx, 1000000)
		}},
		{"ValidateNetwork", func() (bool, error) {
			return req.ValidateNetwork(ctx, 1000)
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valid, err := tc.fn()
			if err != nil {
				t.Errorf("%s() returned error: %v", tc.name, err)
			}
			if valid {
				t.Errorf("%s() should return false for unimplemented method", tc.name)
			}
		})
	}
}
