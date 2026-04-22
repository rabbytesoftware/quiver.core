package catalog

import (
	"context"
	"errors"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
)

func makeManifest(name string) *domain.Arrow {
	return &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:    name,
			Version: "1.0.0",
		},
	}
}

// seedArrowViaCommand sends an AddArrow command to asynx for the given arrow
func seedArrowViaCommand(
	t *testing.T,
	axArrow asynx.Asynx[domain.Arrow],
	arrow *domain.Arrow,
) {
	t.Helper()

	cmd := arrowcmds.AddArrow{
		Namespace:     arrow.Namespace,
		ArrowMeta:     arrow.ArrowMeta,
		Variables:     arrow.Variables,
		Netbridge:     arrow.Netbridge,
		Targets:       arrow.Targets,
		DirectInstall: arrow.UserInstalled,
	}
	_, err := axArrow.Send(context.Background(), cmd)
	require.NoError(t, err)
	axArrow.WaitPublish()
}

func newAsynxArrow(es asynxModels.Store) (asynx.Asynx[domain.Arrow], error) {
	return asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

func newAsynxRuntime(es asynxModels.Store) (asynx.Asynx[domainRuntime.ArrowRuntime], error) {
	return asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

func testCatalog(t *testing.T) (*catalogService, Catalog) {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)

	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	cat, err := store.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	svc := &catalogService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		store:     cat,
	}

	err = svc.registerProjections()
	require.NoError(t, err)

	return svc, svc
}

func TestGet_NotFound_ReturnsNil(t *testing.T) {
	_, cat := testCatalog(t)

	vm, err := cat.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Nil(t, vm)
}

func TestProjection_ArrowAdded_CreatesViewModelWithOneVersion(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

	arrow := m
	arrow.Namespace = "github.com/org/repo@v1.0"
	seedArrowViaCommand(t, svc.axArrow, arrow)

	vm, err := cat.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	require.NotNil(t, vm)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), vm.Namespace)
	assert.Len(t, vm.Versions, 1)
	assert.Equal(t, domain.Namespace("github.com/org/repo@v1.0"), vm.Versions[0].Namespace)
}

func TestProjection_MultipleVersionsAdded_AggregatesInViewModel(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

	// Add v1.0
	v1 := *m
	v1.Namespace = "github.com/org/repo@v1.0"
	v1.Version = "1.0"
	seedArrowViaCommand(t, svc.axArrow, &v1)

	// Add v2.0
	v2 := *m
	v2.Namespace = "github.com/org/repo@v2.0"
	v2.Version = "2.0"
	seedArrowViaCommand(t, svc.axArrow, &v2)

	vm, err := cat.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	require.NotNil(t, vm)
	assert.Len(t, vm.Versions, 2)
	assert.Equal(t, domain.Namespace("github.com/org/repo@v1.0"), vm.Versions[0].Namespace)
	assert.Equal(t, domain.Namespace("github.com/org/repo@v2.0"), vm.Versions[1].Namespace)
}

func TestProjection_UserInstalledPreferred_SelectsUserInstalledMetadata(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

	// Add v1.0 as user installed
	v1 := *m
	v1.Namespace = "github.com/org/repo@v1.0"
	v1.Version = "1.0"
	v1.UserInstalled = true
	seedArrowViaCommand(t, svc.axArrow, &v1)

	// Add v2.0 as NOT user installed
	v2 := *m
	v2.Namespace = "github.com/org/repo@v2.0"
	v2.Version = "2.0"
	v2.UserInstalled = false
	seedArrowViaCommand(t, svc.axArrow, &v2)

	vm, err := cat.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	require.NotNil(t, vm)
	// Metadata should be from v1 since it's user-installed
	assert.Equal(t, domain.Namespace("github.com/org/repo@v1.0"), vm.Metadata.Namespace)
}

func TestProjection_OnForget_RemovesVersionAndDeletesEmptyViewModel(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

	v1 := *m
	v1.Namespace = "github.com/org/repo@v1.0"
	seedArrowViaCommand(t, svc.axArrow, &v1)

	vm, err := cat.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	require.NotNil(t, vm)

	// Forget the version
	err = svc.axArrow.Forget(context.Background(), "github.com/org/repo@v1.0")
	require.NoError(t, err)
	svc.axArrow.WaitPublish()

	// View model should be gone
	vm, err = cat.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Nil(t, vm)
}

