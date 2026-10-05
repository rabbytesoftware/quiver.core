package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/models"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
)

func run(t *testing.T, s domainstep.UIStep, req wizstep.Request) (*domainRuntime.Surface, error) {
	t.Helper()
	var got *domainRuntime.Surface
	req.Emit = func(e models.Event) {
		require.Equal(t, models.EventKindSurface, e.Kind)
		got = e.Surface
	}
	err := NewHandler().Execute(t.Context(), req, s)
	return got, err
}

func TestHandler_Listen(t *testing.T) {
	req := wizstep.Request{Vars: map[string]string{domain.VarArrowUIListen: "/run/x.sock"}}
	got, err := run(t, domainstep.NewUIStep("Chat", []string{"unix"}, "", "", true), req)
	require.NoError(t, err)
	require.Equal(t, &domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen, Path: "/"}, got)
}

func TestHandler_ListenWithoutProvisionedSocket(t *testing.T) {
	_, err := run(t, domainstep.NewUIStep("Chat", []string{"unix"}, "", "", true), wizstep.Request{Vars: map[string]string{}})
	require.ErrorIs(t, err, ErrNoSocket)
}

func TestHandler_StaticIsReadyAndAbsolute(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "dist"), 0o755))
	wantDir, err := filepath.EvalSymlinks(filepath.Join(work, "dist")) // t.TempDir is symlinked on macOS
	require.NoError(t, err)

	got, err := run(t, domainstep.NewUIStep("Docs", nil, "./dist", "/start", true), wizstep.Request{WorkDir: work})
	require.NoError(t, err)
	require.Equal(t, &domainRuntime.Surface{
		Mode:  domainRuntime.SurfaceModeStatic,
		Path:  "/start",
		Dir:   wantDir,
		Ready: true,
	}, got)
}

func TestHandler_StaticRejectsEscapeAndMissing(t *testing.T) {
	work := t.TempDir()
	for _, dir := range []string{"../outside", "/etc", "./nope"} {
		_, err := run(t, domainstep.NewUIStep("Docs", nil, dir, "", true), wizstep.Request{WorkDir: work})
		require.ErrorIs(t, err, ErrBadStaticDir, dir)
	}
}

func TestHandler_StaticRejectsSymlinkEscape(t *testing.T) {
	work, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(work, "link")))

	_, err := run(t, domainstep.NewUIStep("Docs", nil, "./link", "", true), wizstep.Request{WorkDir: work})
	require.ErrorIs(t, err, ErrBadStaticDir)
}

func TestHandler_StaticExpandsVariables(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "site"), 0o755))
	req := wizstep.Request{WorkDir: work, Vars: map[string]string{"SITE": "site"}}

	want, err := filepath.EvalSymlinks(filepath.Join(work, "site"))
	require.NoError(t, err)

	got, err := run(t, domainstep.NewUIStep("Docs", nil, "./${SITE}", "", true), req)
	require.NoError(t, err)
	require.Equal(t, want, got.Dir)
}

func TestHandler_NilEmitIsNobodyListening(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "dist"), 0o755))

	testCases := []struct {
		name string
		step domainstep.UIStep
		req  wizstep.Request
	}{
		{
			name: "listen",
			step: domainstep.NewUIStep("Chat", []string{"unix"}, "", "", true),
			req:  wizstep.Request{Vars: map[string]string{domain.VarArrowUIListen: "/run/x.sock"}},
		},
		{
			name: "static",
			step: domainstep.NewUIStep("Docs", nil, "./dist", "", true),
			req:  wizstep.Request{WorkDir: work},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				require.NoError(t, NewHandler().Execute(t.Context(), tc.req, tc.step))
			})
		})
	}
}
