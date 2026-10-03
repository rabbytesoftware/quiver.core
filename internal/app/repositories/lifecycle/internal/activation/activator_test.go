package activation_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/activation"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

const coreRow = domain.Namespace("github.com/rabbytesoftware/quiver.core@nightly-latest")

type fakeArrow struct{ err error }

func (f fakeArrow) ResolveCatalogued(_ context.Context, ns domain.Namespace) (domain.Namespace, error) {
	if f.err != nil {
		return "", f.err
	}
	return coreRow, nil
}

type fakeRuntime struct {
	rt       *domainRuntime.ArrowRuntime
	getErr   error
	markErr  error
	clearErr error
	calls    []string
}

func (f *fakeRuntime) GetRuntime(context.Context, domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
	return f.rt, f.getErr
}

func (f *fakeRuntime) MarkActivating(context.Context, domain.Namespace) error {
	f.calls = append(f.calls, "mark")
	if f.markErr == nil && f.rt != nil && f.rt.PendingActivation != nil {
		marked := *f.rt.PendingActivation
		marked.Activating = true
		f.rt.PendingActivation = &marked
	}
	return f.markErr
}

func (f *fakeRuntime) ClearPendingActivation(context.Context, domain.Namespace) error {
	f.calls = append(f.calls, "clear")
	return f.clearErr
}

type fakeTrigger struct {
	fired []string
	state bool
	log   *[]string
}

func (f *fakeTrigger) Fire(path string) {
	f.fired = append(f.fired, path)
	f.state = true
	if f.log != nil {
		*f.log = append(*f.log, "fire")
	}
}

func (f *fakeTrigger) Fired() bool { return f.state }

func stagedRuntime(t *testing.T) (*fakeRuntime, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "quiver-new")
	require.NoError(t, os.WriteFile(path, []byte("new build"), 0o755))
	size, digest, err := selfupdate.Fingerprint(context.Background(), path)
	require.NoError(t, err)
	return &fakeRuntime{rt: &domainRuntime.ArrowRuntime{
		Ref:               coreRow,
		State:             domain.ArrowStateReady,
		PendingActivation: &domainRuntime.PendingActivation{Version: "nightly-2", Path: path, Size: size, Digest: digest},
	}}, path
}

func TestActivator_Activate_FiresTheTriggerOnceTheMarkIsDurable(t *testing.T) {
	rt, path := stagedRuntime(t)
	trig := &fakeTrigger{log: &rt.calls}
	activator := activation.NewActivator(fakeArrow{}, rt, trig)

	started, err := activator.Activate(context.Background(), "github.com/rabbytesoftware/quiver.core")

	require.NoError(t, err)
	assert.True(t, started)
	assert.Equal(t, []string{path}, trig.fired)
	assert.Equal(t, []string{"mark", "fire"}, rt.calls)
}

func TestActivator_Activate_TwiceIsAOneShot(t *testing.T) {
	rt, _ := stagedRuntime(t)
	trig := &fakeTrigger{}
	activator := activation.NewActivator(fakeArrow{}, rt, trig)

	first, err := activator.Activate(context.Background(), coreRow)
	require.NoError(t, err)
	second, err := activator.Activate(context.Background(), coreRow)

	require.NoError(t, err)
	assert.True(t, first)
	assert.False(t, second)
	assert.Len(t, trig.fired, 1)
}

func TestActivator_Activate_NothingToApplyIsANoOp(t *testing.T) {
	staged, _ := stagedRuntime(t)
	alreadyActivating, _ := stagedRuntime(t)
	alreadyActivating.rt.PendingActivation.Activating = true

	testCases := []struct {
		name string
		rt   *fakeRuntime
		trig *fakeTrigger
	}{
		{name: "no runtime", rt: &fakeRuntime{}, trig: &fakeTrigger{}},
		{name: "nothing staged", rt: &fakeRuntime{rt: &domainRuntime.ArrowRuntime{Ref: coreRow, State: domain.ArrowStateReady}}, trig: &fakeTrigger{}},
		{name: "handover already under way in the record", rt: alreadyActivating, trig: &fakeTrigger{}},
		{name: "handover already under way in this process", rt: staged, trig: &fakeTrigger{state: true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			started, err := activation.NewActivator(fakeArrow{}, tc.rt, tc.trig).Activate(context.Background(), coreRow)

			require.NoError(t, err)
			assert.False(t, started)
			assert.Empty(t, tc.trig.fired)
			assert.Empty(t, tc.rt.calls)
		})
	}
}