func TestProjection_OnForgetOneOfMany_RemovesVersionKeepsViewModel(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

	// Add v1.0
	v1 := *m
	v1.Namespace = "github.com/org/repo@v1.0"
	seedArrowViaCommand(t, svc.axArrow, &v1)

	// Add v2.0
	v2 := *m
	v2.Namespace = "github.com/org/repo@v2.0"
	seedArrowViaCommand(t, svc.axArrow, &v2)

	// Forget v1.0
	err := svc.axArrow.Forget(context.Background(), "github.com/org/repo@v1.0")
	require.NoError(t, err)
	svc.axArrow.WaitPublish()

	// View model should still exist with only v2.0
	vm, err := cat.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	require.NotNil(t, vm)
	assert.Len(t, vm.Versions, 1)
	assert.Equal(t, domain.Namespace("github.com/org/repo@v2.0"), vm.Versions[0].Namespace)
}

func TestNew_ValidArgs_ReturnsCatalog(t *testing.T) {
	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)

	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	cat, err := store.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	result, err := New(axArrow, axRuntime, cat)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNew_FailsWhenAsynxSubscribeFails(t *testing.T) {
	wantErr := errors.New("subscribe failed")

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	s, err := store.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	cat, err := New(&failingArrowAsynx{err: wantErr}, axRuntime, s)

	assert.Nil(t, cat)
	require.ErrorIs(t, err, wantErr)
}

func TestList_ReturnsAllAggregatedViewModels(t *testing.T) {
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t)

	// Add two versions of repo1
	v1 := *m
	v1.Namespace = "github.com/org/repo1@v1.0"
	seedArrowViaCommand(t, svc.axArrow, &v1)

	v2 := *m
	v2.Namespace = "github.com/org/repo1@v2.0"
	seedArrowViaCommand(t, svc.axArrow, &v2)

	// Add one version of repo2
	v3 := *m
	v3.Namespace = "github.com/org/repo2@v1.0"
	seedArrowViaCommand(t, svc.axArrow, &v3)

	result, err := cat.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, result, 2)

	// Check aggregation is correct
	var repo1, repo2 *store.ArrowViewModel
	for i := range result {
		if result[i].Namespace == "github.com/org/repo1" {
			repo1 = &result[i]
		} else if result[i].Namespace == "github.com/org/repo2" {
			repo2 = &result[i]
		}
	}

	require.NotNil(t, repo1)
	require.NotNil(t, repo2)
	assert.Len(t, repo1.Versions, 2)
	assert.Len(t, repo2.Versions, 1)
}

// failingArrowAsynx is a minimal asynx.Asynx[domain.Arrow] stub whose
// Subscribe always returns an error.
type failingArrowAsynx struct{ err error }

func (f *failingArrowAsynx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Arrow],
	_ ...asynxModels.SubscriptionOpt[domain.Arrow],
) (string, error) {
	return "", f.err
}
func (f *failingArrowAsynx) Send(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}
func (f *failingArrowAsynx) SendWait(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}
func (f *failingArrowAsynx) Get(_ context.Context, _ string) (domain.Arrow, error) {
	return domain.Arrow{}, nil
}
func (f *failingArrowAsynx) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *failingArrowAsynx) Preload(_ context.Context, _ string) error        { return nil }
func (f *failingArrowAsynx) Unsubscribe(_ string) error                       { return nil }
func (f *failingArrowAsynx) Replay(
	_ context.Context, _ string, _ int64, _ int64,
	_ asynxModels.ProjectionHandler[domain.Arrow],
) error {
	return nil
}
func (f *failingArrowAsynx) Shutdown(_ context.Context) error         { return nil }
func (f *failingArrowAsynx) WaitPublish()                             {}
func (f *failingArrowAsynx) Forget(_ context.Context, _ string) error { return nil }
func (f *failingArrowAsynx) OnForget(_ asynxModels.ForgetHandler[domain.Arrow]) (string, error) {
	return "", f.err
}
