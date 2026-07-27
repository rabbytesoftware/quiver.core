package projections_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store/internal/projections"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store/internal/storage"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// ─── stub store ───────────────────────────────────────────────────────────────

// stubStore forwards to a real storage.Store unless a failure is configured for
// the call under test.
type stubStore struct {
	real storage.Store

	saveErr        error
	saveVersionErr error
	deleteErr      error
	findErr        error

	deleted []string
	saved   []storage.ViewModel
}

func (s *stubStore) Save(
	ctx context.Context,
	vm storage.ViewModel,
) error {
	s.saved = append(s.saved, vm)
	if s.saveErr != nil {
		return s.saveErr
	}
	if s.real != nil {
		return s.real.Save(ctx, vm)
	}
	return nil
}

func (s *stubStore) SaveVersion(
	ctx context.Context,
	ns domain.Namespace,
	arrow domain.Arrow,
) error {
	if s.saveVersionErr != nil {
		return s.saveVersionErr
	}
	if s.real != nil {
		return s.real.SaveVersion(ctx, ns, arrow)
	}
	return nil
}

func (s *stubStore) Delete(
	ctx context.Context,
	ns string,
) error {
	s.deleted = append(s.deleted, ns)
	if s.deleteErr != nil {
		return s.deleteErr
	}
	if s.real != nil {
		return s.real.Delete(ctx, ns)
	}
	return nil
}

func (s *stubStore) FindByKey(
	ctx context.Context,
	ns string,
) (*storage.ViewModel, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	if s.real != nil {
		return s.real.FindByKey(ctx, ns)
	}
	return nil, nil
}

func (s *stubStore) FindAll(
	ctx context.Context,
) ([]storage.ViewModel, error) {
	if s.real != nil {
		return s.real.FindAll(ctx)
	}
	return nil, nil
}

func (s *stubStore) Search(
	ctx context.Context,
	q storage.Query,
) ([]storage.ViewModel, error) {
	if s.real != nil {
		return s.real.Search(ctx, q)
	}
	return nil, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func newRealStore(t *testing.T) storage.Store {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	s, err := storage.New(db)
	require.NoError(t, err)
	return s
}

func arrowAt(ns string) domain.Arrow {
	return domain.Arrow{
		Namespace: domain.Namespace(ns),
		ArrowMeta: domain.ArrowMeta{Name: "pkg"},
	}
}

// ─── Apply ───────────────────────────────────────────────────────────────────

func TestProjector_Apply_WritesVersionRow(t *testing.T) {
	real := newRealStore(t)
	p := projections.New(real)

	arrow := arrowAt("github.com/user/pkg@v1.0.0")
	require.NoError(t, p.Apply(context.Background(), arrow))

	vm, err := real.FindByKey(context.Background(), "github.com/user/pkg")
	require.NoError(t, err)
	require.NotNil(t, vm)
	assert.Equal(t, "pkg", vm.Metadata.Name)
	assert.Len(t, vm.Versions, 1)
}

func TestProjector_Apply_SaveVersionErrorIsReturned(t *testing.T) {
	boom := errors.New("disk full")
	p := projections.New(&stubStore{saveVersionErr: boom})

	err := p.Apply(context.Background(), arrowAt("github.com/user/pkg@v1.0.0"))

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

// ─── Forget ──────────────────────────────────────────────────────────────────

func TestProjector_Forget_LastVersionDeletesNamespace(t *testing.T) {
	real := newRealStore(t)
	stub := &stubStore{real: real}
	p := projections.New(stub)

	arrow := arrowAt("github.com/user/pkg@v1.0.0")
	require.NoError(t, p.Apply(context.Background(), arrow))

	require.NoError(t, p.Forget(context.Background(), arrow))

	assert.Equal(t, []string{"github.com/user/pkg"}, stub.deleted)

	vm, err := real.FindByKey(context.Background(), "github.com/user/pkg")
	require.NoError(t, err)
	assert.Nil(t, vm)
}

func TestProjector_Forget_KeepsRemainingVersions(t *testing.T) {
	real := newRealStore(t)
	stub := &stubStore{real: real}
	p := projections.New(stub)

	v1 := arrowAt("github.com/user/pkg@v1.0.0")
	v2 := arrowAt("github.com/user/pkg@v2.0.0")
	require.NoError(t, p.Apply(context.Background(), v1))
	require.NoError(t, p.Apply(context.Background(), v2))

	require.NoError(t, p.Forget(context.Background(), v1))

	assert.Empty(t, stub.deleted)

	vm, err := real.FindByKey(context.Background(), "github.com/user/pkg")
	require.NoError(t, err)
	require.NotNil(t, vm)
	require.Len(t, vm.Versions, 1)
	assert.Equal(t, v2.Namespace, vm.Versions[0].Namespace)
}

func TestProjector_Forget_UnknownNamespaceIsNoOp(t *testing.T) {
	p := projections.New(&stubStore{})

	err := p.Forget(context.Background(), arrowAt("github.com/user/pkg@v1.0.0"))

	require.NoError(t, err)
}

// A read that failed cannot tell us whether anything is left, so the projector
// declines to guess rather than deleting a namespace that may still have
// versions.
func TestProjector_Forget_FindErrorLeavesReadModelAlone(t *testing.T) {
	stub := &stubStore{findErr: errors.New("db closed")}
	p := projections.New(stub)

	err := p.Forget(context.Background(), arrowAt("github.com/user/pkg@v1.0.0"))

	require.NoError(t, err)
	assert.Empty(t, stub.deleted)
	assert.Empty(t, stub.saved)
}

func TestProjector_Forget_DeleteErrorIsReturned(t *testing.T) {
	real := newRealStore(t)
	stub := &stubStore{real: real}
	p := projections.New(stub)

	arrow := arrowAt("github.com/user/pkg@v1.0.0")
	require.NoError(t, p.Apply(context.Background(), arrow))

	boom := errors.New("delete failed")
	stub.deleteErr = boom

	err := p.Forget(context.Background(), arrow)

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestProjector_Forget_SaveErrorIsReturned(t *testing.T) {
	real := newRealStore(t)
	stub := &stubStore{real: real}
	p := projections.New(stub)

	v1 := arrowAt("github.com/user/pkg@v1.0.0")
	v2 := arrowAt("github.com/user/pkg@v2.0.0")
	require.NoError(t, p.Apply(context.Background(), v1))
	require.NoError(t, p.Apply(context.Background(), v2))

	boom := errors.New("save failed")
	stub.saveErr = boom

	err := p.Forget(context.Background(), v1)

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}
