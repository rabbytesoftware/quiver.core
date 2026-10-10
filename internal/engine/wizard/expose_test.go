package wizard

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

type exposeSandbox struct {
	home string
	bin  string
	w    Wizard
}

func newExposeSandbox(
	t *testing.T,
) exposeSandbox {
	t.Helper()
	if goruntime.GOOS == "windows" {
		t.Skip("exposes command folders on the user Path instead of symlinks")
	}
	home := t.TempDir()
	bin, err := paths.BinAt(home)
	require.NoError(t, err)
	w, err := New(nil, testExtractMaxBytes, WithSandboxHome(home))
	require.NoError(t, err)
	return exposeSandbox{home: home, bin: bin, w: w}
}

func (s exposeSandbox) emptyWorkdir(
	t *testing.T,
	ns domain.Namespace,
) string {
	t.Helper()
	nsDir, err := paths.NamespacesAt(s.home)
	require.NoError(t, err)
	dir := filepath.Join(nsDir, filepath.FromSlash(ns.String()))
	require.NoError(t, os.MkdirAll(dir, 0o750))
	return dir
}

func (s exposeSandbox) workdir(
	t *testing.T,
	ns domain.Namespace,
) string {
	t.Helper()
	dir := s.emptyWorkdir(t, ns)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tool"), []byte("#!/bin/sh\n"), 0o700)) // #nosec G306 -- fixture executable
	return dir
}

func (s exposeSandbox) run(
	ns domain.Namespace,
	method string,
	workdir string,
	steps ...domainstep.Step,
) testRecord {
	return runSync(context.Background(), s.w, RunRequest{Namespace: ns, Method: method, WorkDir: workdir, Steps: steps})
}

func (s exposeSandbox) link(
	t *testing.T,
	name string,
) string {
	t.Helper()
	target, err := os.Readlink(filepath.Join(s.bin, name))
	require.NoError(t, err)
	return target
}

func TestStart_ExposeSteps_ReportEachEntryWithoutFailingTheRun(t *testing.T) {
	s := newExposeSandbox(t)
	ns := domain.Namespace("github.com/acme/tool@v1")
	wd := s.workdir(t, ns)
	require.NoError(t, os.WriteFile(filepath.Join(s.bin, "clash"), []byte("mine"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(wd, "clash"), []byte("x"), 0o700)) // #nosec G306 -- fixture executable

	rec := s.run(ns, domain.MethodInstall, wd,
		domainstep.NewRunStep("prepare", "true", false, "", true),
		domainstep.NewExposeStep("cli", "tool", "${INSTALL_PATH}/tool"),
		domainstep.NewExposeStep("cli", "clash", "${INSTALL_PATH}/clash"),
	)

	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, rec.Outcome)
	assert.Equal(t, []int{0, 1, 2}, rec.Started)
	assert.Equal(t, []int{0, 1}, rec.Completed)
	assert.Equal(t, []int{2}, rec.Failed)
	assert.Equal(t, filepath.Join(wd, "tool"), s.link(t, "tool"))
}

func TestStart_ExposeAutoStepThatFindsNothing_CompletesWithANote(t *testing.T) {
	s := newExposeSandbox(t)
	ns := domain.Namespace("github.com/acme/tool@v1")
	wd := s.emptyWorkdir(t, ns)

	rec := s.run(ns, domain.MethodInstall, wd,
		domainstep.NewExposeStep("desktop", "Tool", domain.ExposeAuto),
		domainstep.NewExposeStep("cli", "tool", domain.ExposeAuto),
	)

	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, rec.Outcome)
	assert.Equal(t, []int{0, 1}, rec.Completed)
	assert.Empty(t, rec.Failed)
	assert.Contains(t, rec.Notes, 0)
	assert.Contains(t, rec.Notes, 1)
}

func TestStart_ExposeStepsAfterAFatalFailure_NeverRun(t *testing.T) {
	s := newExposeSandbox(t)
	ns := domain.Namespace("github.com/acme/tool@v1")
	wd := s.workdir(t, ns)

	rec := s.run(ns, domain.MethodInstall, wd,
		domainstep.NewRunStep("broken", "false", false, "", true),
		domainstep.NewExposeStep("cli", "tool", "${INSTALL_PATH}/tool"),
	)

	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, rec.Outcome)
	assert.Equal(t, []int{0}, rec.Started)
	assert.NoFileExists(t, filepath.Join(s.bin, "tool"))
}

