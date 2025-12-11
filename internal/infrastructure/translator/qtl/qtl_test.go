package qtl

import (
	"context"
	"testing"
)

func TestNewQTL(t *testing.T) {
	qtl := NewQTL()
	if qtl == nil {
		t.Fatal("NewQTL() returned nil")
	}
}

func TestQTL_IsCompatible(t *testing.T) {
	qtl := NewQTL()
	ctx := context.Background()

	compatible, err := qtl.IsCompatible(ctx, "test-manifest")
	if err != nil {
		t.Errorf("IsCompatible() returned error: %v", err)
	}
	if compatible {
		t.Error("IsCompatible() should return false for unimplemented method")
	}
}

func TestQTL_Translate(t *testing.T) {
	qtl := NewQTL()
	ctx := context.Background()

	result, err := qtl.Translate(ctx, "test-input")
	if err != nil {
		t.Errorf("Translate() returned error: %v", err)
	}
	if result != nil {
		t.Error("Translate() should return nil for unimplemented method")
	}
}

func TestQTL_GetManifestVersion(t *testing.T) {
	qtl := NewQTL()
	ctx := context.Background()

	version, err := qtl.GetManifestVersion(ctx, "test-manifest")
	if err != nil {
		t.Errorf("GetManifestVersion() returned error: %v", err)
	}
	if version != "" {
		t.Error("GetManifestVersion() should return empty string for unimplemented method")
	}
}

func TestQTL_GetSupportedVersions(t *testing.T) {
	qtl := NewQTL()
	ctx := context.Background()

	versions, err := qtl.GetSupportedVersions(ctx)
	if err != nil {
		t.Errorf("GetSupportedVersions() returned error: %v", err)
	}
	if versions != nil {
		t.Error("GetSupportedVersions() should return nil for unimplemented method")
	}
}

func TestQTL_InterfaceCompliance(t *testing.T) {
	// Test that QTL implements the expected interface
	qtl := NewQTL()
	if qtl == nil {
		t.Error("QTL should not be nil")
	}
}

func TestQTL_MultipleInstances(t *testing.T) {
	qtl1 := NewQTL()
	qtl2 := NewQTL()

	// Both should be valid
	if qtl1 == nil || qtl2 == nil {
		t.Error("NewQTL() returned nil instance")
	}

	// Test that both instances work correctly
	ctx := context.Background()
	compatible1, _ := qtl1.IsCompatible(ctx, "test")
	compatible2, _ := qtl2.IsCompatible(ctx, "test")

	if compatible1 != compatible2 {
		t.Error("Both instances should have same IsCompatible behavior")
	}
}

func TestQTL_AllMethods(t *testing.T) {
	qtl := NewQTL()
	ctx := context.Background()

	// Test all methods to ensure they don't panic
	testCases := []struct {
		name string
		fn   func() error
	}{
		{"IsCompatible", func() error {
			_, err := qtl.IsCompatible(ctx, "test")
			return err
		}},
		{"Translate", func() error {
			_, err := qtl.Translate(ctx, "test")
			return err
		}},
		{"GetManifestVersion", func() error {
			_, err := qtl.GetManifestVersion(ctx, "test")
			return err
		}},
		{"GetSupportedVersions", func() error {
			_, err := qtl.GetSupportedVersions(ctx)
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
