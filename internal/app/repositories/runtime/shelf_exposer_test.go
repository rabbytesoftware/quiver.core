package runtime

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	runtimeMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

const (
	exposerNs      = domain.Namespace("github.com/user/repo@v1.0.0")
	exposerWorkdir = "/q/namespaces/github.com/user/repo@v1.0.0"
	exposerOS      = domain.OSDarwinARM64
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureWarnings(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func exposedArrow() *domain.Arrow {
	return &domain.Arrow{
		Namespace: exposerNs,
		ArrowMeta: domain.ArrowMeta{Media: domain.ArrowMedia{Icon: "icon.png"}},
		Targets: map[domain.OS]domain.Target{
			exposerOS: {Expose: domain.Expose{CLI: []domain.ExposeEntry{{Name: "tool", Path: "bin/tool"}}}},
		},
	}
}

type applyCall struct {
	ns      domain.Namespace
	workdir string
	expose  domain.Expose
	media   domain.ArrowMedia
}

type recordingShelf struct {
	mu      sync.Mutex
	applies []applyCall
	removes []domain.Namespace
	applied shelf.Applied
	err     error
	rmErr   error
}

func (r *recordingShelf) stub() *runtimeMocks.MockShelf {
	return &runtimeMocks.MockShelf{
		ApplyFn: func(_ context.Context, ns domain.Namespace, workdir string, expose domain.Expose, media domain.ArrowMedia) (shelf.Applied, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.applies = append(r.applies, applyCall{ns: ns, workdir: workdir, expose: expose, media: media})
			return r.applied, r.err
		},
		RemoveFn: func(_ context.Context, ns domain.Namespace) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.removes = append(r.removes, ns)
			return r.rmErr
		},
	}
}

func (r *recordingShelf) applyCalls() []applyCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]applyCall(nil), r.applies...)
}

func (r *recordingShelf) removeCalls() []domain.Namespace {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.Namespace(nil), r.removes...)
}

func arrowFrom(
	arrow *domain.Arrow,
	err error,
) GetArrowFn {
	return func(context.Context, domain.Namespace) (*domain.Arrow, error) {
		return arrow, err
	}
}

func sampleApplied() shelf.Applied {
	return shelf.Applied{
		Entries: []shelf.AppliedEntry{{Kind: domain.ExposeKindCLI, Name: "tool", Target: exposerWorkdir + "/bin/tool", Location: "/q/bin/tool"}},
		Refused: []shelf.Refusal{{Kind: domain.ExposeKindDesktop, Name: "App", Reason: "target not found"}},
	}
}

func sampleResult() *domainRuntime.ExposeResult {
	return &domainRuntime.ExposeResult{
		Entries: []domainRuntime.ExposedEntry{{Kind: domain.ExposeKindCLI, Name: "tool", Target: exposerWorkdir + "/bin/tool", Location: "/q/bin/tool"}},
		Refused: []domainRuntime.ExposeRefusal{{Kind: domain.ExposeKindDesktop, Name: "App", Reason: "target not found"}},
	}
}

func TestShelfExposer_Apply(t *testing.T) {
	testCases := []struct {
		name       string
		arrow      *domain.Arrow
		arrowErr   error
		applied    shelf.Applied
		applyErr   error
		want       *domainRuntime.ExposeResult
		wantCalls  int
		wantExpose domain.Expose
		wantWarn   string
	}{
		{
			name:       "entries are applied and mapped",
			arrow:      exposedArrow(),
			applied:    sampleApplied(),
			want:       sampleResult(),
			wantCalls:  1,
			wantExpose: exposedArrow().Targets[exposerOS].Expose,
		},
		{
			name:       "empty result maps to nil",
			arrow:      exposedArrow(),
			wantCalls:  1,
			wantExpose: exposedArrow().Targets[exposerOS].Expose,
		},
		{
			name:       "shelf error keeps the partial result and warns",
			arrow:      exposedArrow(),
			applied:    sampleApplied(),
			applyErr:   errors.New("disk full"),
			want:       sampleResult(),
			wantCalls:  1,
			wantExpose: exposedArrow().Targets[exposerOS].Expose,
			wantWarn:   "disk full",
		},
		{
			name:      "missing target applies an empty expose",
			arrow:     &domain.Arrow{Namespace: exposerNs},
			wantCalls: 1,
		},
		{
			name:     "arrow lookup error skips the shelf",
			arrowErr: errors.New("store down"),
			wantWarn: "store down",
		},
		{
			name: "nil arrow skips the shelf",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureWarnings(t)
			rec := &recordingShelf{applied: tc.applied, err: tc.applyErr}
			e := newShelfExposer(rec.stub(), nil, arrowFrom(tc.arrow, tc.arrowErr), nil, exposerOS)

			got := e.Apply(context.Background(), exposerNs, exposerWorkdir)

			assert.Equal(t, tc.want, got)
			calls := rec.applyCalls()
			require.Len(t, calls, tc.wantCalls)
			if tc.wantCalls > 0 {
				assert.Equal(t, applyCall{ns: exposerNs, workdir: exposerWorkdir, expose: tc.wantExpose, media: tc.arrow.Media}, calls[0])
			}
			if tc.wantWarn != "" {
				assert.Contains(t, logs.String(), tc.wantWarn)
			}
		})
	}
}

