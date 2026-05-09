package arrow

import (
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestExportStaticRule_Valid(t *testing.T) {
	rule := ExportStaticRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Exports: map[string]string{
					"BIN": "/usr/bin/tool",
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestExportStaticRule_ExportHasVarInterpolation(t *testing.T) {
	rule := ExportStaticRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Exports: map[string]string{
					"BIN": "${INSTALL_PATH}/bin",
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for var interpolation in export, got none")
	}
	if errs[0].Rule != "export_var_interpolation" {
		t.Fatalf("expected rule %q, got %q", "export_var_interpolation", errs[0].Rule)
	}
}

func TestExportStaticRule_EmptyTargets(t *testing.T) {
	rule := ExportStaticRule{}
	m := &domain.Arrow{}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}
