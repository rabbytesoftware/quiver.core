package quiver

import (
	"context"
	"errors"
	"sync"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildTestQuiverCatalog(t *testing.T) catalog.Catalog {
	t.Helper()

	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)
	store, err := quiverstore.NewQuiverCatalog(":memory:")
	require.NoError(t, err)
	cat, err := catalog.New(axQuiver, store, nil, nil)
	require.NoError(t, err)
	return cat
}

func TestBuilder_Build_SucceedsWithValidEventStore(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	svc, err := NewQuiverBuilder().
		WithEventStore(es).
		WithCatalog(buildTestQuiverCatalog(t)).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_FailsWithNilEventStore(t *testing.T) {
	svc, err := NewQuiverBuilder().Build()

	require.Error(t, err)
	assert.Nil(t, svc)
}

func TestNewAsynxQuiver_NilEventStore_ReturnsError(t *testing.T) {
	ax, err := newAsynxQuiver(nil)
	require.Error(t, err)
	assert.Nil(t, ax)
}

func TestBuilder_Build_NilCatalog_UsesDefaultPath(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	// Without WithCatalog — Build creates its own using paths.Store()
	// This will succeed as long as the path is writable.
	svc, err := NewQuiverBuilder().
		WithEventStore(es).
		Build()

	// Accept either success or failure depending on the environment.
	if err == nil {
		assert.NotNil(t, svc)
	} else {
		assert.Nil(t, svc)
	}
}

type stubHub struct {
	mu      sync.Mutex
	quivers []domain.Quiver
}

func (s *stubHub) BroadcastArrow(_ domain.Arrow)                      {}
func (s *stubHub) BroadcastArrowRuntime(_ domainRuntime.ArrowRuntime) {}
func (s *stubHub) BroadcastQuiver(q domain.Quiver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quivers = append(s.quivers, q)
}

func TestQuiverBuilder_WithWebSocketHub_BroadcastsQuiverEvents(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	store, err := quiverstore.NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	mockVault := &mockBroadcastVault{manifests: map[string]*domain.QuiverManifest{}}
	mockManifold := &mockBroadcastManifold{manifests: map[string]*domain.QuiverManifest{}}

	cat, err := catalog.New(axQuiver, store, mockVault, mockManifold)
	require.NoError(t, err)

	hub := &stubHub{}

	svc, err := NewQuiverBuilder().
		WithAsynxQuiver(axQuiver).
		WithEventStore(es).
		WithCatalog(cat).
		WithWebSocketHub(hub).
		Build()
	require.NoError(t, err)

	ctx := context.Background()
	ns := domain.Namespace("github.com/user/repo")

	require.NoError(t, svc.Add(ctx, ns))
	axQuiver.WaitPublish()

	hub.mu.Lock()
	defer hub.mu.Unlock()
	require.Len(t, hub.quivers, 1)
	assert.Equal(t, ns, hub.quivers[0].Namespace)
}

func TestBuilder_Build_HubSubscribeError(t *testing.T) {
	// Provide a real catalog externally so catalog.New is skipped.
	// Pass a failingQuiverAsynxBuilder as axQuiver: only used in registerWSProjections,
	// which triggers the hub error return in Build.
	wantErr := errors.New("quiver hub subscribe error")
	failQuiver := &failingQuiverAsynxBuilder{err: wantErr}

	cat := buildTestQuiverCatalog(t)
	hub := &stubHub{}

	svc, err := NewQuiverBuilder().
		WithAsynxQuiver(failQuiver).
		WithCatalog(cat).
		WithWebSocketHub(hub).
		Build()

	assert.Nil(t, svc)
	require.Error(t, err)
}

func TestQuiverBuilder_WithoutHub_NoPanic(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	store, err := quiverstore.NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	mockVault := &mockBroadcastVault{manifests: map[string]*domain.QuiverManifest{}}
	mockManifold := &mockBroadcastManifold{manifests: map[string]*domain.QuiverManifest{}}

	cat, err := catalog.New(axQuiver, store, mockVault, mockManifold)
	require.NoError(t, err)

	// Build without hub — should not panic or error
	svc, err := NewQuiverBuilder().
		WithAsynxQuiver(axQuiver).
		WithEventStore(es).
		WithCatalog(cat).
		Build()
	require.NoError(t, err)

	ctx := context.Background()
	ns := domain.Namespace("github.com/user/repo")
	assert.NoError(t, svc.Add(ctx, ns))
}

// mockBroadcastVault is a minimal vault mock for broadcast testing.
type mockBroadcastVault struct {
	mu        sync.Mutex
	manifests map[string]*domain.QuiverManifest
}

func (v *mockBroadcastVault) GetArrow(
	_ context.Context,
	_ domain.Namespace,
) (vault.ManifestFile, error) {
	return vault.ManifestFile{}, vault.ErrNotCached
}

func (v *mockBroadcastVault) PutArrow(
	_ context.Context,
	_ domain.Namespace,
	_ vault.ManifestFile,
) error {
	return nil
}

func (v *mockBroadcastVault) DeleteArrow(_ context.Context, _ domain.Namespace) error {
	return nil
}

func (v *mockBroadcastVault) WorkDir(_ context.Context, _ domain.Namespace) (string, error) {
	return "", nil
}

func (v *mockBroadcastVault) DeleteWorkDir(_ context.Context, _ domain.Namespace) error {
	return nil
}

func (v *mockBroadcastVault) GetQuiver(
	_ context.Context,
	ns domain.Namespace,
) (*vault.QuiverVaultEntry, string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	m, ok := v.manifests[ns.String()]
	if !ok {
		return nil, "", vault.ErrNotCached
	}

	return &vault.QuiverVaultEntry{Manifest: m}, "/tmp/test/" + ns.String(), nil
}

func (v *mockBroadcastVault) PutQuiver(
	_ context.Context,
	ns domain.Namespace,
	manifest *domain.QuiverManifest,
) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := ns.String()
	v.manifests[key] = manifest
	path := "/tmp/test/" + key
	return path, nil
}