func TestStart_UnexposeStep_RemovesOnlyThisWorkdirsEntries(t *testing.T) {
	s := newExposeSandbox(t)
	v1 := domain.Namespace("github.com/acme/tool@v1")
	v2 := domain.Namespace("github.com/acme/tool@v2")
	wd1 := s.workdir(t, v1)
	wd2 := s.workdir(t, v2)
	expose := domainstep.NewExposeStep("cli", "tool", "${INSTALL_PATH}/tool")
	s.run(v1, domain.MethodInstall, wd1, expose)
	s.run(v2, domain.MethodInstall, wd2, expose)
	require.Equal(t, filepath.Join(wd2, "tool"), s.link(t, "tool"))

	rec := s.run(v1, domain.MethodUninstall, wd1, domainstep.NewUnexposeStep())

	assert.Equal(t, []int{0}, rec.Completed)
	assert.Equal(t, filepath.Join(wd2, "tool"), s.link(t, "tool"))

	rec = s.run(v2, domain.MethodUninstall, wd2, domainstep.NewUnexposeStep())

	assert.Equal(t, []int{0}, rec.Completed)
	_, err := os.Lstat(filepath.Join(s.bin, "tool"))
	assert.True(t, os.IsNotExist(err))
}

func TestStart_UnexposeStep_OutsideTheNamespacesFails(t *testing.T) {
	s := newExposeSandbox(t)

	rec := s.run("github.com/acme/tool@v1", domain.MethodUninstall, t.TempDir(), domainstep.NewUnexposeStep())

	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, rec.Outcome)
	assert.Equal(t, []int{0}, rec.Failed)
}

func TestWizard_PathStatus_StaysInTheSandbox(t *testing.T) {
	s := newExposeSandbox(t)

	status, err := s.w.PathStatus(context.Background())
	require.NoError(t, err)
	assert.Equal(t, s.bin, status.BinDir)

	status, err = s.w.SetupPath(context.Background())
	require.NoError(t, err)
	assert.Equal(t, s.bin, status.BinDir)
	for _, f := range status.Files {
		rel, err := filepath.Rel(s.home, f)
		require.NoError(t, err)
		assert.NotContains(t, rel, "..")
	}
}

func TestLeadingExposeSteps(t *testing.T) {
	expose := domainstep.NewExposeStep("cli", "tool", "tool")
	run := domainstep.NewRunStep("r", "true", false, "", true)

	assert.Empty(t, leadingExposeSteps([]domainstep.Step{run, expose}))
	assert.Equal(t, []domainstep.ExposeStep{expose, expose}, leadingExposeSteps([]domainstep.Step{expose, expose, run, expose}))
}

func TestPlan_ExposesTheTargetOfTheGivenOS(t *testing.T) {
	arrow := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Media: domain.ArrowMedia{Icon: "icon.png"}},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {Expose: domain.Expose{CLI: []domain.ExposeEntry{{Name: "tool", Path: "bin/tool"}}}},
		},
	}
	run := domainstep.NewRunStep("r", "true", false, "", true)
	want := domainstep.NewExposeStep("cli", "tool", "bin/tool")
	want.MediaIcon = "icon.png"

	assert.Equal(t, []domainstep.Step{run, want}, Plan(domain.MethodInstall, arrow, domain.OSLinuxAMD64, []domainstep.Step{run}))
	assert.Equal(t, []domainstep.Step{run}, Plan(domain.MethodInstall, arrow, domain.OSDarwinARM64, []domainstep.Step{run}))
}

func TestStart_RunStepNamingTheAppStartsTheExposedExecutable(t *testing.T) {
	s := newExposeSandbox(t)
	ns := domain.Namespace("github.com/acme/tool@v1")
	wd := s.emptyWorkdir(t, ns)
	out := filepath.Join(wd, "ran")
	app := filepath.Join(wd, "app")
	require.NoError(t, os.WriteFile(app, []byte("#!/bin/sh\necho started > \""+out+"\"\n"), 0o700)) // #nosec G306 -- fixture executable
	start := domainstep.NewRunStep("Start", `exec "${ARROW_APP}"`, false, "10s", true)

	missing := s.run(ns, domain.MethodExecute, wd, start)
	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, missing.Outcome, "no recorded app fails the start instead of running an empty command")
	assert.NoFileExists(t, out)

	require.NoError(t, os.WriteFile(filepath.Join(wd, ".quiver-app"), []byte(app+"\n"), 0o600))
	started := s.run(ns, domain.MethodExecute, wd, start)
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, started.Outcome)
	assert.FileExists(t, out)
}
