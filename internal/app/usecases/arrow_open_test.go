package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type fakeLauncher struct {
	launchable map[domain.Namespace]bool
	launchErr  error
	launched   []domain.Namespace
}

func (f *fakeLauncher) Launchable(
	_ context.Context,
	ns domain.Namespace,
) bool {
	return f.launchable[ns]
}

func (f *fakeLauncher) Launch(
	_ context.Context,
	ns domain.Namespace,
) error {
	f.launched = append(f.launched, ns)
	return f.launchErr
}

const openNS = domain.Namespace("github.com/acme/tool@v1")

func newOpenUC(
	state domain.ArrowState,
	l Launcher,
) ArrowUsecase {
	a := &ucmocks.MockArrow{
		ResolveCataloguedFn: func(_ context.Context, ns domain.Namespace) (domain.Namespace, error) {
			return openNS, nil
		},
		ListFn: func(_ context.Context, _ *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{{Namespace: "github.com/acme/tool", Versions: []models.VersionView{{Namespace: openNS}}}}, nil
		},
		GetDetailFn: func(_ context.Context, _ domain.Namespace) (*models.ArrowDetailView, error) {
			return &models.ArrowDetailView{Metadata: domain.Arrow{Namespace: openNS}}, nil
		},
	}
	rt := &ucmocks.MockRuntime{
		GetStateFn: func(_ context.Context, _ domain.Namespace) (domain.ArrowState, error) {
			return state, nil
		},
		GetRuntimeFn: func(_ context.Context, _ domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{State: state}, nil
		},
	}
	return NewArrowUsecase(a, &ucmocks.MockGraph{}, rt, lifecycle.New(a, rt, &ucmocks.MockGraph{}), l)
}

func TestArrowOpen_LaunchesAReadyArrow(t *testing.T) {
	l := &fakeLauncher{}

	err := newOpenUC(domain.ArrowStateReady, l).Open(context.Background(), "github.com/acme/tool")

	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{openNS}, l.launched, "the catalogued, ref-qualified identity is launched")
}

func TestArrowOpen_RefusesWhatIsNotReady(t *testing.T) {
	for _, state := range []domain.ArrowState{domain.ArrowStateAbsent, domain.ArrowStateInstalling, domain.ArrowStateUpdating} {
		l := &fakeLauncher{}

		err := newOpenUC(state, l).Open(context.Background(), "github.com/acme/tool")

		assert.ErrorIs(t, err, apperrors.ErrNotOpenable, string(state))
		assert.Empty(t, l.launched, string(state))
	}
}

func TestArrowOpen_WithoutALauncherIsNotOpenable(t *testing.T) {
	err := newOpenUC(domain.ArrowStateReady, nil).Open(context.Background(), "github.com/acme/tool")

	assert.ErrorIs(t, err, apperrors.ErrNotOpenable)
}

func TestArrowOpen_PassesTheLaunchErrorThrough(t *testing.T) {
	boom := errors.New("spawn failed")

	err := newOpenUC(domain.ArrowStateReady, &fakeLauncher{launchErr: boom}).Open(context.Background(), "github.com/acme/tool")

	assert.ErrorIs(t, err, boom)
}

func TestArrowList_FlagsOpenableVersions(t *testing.T) {
	testCases := []struct {
		name       string
		state      domain.ArrowState
		launchable bool
		want       bool
	}{
		{"ready and launchable", domain.ArrowStateReady, true, true},
		{"ready without a desktop entry", domain.ArrowStateReady, false, false},
		{"launchable but not ready", domain.ArrowStateInstalling, true, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			l := &fakeLauncher{launchable: map[domain.Namespace]bool{openNS: tc.launchable}}

			got, err := newOpenUC(tc.state, l).List(context.Background(), nil)

			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, tc.want, got[0].Versions[0].Openable)
		})
	}
}

func TestArrowGetDetail_FlagsOpenable(t *testing.T) {
	l := &fakeLauncher{launchable: map[domain.Namespace]bool{openNS: true}}

	ready, err := newOpenUC(domain.ArrowStateReady, l).GetDetail(context.Background(), openNS)
	require.NoError(t, err)
	assert.True(t, ready.Openable)

	notReady, err := newOpenUC(domain.ArrowStateInstalling, l).GetDetail(context.Background(), openNS)
	require.NoError(t, err)
	assert.False(t, notReady.Openable)
}
