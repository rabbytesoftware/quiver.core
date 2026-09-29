package expose

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
)

type stubShelf struct {
	shelf.Shelf
	applied   shelf.Applied
	applyErr  error
	removeErr error

	ns      domain.Namespace
	workdir string
	expose  domain.Expose
	media   domain.ArrowMedia
	removed []string
}

func (s *stubShelf) Apply(
	_ context.Context,
	ns domain.Namespace,
	workdir string,
	expose domain.Expose,
	media domain.ArrowMedia,
) (shelf.Applied, error) {
	s.ns, s.workdir, s.expose, s.media = ns, workdir, expose, media
	return s.applied, s.applyErr
}

func (s *stubShelf) Remove(
	_ context.Context,
	workdir string,
) error {
	s.removed = append(s.removed, workdir)
	return s.removeErr
}

func exposeStep(
	kind string,
	name string,
) domainstep.ExposeStep {
	s := domainstep.NewExposeStep(kind, name, "${INSTALL_PATH}/"+name)
	s.MediaIcon = "/media.png"
	return s
}

func request() wizstep.Request {
	return wizstep.Request{NSKey: "github.com/acme/tool@v1", WorkDir: "/ns/github.com/acme/tool@v1"}
}

func TestHandler_Expose_AppliesTheBlockInOnePass(t *testing.T) {
	sh := &stubShelf{}
	steps := []domainstep.ExposeStep{exposeStep("cli", "tool"), exposeStep("desktop", "Tool"), exposeStep("cli", "helper")}

	errs := NewHandler(sh).Expose(context.Background(), request(), steps)

	assert.Equal(t, []error{nil, nil, nil}, errs)
	assert.Equal(t, domain.Namespace("github.com/acme/tool@v1"), sh.ns)
	assert.Equal(t, "/ns/github.com/acme/tool@v1", sh.workdir)
	assert.Equal(t, domain.ArrowMedia{Icon: "/media.png"}, sh.media)
	assert.Equal(t, domain.Expose{
		CLI:     []domain.ExposeEntry{{Name: "tool", Path: "${INSTALL_PATH}/tool"}, {Name: "helper", Path: "${INSTALL_PATH}/helper"}},
		Desktop: []domain.ExposeEntry{{Name: "Tool", Path: "${INSTALL_PATH}/Tool"}},
	}, sh.expose)
}

func TestHandler_Expose_ReportsEachEntry(t *testing.T) {
	boom := errors.New("disk full")
	steps := []domainstep.ExposeStep{
		exposeStep("cli", "tool"),
		exposeStep("desktop", "Tool"),
		exposeStep("cli", "helper"),
		exposeStep("tray", "odd"),
	}
	placedTool := shelf.AppliedEntry{Kind: domain.ExposeKindCLI, Index: 0, Name: "tool"}
	refusedApp := shelf.Refusal{Kind: domain.ExposeKindDesktop, Index: 0, Name: "Tool", Reason: "owned by github.com/other/thing"}

	testCases := []struct {
		name     string
		applied  shelf.Applied
		applyErr error
		wantIs   []error
	}{
		{
			name:    "placed and absent entries succeed, refused ones fail",
			applied: shelf.Applied{Entries: []shelf.AppliedEntry{placedTool}, Refused: []shelf.Refusal{refusedApp}},
			wantIs:  []error{nil, ErrRefused, nil, ErrUnknownKind},
		},
		{
			name:     "a failed pass fails every entry it did not place",
			applied:  shelf.Applied{Entries: []shelf.AppliedEntry{placedTool}, Refused: []shelf.Refusal{refusedApp}},
			applyErr: boom,
			wantIs:   []error{nil, ErrRefused, boom, ErrUnknownKind},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sh := &stubShelf{applied: tc.applied, applyErr: tc.applyErr}

			errs := NewHandler(sh).Expose(context.Background(), request(), steps)

			require.Len(t, errs, len(tc.wantIs))
			for i, want := range tc.wantIs {
				if want == nil {
					assert.NoError(t, errs[i], "step %d", i)
					continue
				}
				assert.ErrorIs(t, errs[i], want, "step %d", i)
			}
		})
	}
}

func TestHandler_Execute_RemovesThisWorkdirsEntries(t *testing.T) {
	boom := errors.New("boom")
	sh := &stubShelf{}
	h := NewHandler(sh)

	require.NoError(t, h.Execute(context.Background(), request(), domainstep.NewUnexposeStep()))
	sh.removeErr = boom
	require.ErrorIs(t, h.Execute(context.Background(), request(), domainstep.NewUnexposeStep()), boom)

	assert.Equal(t, []string{"/ns/github.com/acme/tool@v1", "/ns/github.com/acme/tool@v1"}, sh.removed)
}