func TestShelfExposer_NilShelf_IsInert(t *testing.T) {
	e := newShelfExposer(nil, &mocks.Vault{WorkDirValue: exposerWorkdir}, arrowFrom(exposedArrow(), nil), nil, exposerOS)

	assert.Nil(t, e.Apply(context.Background(), exposerNs, exposerWorkdir))
	assert.Nil(t, e.Reapply(context.Background(), exposerNs))
	assert.Nil(t, e.applyVersioned(context.Background(), exposerNs))
	e.Remove(context.Background(), exposerNs)
}

func TestShelfExposer_Reapply(t *testing.T) {
	testCases := []struct {
		name      string
		arrow     *domain.Arrow
		arrowErr  error
		vault     *mocks.Vault
		want      *domainRuntime.ExposeResult
		wantCalls int
		wantWarn  string
	}{
		{
			name:      "ready arrow with expose is re-applied at its versioned workdir",
			arrow:     exposedArrow(),
			vault:     &mocks.Vault{WorkDirValue: exposerWorkdir},
			want:      sampleResult(),
			wantCalls: 1,
		},
		{
			name:  "arrow without expose is skipped",
			arrow: &domain.Arrow{Namespace: exposerNs},
			vault: &mocks.Vault{WorkDirValue: exposerWorkdir},
		},
		{
			name:     "arrow lookup error is skipped",
			arrowErr: errors.New("store down"),
			vault:    &mocks.Vault{WorkDirValue: exposerWorkdir},
			wantWarn: "store down",
		},
		{
			name:     "workdir error is skipped",
			arrow:    exposedArrow(),
			vault:    &mocks.Vault{WorkDirErr: errors.New("no workdir")},
			wantWarn: "no workdir",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureWarnings(t)
			rec := &recordingShelf{applied: sampleApplied()}
			e := newShelfExposer(rec.stub(), tc.vault, arrowFrom(tc.arrow, tc.arrowErr), nil, exposerOS)

			got := e.Reapply(context.Background(), exposerNs)

			assert.Equal(t, tc.want, got)
			calls := rec.applyCalls()
			require.Len(t, calls, tc.wantCalls)
			if tc.wantCalls > 0 {
				assert.Equal(t, exposerWorkdir, calls[0].workdir)
				assert.Equal(t, []domain.Namespace{exposerNs}, tc.vault.WorkDirNamespaces)
			}
			if tc.wantWarn != "" {
				assert.Contains(t, logs.String(), tc.wantWarn)
			}
		})
	}
}

func TestShelfExposer_Reapply_NilVault_Skips(t *testing.T) {
	rec := &recordingShelf{}
	e := newShelfExposer(rec.stub(), nil, arrowFrom(exposedArrow(), nil), nil, exposerOS)

	assert.Nil(t, e.Reapply(context.Background(), exposerNs))
	assert.Empty(t, rec.applyCalls())
}

func TestShelfExposer_ApplyVersioned(t *testing.T) {
	testCases := []struct {
		name      string
		arrow     *domain.Arrow
		arrowErr  error
		vault     *mocks.Vault
		wantCalls int
	}{
		{
			name:      "arrow with expose is applied",
			arrow:     exposedArrow(),
			vault:     &mocks.Vault{WorkDirValue: exposerWorkdir},
			wantCalls: 1,
		},
		{
			name:      "arrow without expose still prunes",
			arrow:     &domain.Arrow{Namespace: exposerNs},
			vault:     &mocks.Vault{WorkDirValue: exposerWorkdir},
			wantCalls: 1,
		},
		{
			name:     "arrow lookup error is skipped",
			arrowErr: errors.New("store down"),
			vault:    &mocks.Vault{WorkDirValue: exposerWorkdir},
		},
		{
			name:  "workdir error is skipped",
			arrow: exposedArrow(),
			vault: &mocks.Vault{WorkDirErr: errors.New("no workdir")},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingShelf{}
			e := newShelfExposer(rec.stub(), tc.vault, arrowFrom(tc.arrow, tc.arrowErr), nil, exposerOS)

			e.applyVersioned(context.Background(), exposerNs)

			require.Len(t, rec.applyCalls(), tc.wantCalls)
		})
	}
}

