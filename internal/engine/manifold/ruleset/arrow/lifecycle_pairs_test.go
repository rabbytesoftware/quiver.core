package arrow

import (
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func TestLifecyclePairsRule_Valid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install:   step.StepList{},
					Uninstall: step.StepList{},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestLifecyclePairsRule_MissingUninstall(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{step.RunStep{}},
					// Uninstall absent
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing uninstall, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_StopWithoutExecute(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Stop: step.StepList{step.RunStep{}},
					// Execute absent — stop without execute is invalid
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing stop, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_FetchOnlyInstallWithoutUninstall_Valid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("fetch", "https://example.com/bin", "${INSTALL_PATH}/bin", "", "30s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestLifecyclePairsRule_ExtractOnlyInstallWithoutUninstall_Valid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewExtractStep("extract", "${WORKDIR}/bin.tar.gz", "${WORKDIR}", "30s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestLifecyclePairsRule_FetchAndExtractOnlyInstallWithoutUninstall_Valid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("fetch", "https://example.com/bin.tar.gz", "${WORKDIR}/bin.tar.gz", "", "30s", true),
						step.NewExtractStep("extract", "${WORKDIR}/bin.tar.gz", "${INSTALL_PATH}", "30s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestLifecyclePairsRule_PortableOnlyInstallWithoutUninstall_Valid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewPortableStep("portable", "${WORKDIR}/app.AppImage", "${WORKDIR}", "30s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestLifecyclePairsRule_FetchAndPortableOnlyInstallWithoutUninstall_Valid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("fetch", "https://example.com/app.AppImage", "${WORKDIR}/app.AppImage", "", "30s", true),
						step.NewPortableStep("portable", "${WORKDIR}/app.AppImage", "${INSTALL_PATH}", "30s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestLifecyclePairsRule_RunOnlyInstallWithoutUninstall_Invalid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{step.NewRunStep("run", "echo hi", false, "10s", true)},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for run-only install without uninstall, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_FetchAndRunInstallWithoutUninstall_Invalid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("fetch", "https://example.com/bin", "${INSTALL_PATH}/bin", "", "30s", true),
						step.NewRunStep("run", "chmod +x bin", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for mixed fetch+run install without uninstall, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_FetchWithNonWorkdirToWithoutUninstall_Invalid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("fetch", "https://example.com/bin", "/usr/local/bin/bin", "", "30s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for fetch with non-workdir-anchored to, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_FetchWithDotDotImmediatelyAfterPrefixWithoutUninstall_Invalid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("fetch", "https://example.com/bin", "${WORKDIR}../x", "", "30s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for to escaping via ${WORKDIR}../x, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_FetchWithTraversalAfterPrefixWithoutUninstall_Invalid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("fetch", "https://example.com/bin", "${WORKDIR}/../../etc/passwd", "", "30s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for to escaping via ${WORKDIR}/../../etc/passwd, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_FetchWithLookalikePrefixWithoutUninstall_Invalid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("fetch", "https://example.com/bin", "${INSTALL_PATHX}/a", "", "30s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for to using a lookalike prefix ${INSTALL_PATHX}, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_FetchWithBadOSArchOverrideToWithoutUninstall_Invalid(t *testing.T) {
	rule := LifecyclePairsRule{}
	f := step.NewFetchStep("fetch", "https://example.com/bin", "${INSTALL_PATH}/bin", "", "30s", true)
	f.To.OSArch = map[string]string{"windows/amd64": "/absolute/bin.exe"}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{f},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for fetch with a bad OSArch override, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_PointerFetchAndExtractOnlyInstallWithoutUninstall_Valid(t *testing.T) {
	rule := LifecyclePairsRule{}
	fetch := step.NewFetchStep("fetch", "https://example.com/bin.tar.gz", "${WORKDIR}/bin.tar.gz", "", "30s", true)
	extract := step.NewExtractStep("extract", "${WORKDIR}/bin.tar.gz", "${INSTALL_PATH}", "30s", true)
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{&fetch, &extract},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestLifecyclePairsRule_PointerFetchAndPortableOnlyInstallWithoutUninstall_Valid(t *testing.T) {
	rule := LifecyclePairsRule{}
	fetch := step.NewFetchStep("fetch", "https://example.com/app.AppImage", "${WORKDIR}/app.AppImage", "", "30s", true)
	portable := step.NewPortableStep("portable", "${WORKDIR}/app.AppImage", "${INSTALL_PATH}", "30s", true)
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{&fetch, &portable},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestLifecyclePairsRule_UninstallWithoutInstall_Invalid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Uninstall: step.StepList{step.NewRunStep("cleanup", "rm -rf .", false, "10s", true)},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for uninstall without install, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_EmptyTargets(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}
