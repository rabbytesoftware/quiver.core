package arrow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestLifecyclePairsRule_EmptyTargets(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}

func TestLifecyclePairsRule_Validate_WorkdirOnlyInstalls(t *testing.T) {
	fetch := func(to string) step.Step {
		return step.NewFetchStep("fetch", "https://example.com/bin", to, "", "30s", true)
	}
	run := step.NewRunStep("run", "echo hi", false, "10s", true)
	badOverride := step.NewFetchStep("fetch", "https://example.com/bin", "${INSTALL_PATH}/bin", "", "30s", true)
	badOverride.To.OSArch = map[string]string{"windows/amd64": "/absolute/bin.exe"}

	testCases := []struct {
		name      string
		lifecycle domain.TargetLifecycle
		wantPair  bool
	}{
		{
			name:      "uninstall without install",
			lifecycle: domain.TargetLifecycle{Uninstall: step.StepList{run}},
			wantPair:  true,
		},
		{
			name:      "fetch-only install",
			lifecycle: domain.TargetLifecycle{Install: step.StepList{fetch("${INSTALL_PATH}/bin")}},
		},
		{
			name: "extract-only install",
			lifecycle: domain.TargetLifecycle{Install: step.StepList{
				step.NewExtractStep("extract", "${WORKDIR}/bin.tar.gz", "${WORKDIR}", "30s", true),
			}},
		},
		{
			name: "portable-only install",
			lifecycle: domain.TargetLifecycle{Install: step.StepList{
				step.NewPortableStep("portable", "${WORKDIR}/app.AppImage", "${WORKDIR}", "30s", true),
			}},
		},
		{
			name: "fetch and extract install",
			lifecycle: domain.TargetLifecycle{Install: step.StepList{
				fetch("${WORKDIR}/bin.tar.gz"),
				step.NewExtractStep("extract", "${WORKDIR}/bin.tar.gz", "${INSTALL_PATH}", "30s", true),
			}},
		},
		{
			name: "fetch and portable install",
			lifecycle: domain.TargetLifecycle{Install: step.StepList{
				fetch("${WORKDIR}/app.AppImage"),
				step.NewPortableStep("portable", "${WORKDIR}/app.AppImage", "${INSTALL_PATH}", "30s", true),
			}},
		},
		{
			name:      "fetch and run install",
			lifecycle: domain.TargetLifecycle{Install: step.StepList{fetch("${INSTALL_PATH}/bin"), run}},
			wantPair:  true,
		},
		{
			name:      "fetch outside the workdir",
			lifecycle: domain.TargetLifecycle{Install: step.StepList{fetch("/usr/local/bin/bin")}},
			wantPair:  true,
		},
		{
			name:      "traversal right after the prefix",
			lifecycle: domain.TargetLifecycle{Install: step.StepList{fetch("${WORKDIR}../x")}},
			wantPair:  true,
		},
		{
			name:      "traversal after the prefix",
			lifecycle: domain.TargetLifecycle{Install: step.StepList{fetch("${WORKDIR}/../../etc/passwd")}},
			wantPair:  true,
		},
		{
			name:      "lookalike prefix",
			lifecycle: domain.TargetLifecycle{Install: step.StepList{fetch("${INSTALL_PATHX}/a")}},
			wantPair:  true,
		},
		{
			name:      "bad os override",
			lifecycle: domain.TargetLifecycle{Install: step.StepList{badOverride}},
			wantPair:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &domain.Arrow{
				Targets: map[domain.OS]domain.Target{
					domain.OSLinuxAMD64: {Lifecycle: tc.lifecycle},
				},
			}

			errs := LifecyclePairsRule{}.Validate(m)

			if !tc.wantPair {
				assert.Empty(t, errs)
				return
			}
			require.NotEmpty(t, errs)
			assert.Equal(t, "missing_pair", errs[0].Rule)
		})
	}
}
