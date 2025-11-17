package translator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	fns "github.com/rabbytesoftware/quiver/internal/infrastructure/fetchnshare"
)

func TestNewTranslator(t *testing.T) {
	mockFNS := fns.NewFNS()
	tr := NewTranslator(mockFNS)
	if tr == nil {
		t.Fatal("NewTranslator() returned nil")
	}
}

func TestTranslator_InterfaceCompliance(t *testing.T) {
	// Test that TranslatorImplementation implements Translator interface
	mockFNS := fns.NewFNS()
	var _ Translator = NewTranslator(mockFNS)
}

func TestTranslator_MultipleInstances(t *testing.T) {
	mockFNS := fns.NewFNS()
	tr1 := NewTranslator(mockFNS)
	tr2 := NewTranslator(mockFNS)

	// Both should be valid
	if tr1 == nil || tr2 == nil {
		t.Error("NewTranslator() returned nil instance")
	}
}

func TestTranslator_TranslateArrow(t *testing.T) {
	mockFNS := fns.NewFNS()
	tr := NewTranslator(mockFNS)
	ctx := context.Background()

	// Test with non-existent file (should return error)
	_, err := tr.TranslateArrow(ctx, "non-existent-file.yaml")
	if err == nil {
		t.Error("TranslateArrow() should return error for non-existent file")
	}
}

func TestTranslator_TranslateQuiver(t *testing.T) {
	mockFNS := fns.NewFNS()
	tr := NewTranslator(mockFNS)
	ctx := context.Background()

	// Test with non-existent file (should return error)
	_, err := tr.TranslateQuiver(ctx, "non-existent-file.yaml")
	if err == nil {
		t.Error("TranslateQuiver() should return error for non-existent file")
	}
}

func TestTranslator_GetManifestInfo(t *testing.T) {
	mockFNS := fns.NewFNS()
	tr := NewTranslator(mockFNS)
	ctx := context.Background()

	// Test with non-existent file (should return error)
	_, err := tr.GetManifestInfo(ctx, "non-existent-file.yaml")
	if err == nil {
		t.Error("GetManifestInfo() should return error for non-existent file")
	}
}

func TestTranslator_IsCompatible(t *testing.T) {
	mockFNS := fns.NewFNS()
	tr := NewTranslator(mockFNS)
	ctx := context.Background()

	// Create a temporary test file with arrow manifest
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.yaml")
	content := []byte("manifest: arrow@v1\nname: test\nversion: 1.0.0\n")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test compatibility
	compatible, err := tr.IsCompatible(ctx, testFile, "arrow")
	if err != nil {
		t.Errorf("IsCompatible() returned error: %v", err)
	}
	if !compatible {
		t.Error("IsCompatible() should return true for arrow manifest")
	}

	// Test with wrong schema type
	compatible, err = tr.IsCompatible(ctx, testFile, "quiver")
	if err != nil {
		t.Errorf("IsCompatible() returned error: %v", err)
	}
	if compatible {
		t.Error("IsCompatible() should return false for mismatched schema type")
	}
}
