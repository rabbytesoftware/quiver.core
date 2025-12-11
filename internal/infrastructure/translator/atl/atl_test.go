package atl

import (
	"context"
	"testing"
)

func TestNewATL(t *testing.T) {
	atl := NewATL()
	if atl == nil {
		t.Fatal("NewATL() returned nil")
	}
}

func TestATL_IsCompatible(t *testing.T) {
	atl := NewATL()
	ctx := context.Background()

	compatible, err := atl.IsCompatible(ctx, "test-manifest")
	if err != nil {
		t.Errorf("IsCompatible() returned error: %v", err)
	}
	if compatible {
		t.Error("IsCompatible() should return false for unimplemented method")
	}
}

func TestATL_Translate(t *testing.T) {
	atl := NewATL()
	ctx := context.Background()

	result, err := atl.Translate(ctx, "test-input")
	if err != nil {
		t.Errorf("Translate() returned error: %v", err)
	}
	if result != nil {
		t.Error("Translate() should return nil for unimplemented method")
	}
}

func TestATL_GetManifestVersion(t *testing.T) {
	atl := NewATL()
	ctx := context.Background()

	version, err := atl.GetManifestVersion(ctx, "test-manifest")
	if err != nil {
		t.Errorf("GetManifestVersion() returned error: %v", err)
	}
	if version != "" {
		t.Error("GetManifestVersion() should return empty string for unimplemented method")
	}
}

func TestATL_GetSupportedVersions(t *testing.T) {
	atl := NewATL()
	ctx := context.Background()

	versions, err := atl.GetSupportedVersions(ctx)
	if err != nil {
		t.Errorf("GetSupportedVersions() returned error: %v", err)
	}
	if versions != nil {
		t.Error("GetSupportedVersions() should return nil for unimplemented method")
	}
}

func TestATL_InterfaceCompliance(t *testing.T) {
	// Test that ATL implements the expected interface
	atl := NewATL()
	if atl == nil {
		t.Error("ATL should not be nil")
	}
}

func TestATL_MultipleInstances(t *testing.T) {
	atl1 := NewATL()
	atl2 := NewATL()

	// Both should be valid
	if atl1 == nil || atl2 == nil {
		t.Error("NewATL() returned nil instance")
	}

	// Test that both instances work correctly
	ctx := context.Background()
	compatible1, _ := atl1.IsCompatible(ctx, "test")
	compatible2, _ := atl2.IsCompatible(ctx, "test")

	if compatible1 != compatible2 {
		t.Error("Both instances should have same IsCompatible behavior")
	}
}

func TestATL_AllMethods(t *testing.T) {
	atl := NewATL()
	ctx := context.Background()

	// Test all methods to ensure they don't panic
	testCases := []struct {
		name string
		fn   func() error
	}{
		{"IsCompatible", func() error {
			_, err := atl.IsCompatible(ctx, "test")
			return err
		}},
		{"Translate", func() error {
			_, err := atl.Translate(ctx, "test")
			return err
		}},
		{"GetManifestVersion", func() error {
			_, err := atl.GetManifestVersion(ctx, "test")
			return err
		}},
		{"GetSupportedVersions", func() error {
			_, err := atl.GetSupportedVersions(ctx)
			return err
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err != nil {
				t.Errorf("%s() returned error: %v", tc.name, err)
			}
		})
	}
}
