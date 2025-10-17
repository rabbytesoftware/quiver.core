package translator

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/models/arrow"
)

func TestValidateArrow(t *testing.T) {
	tr := &ArrowTranslatorImpl{}

	validArrow := arrow.Arrow{Version: "1.2.0"}

	err := tr.ValidateArrow("1.2.1", "1.3.0", validArrow)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	err = tr.ValidateArrow("1.1.0", "1.3.0", validArrow)
	if err == nil {
		t.Errorf("Expected error for older version, got nil")
	}

	err = tr.ValidateArrow("invalid", "1.3.0", validArrow)
	if err == nil {
		t.Errorf("Expected error for invalid current version, got nil")
	}
}