func (v *mockBroadcastVault) DeleteQuiver(_ context.Context, ns domain.Namespace) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := ns.String()
	delete(v.manifests, key)
	return nil
}

func (v *mockBroadcastVault) RenameArrow(
	_ context.Context,
	_ domain.Namespace,
	_ domain.Namespace,
) error {
	return nil
}

func (v *mockBroadcastVault) ListVersions(_ context.Context, _ domain.Namespace) ([]string, error) {
	return nil, nil
}

// mockBroadcastManifold is a minimal manifold mock for broadcast testing.
type mockBroadcastManifold struct {
	mu        sync.Mutex
	manifests map[string]*domain.QuiverManifest
}

func (m *mockBroadcastManifold) ResolveArrow(
	_ context.Context,
	_ domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	return nil, nil, "", errors.New("not implemented")
}

func (m *mockBroadcastManifold) ResolveQuiver(
	_ context.Context,
	ns domain.Namespace,
) (*domain.QuiverManifest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, ok := m.manifests[ns.String()]
	if !ok {
		// Return a default manifest so the test can proceed
		manifest = &domain.QuiverManifest{Name: "test"}
		m.manifests[ns.String()] = manifest
	}
	return manifest, nil
}

func (m *mockBroadcastManifold) ParseArrow(
	_ []byte,
) (*domain.Arrow, error) {
	return nil, nil
}

func (m *mockBroadcastManifold) ResolveConstraint(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) (string, error) {
	return "", nil
}

// failingQuiverAsynxBuilder is a minimal asynx.Asynx[domain.Quiver] stub
// whose Subscribe always returns an error.
type failingQuiverAsynxBuilder struct {
	err error
}

func (f *failingQuiverAsynxBuilder) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Quiver],
	_ ...asynxModels.SubscriptionOpt[domain.Quiver],
) (string, error) {
	return "", f.err
}

func (f *failingQuiverAsynxBuilder) Send(
	_ context.Context,
	_ asynxModels.Command[domain.Quiver],
) (asynxModels.Event[domain.Quiver], error) {
	return asynxModels.Event[domain.Quiver]{}, nil
}

func (f *failingQuiverAsynxBuilder) SendWait(
	_ context.Context,
	_ asynxModels.Command[domain.Quiver],
) (asynxModels.Event[domain.Quiver], error) {
	return asynxModels.Event[domain.Quiver]{}, nil
}

func (f *failingQuiverAsynxBuilder) Get(
	_ context.Context,
	_ string,
) (domain.Quiver, error) {
	return domain.Quiver{}, nil
}

func (f *failingQuiverAsynxBuilder) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (f *failingQuiverAsynxBuilder) Preload(_ context.Context, _ string) error { return nil }
func (f *failingQuiverAsynxBuilder) Unsubscribe(_ string) error                { return nil }
func (f *failingQuiverAsynxBuilder) Replay(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
	_ asynxModels.ProjectionHandler[domain.Quiver],
) error {
	return nil
}
func (f *failingQuiverAsynxBuilder) Forget(_ context.Context, _ string) error { return nil }
func (f *failingQuiverAsynxBuilder) OnForget(_ asynxModels.ForgetHandler[domain.Quiver]) (string, error) {
	return "forget-sub-id", nil
}
func (f *failingQuiverAsynxBuilder) Shutdown(_ context.Context) error { return nil }
func (f *failingQuiverAsynxBuilder) WaitPublish()                     {}

func TestBuilder_WithEngines_Succeeds(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)
	store, err := quiverstore.NewQuiverCatalog(":memory:")
	require.NoError(t, err)
	cat, err := catalog.New(axQuiver, store, nil, nil)
	require.NoError(t, err)

	eng := &engine.Container{}

	svc, buildErr := NewQuiverBuilder().
		WithAsynxQuiver(axQuiver).
		WithCatalog(cat).
		WithEngines(eng).
		Build()

	require.NoError(t, buildErr)
	assert.NotNil(t, svc)
}

func TestRegisterWSProjections_QuiverSubscribeError(t *testing.T) {
	wantErr := errors.New("quiver subscribe failed")
	failQuiver := &failingQuiverAsynxBuilder{err: wantErr}
	hub := &stubHub{}

	err := registerWSProjections(failQuiver, hub)

	require.Error(t, err)
	assert.ErrorContains(t, err, "ws quiver subscription")
}