func catalogOf(
	namespaces ...domain.Namespace,
) ListArrowsFn {
	return func(context.Context) ([]models.ArrowView, error) {
		views := make([]models.ArrowView, 0, len(namespaces))
		for _, ns := range namespaces {
			views = append(views, models.ArrowView{
				Namespace: ns.BareNamespace(),
				Versions:  []models.VersionView{{Namespace: ns}},
			})
		}
		return views, nil
	}
}

func installedArrows(
	installed map[domain.Namespace]bool,
	errs map[domain.Namespace]error,
) GetArrowFn {
	return func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		if err, ok := errs[ns]; ok {
			return nil, err
		}
		arrow := &domain.Arrow{Namespace: ns}
		if installed[ns] {
			arrow.InstalledAt = time.Unix(1, 0)
		}
		return arrow, nil
	}
}

func TestShelfExposer_Remove(t *testing.T) {
	const (
		sibling = domain.Namespace("github.com/user/repo@v2.0.0")
		other   = domain.Namespace("github.com/other/repo@v1.0.0")
	)
	testCases := []struct {
		name       string
		listArrows ListArrowsFn
		installed  map[domain.Namespace]bool
		errs       map[domain.Namespace]error
		rmErr      error
		wantRemove bool
		wantWarn   string
	}{
		{
			name:       "no other ref removes",
			listArrows: catalogOf(exposerNs),
			installed:  map[domain.Namespace]bool{exposerNs: true},
			wantRemove: true,
		},
		{
			name:       "installed sibling ref keeps the entries",
			listArrows: catalogOf(exposerNs, sibling),
			installed:  map[domain.Namespace]bool{sibling: true},
		},
		{
			name:       "catalogued but uninstalled sibling removes",
			listArrows: catalogOf(exposerNs, sibling),
			wantRemove: true,
		},
		{
			name:       "installed arrow of another namespace removes",
			listArrows: catalogOf(exposerNs, other),
			installed:  map[domain.Namespace]bool{other: true},
			wantRemove: true,
		},
		{
			name:       "forgotten sibling removes",
			listArrows: catalogOf(exposerNs, sibling),
			errs:       map[domain.Namespace]error{sibling: asynxModels.ErrNotFound},
			wantRemove: true,
		},
		{
			name:       "unreadable sibling keeps the entries",
			listArrows: catalogOf(exposerNs, sibling),
			errs:       map[domain.Namespace]error{sibling: errors.New("store down")},
		},
		{
			name: "catalog error keeps the entries and warns",
			listArrows: func(context.Context) ([]models.ArrowView, error) {
				return nil, errors.New("catalog down")
			},
			wantWarn: "catalog down",
		},
		{
			name:       "shelf remove error warns",
			listArrows: catalogOf(exposerNs),
			rmErr:      errors.New("busy"),
			wantRemove: true,
			wantWarn:   "busy",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureWarnings(t)
			rec := &recordingShelf{rmErr: tc.rmErr}
			e := newShelfExposer(rec.stub(), nil, installedArrows(tc.installed, tc.errs), tc.listArrows, exposerOS)

			e.Remove(context.Background(), exposerNs)

			if tc.wantRemove {
				assert.Equal(t, []domain.Namespace{exposerNs}, rec.removeCalls())
			} else {
				assert.Empty(t, rec.removeCalls())
			}
			if tc.wantWarn != "" {
				assert.Contains(t, logs.String(), tc.wantWarn)
			}
		})
	}
}

func TestWithExposed(t *testing.T) {
	testCases := []struct {
		name       string
		lastReturn *domainRuntime.Return
		exposed    *domainRuntime.ExposeResult
		want       *domainRuntime.Return
	}{
		{name: "nil last return stays nil", exposed: sampleResult()},
		{
			name:       "exposed replaces the carried result",
			lastReturn: &domainRuntime.Return{Method: domain.MethodUpdate, Exposed: &domainRuntime.ExposeResult{}},
			exposed:    sampleResult(),
			want:       &domainRuntime.Return{Method: domain.MethodUpdate, Exposed: sampleResult()},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var before domainRuntime.Return
			if tc.lastReturn != nil {
				before = *tc.lastReturn
			}

			got := withExposed(tc.lastReturn, tc.exposed)

			assert.Equal(t, tc.want, got)
			if tc.lastReturn != nil {
				assert.Equal(t, before, *tc.lastReturn)
			}
		})
	}
}
