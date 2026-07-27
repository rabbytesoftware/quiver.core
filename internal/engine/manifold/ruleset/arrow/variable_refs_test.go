package arrow

import (
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func TestVariableRefsRule_Valid_KnownVar(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Variables: []domain.Variable{
			{Name: "MY_VAR"},
		},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "${MY_VAR}/bin/install", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for known variable ref, got: %v", errs)
	}
}

func TestVariableRefsRule_Valid_BuiltinVars(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "${INSTALL_PATH}/bin && ${ARROW_NAMESPACE} && ${PLATFORM}", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for builtin vars, got: %v", errs)
	}
}

func TestVariableRefsRule_Valid_WorkdirBuiltin(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("dl", "https://example.com/file", "${WORKDIR}/file", "", "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for WORKDIR builtin, got: %v", errs)
	}
}

func TestVariableRefsRule_Valid_RefBuiltin(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep(
							"dl",
							"https://example.com/releases/download/${REF}/app.tar.gz",
							"${WORKDIR}/app.tar.gz",
							"",
							"10s",
							true,
						),
						step.NewRunStep("install", "./install.sh --ref ${REF}", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for REF builtin, got: %v", errs)
	}
}

func TestVariableRefsRule_RefBuiltin_UnknownSiblingStillRejected(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "./install.sh ${REF} ${VERSION}", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got: %v", errs)
	}
	if errs[0].Rule != "unresolved_variable" {
		t.Fatalf("expected rule %q, got %q", "unresolved_variable", errs[0].Rule)
	}
	if errs[0].Message != "unknown variable ${VERSION}" {
		t.Fatalf("expected VERSION to be the rejected token, got %q", errs[0].Message)
	}
}

func TestVariableRefsRule_ColonTokenSkipped(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSWindowsAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Uninstall: step.StepList{
						step.NewRunStep("uninstall", "Start-Process '${env:ProgramFiles}\\App\\uninstall.exe'", false, "", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for colon token (PowerShell env var), got: %v", errs)
	}
}

func TestVariableRefsRule_UnresolvedVar(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "${UNKNOWN_VAR}/bin/install", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for unresolved variable, got none")
	}
	if errs[0].Rule != "unresolved_variable" {
		t.Fatalf("expected rule %q, got %q", "unresolved_variable", errs[0].Rule)
	}
}

func TestVariableRefsRule_NetbridgeVarKnown(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Netbridge: []netbridge.PortDef{
			{Name: "GAME_PORT", Protocol: "tcp", Default: 27015},
		},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "./server --port ${GAME_PORT}", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("netbridge var should be known, got: %v", errs)
	}
}

func TestVariableRefsRule_DottedTokenSkipped(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "${env.MY_PATH}/bin/install", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("dotted token should be skipped, got: %v", errs)
	}
}

func TestVariableRefsRule_FetchStep_UnresolvedVar(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("dl", "https://example.com/${UNKNOWN_VAR}/file", "./file", "", "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for unresolved var in fetch step URL, got none")
	}
	if errs[0].Rule != "unresolved_variable" {
		t.Fatalf("expected rule %q, got %q", "unresolved_variable", errs[0].Rule)
	}
}

func TestVariableRefsRule_FetchStep_ToUnresolvedVar(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("dl", "https://example.com/file", "${UNKNOWN_DEST}/file", "", "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for unresolved var in fetch step to field, got none")
	}
}

func TestVariableRefsRule_MethodStep_UnresolvedVar(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Methods: map[string]domain.Method{
					"update": {
						AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
						Steps: step.StepList{
							step.NewRunStep("update", "${UNKNOWN_UPDATE_VAR}/update.sh", false, "10s", true),
						},
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for unresolved var in method step, got none")
	}
	if errs[0].Rule != "unresolved_variable" {
		t.Fatalf("expected rule %q, got %q", "unresolved_variable", errs[0].Rule)
	}
}

func TestVariableRefsRule_EmptyTargets(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.Arrow{}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}

func TestVariableRefsRule_RunStep_OSArchVariantUnresolvedVar(t *testing.T) {
	rule := VariableRefsRule{}
	manifest := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Name: "test"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.RunStep{
							BasicStep: step.BasicStep{},
							Command: step.Overrideable[string]{
								Default: "echo safe",
								OSArch:  map[string]string{"linux/amd64": "./tool ${UNKNOWN_VAR}"},
							},
							Timeout:  step.Overrideable[string]{Default: "30s"},
							Elevated: step.Overrideable[bool]{},
						},
					},
				},
			},
		},
	}
	errs := rule.Validate(manifest)
	if len(errs) == 0 {
		t.Fatal("expected error for unknown variable in OSArch variant, got none")
	}
}

func TestVariableRefsRule_FetchStep_OSArchVariantUnresolvedVar(t *testing.T) {
	rule := VariableRefsRule{}
	manifest := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Name: "test"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.FetchStep{
							BasicStep: step.BasicStep{},
							URL: step.Overrideable[string]{
								Default: "https://example.com/default.tar.gz",
								OSArch:  map[string]string{"linux/amd64": "https://example.com/${UNKNOWN_URL_VAR}.tar.gz"},
							},
							To:       step.Overrideable[string]{Default: "/tmp/foo"},
							Checksum: step.Overrideable[string]{},
							Timeout:  step.Overrideable[string]{Default: "30s"},
						},
					},
				},
			},
		},
	}
	errs := rule.Validate(manifest)
	if len(errs) == 0 {
		t.Fatal("expected error for unknown variable in FetchStep URL OSArch variant, got none")
	}
}

func TestVariableRefsRule_OSArchVariantKnownVar(t *testing.T) {
	rule := VariableRefsRule{}
	manifest := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Name: "test"},
		Variables: []domain.Variable{{Name: "MY_VAR"}},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.RunStep{
							BasicStep: step.BasicStep{},
							Command: step.Overrideable[string]{
								Default: "echo safe",
								OSArch:  map[string]string{"linux/amd64": "./tool ${MY_VAR}"},
							},
							Timeout:  step.Overrideable[string]{Default: "30s"},
							Elevated: step.Overrideable[bool]{},
						},
					},
				},
			},
		},
	}
	errs := rule.Validate(manifest)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for known variable in OSArch variant, got: %v", errs)
	}
}