func TestActivator_Activate_LosingTheRaceToAnotherActivateIsANoOp(t *testing.T) {
	rt, _ := stagedRuntime(t)
	rt.markErr = apperrors.ErrStateViolation
	trig := &fakeTrigger{}

	started, err := activation.NewActivator(fakeArrow{}, rt, trig).Activate(context.Background(), coreRow)

	require.NoError(t, err)
	assert.False(t, started)
	assert.Empty(t, trig.fired)
}

func TestActivator_Activate_Errors(t *testing.T) {
	boom := errors.New("event store down")

	testCases := []struct {
		name    string
		arrow   fakeArrow
		build   func(t *testing.T) *fakeRuntime
		trig    activation.Trigger
		wantErr error
	}{
		{
			name:    "unknown namespace",
			arrow:   fakeArrow{err: apperrors.ErrNotFound},
			build:   func(t *testing.T) *fakeRuntime { rt, _ := stagedRuntime(t); return rt },
			trig:    &fakeTrigger{},
			wantErr: apperrors.ErrNotFound,
		},
		{
			name:    "runtime unreadable",
			build:   func(*testing.T) *fakeRuntime { return &fakeRuntime{getErr: boom} },
			trig:    &fakeTrigger{},
			wantErr: boom,
		},
		{
			name:    "no trigger in this process",
			build:   func(t *testing.T) *fakeRuntime { rt, _ := stagedRuntime(t); return rt },
			trig:    nil,
			wantErr: apperrors.ErrStateViolation,
		},
		{
			name: "mark fails",
			build: func(t *testing.T) *fakeRuntime {
				rt, _ := stagedRuntime(t)
				rt.markErr = boom
				return rt
			},
			trig:    &fakeTrigger{},
			wantErr: boom,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rt := tc.build(t)

			started, err := activation.NewActivator(tc.arrow, rt, tc.trig).Activate(context.Background(), coreRow)

			require.ErrorIs(t, err, tc.wantErr)
			assert.False(t, started)
			if trig, ok := tc.trig.(*fakeTrigger); ok {
				assert.Empty(t, trig.fired)
			}
		})
	}
}

func TestActivator_Activate_AnUnusableBinaryIsDiscardedNotExecuted(t *testing.T) {
	testCases := []struct {
		name   string
		break_ func(path string)
	}{
		{name: "truncated", break_: func(path string) { require.NoError(t, os.WriteFile(path, []byte("new"), 0o755)) }},
		{name: "replaced", break_: func(path string) { require.NoError(t, os.WriteFile(path, []byte("NEW BUILD"), 0o755)) }},
		{name: "gone", break_: func(path string) { require.NoError(t, os.Remove(path)) }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rt, path := stagedRuntime(t)
			tc.break_(path)
			trig := &fakeTrigger{}

			started, err := activation.NewActivator(fakeArrow{}, rt, trig).Activate(context.Background(), coreRow)

			require.ErrorIs(t, err, apperrors.ErrStateViolation)
			require.ErrorIs(t, err, selfupdate.ErrStagedUnusable)
			assert.False(t, started)
			assert.Empty(t, trig.fired)
			assert.Equal(t, []string{"clear"}, rt.calls, "the record is dropped")
			assert.NoFileExists(t, path)
		})
	}
}

func TestActivator_Activate_ADiscardThatCannotBeRecordedStillRefuses(t *testing.T) {
	rt, path := stagedRuntime(t)
	require.NoError(t, os.Remove(path))
	rt.clearErr = errors.New("event store down")
	trig := &fakeTrigger{}

	started, err := activation.NewActivator(fakeArrow{}, rt, trig).Activate(context.Background(), coreRow)

	require.ErrorIs(t, err, selfupdate.ErrStagedUnusable)
	assert.False(t, started)
	assert.Empty(t, trig.fired)
}
