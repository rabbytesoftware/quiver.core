package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

type Vault struct {
	GetArrowFile  vault.ManifestFile
	GetArrowErr   error
	PutArrowErr   error
	PutArrowCalls int
	// PutArrowFiles records what was actually cached. A caller that omits Meta
	// writes a manifest the vault lane of search can never answer with, and a
	// call count alone cannot tell that apart from a correct write.
	PutArrowFiles []vault.ManifestFile
	// ArrowOps records every DeleteArrow and PutArrow as "delete <ns>" or
	// "put <ns>", in call order, so a test can assert a cache was replaced
	// for the right namespace and in the right order.
	ArrowOps []string

	PutArrowNotFoundErr   error
	PutArrowNotFoundCalls int
	// PutArrowNotFoundNamespaces records which namespaces were marked
	// confirmed-absent, so a test can assert the right one without a call
	// count alone standing in for it.
	PutArrowNotFoundNamespaces []domain.Namespace
	PutArrowNotFoundCommits    []string

	DeleteArrowErr   error
	DeleteArrowCalls int
	ListVersionsResp []string
	ListVersionsErr  error

	WorkDirValue      string
	WorkDirErr        error
	WorkDirNamespaces []domain.Namespace

	// GetRefsEntry is what GetRefs returns; nil means no ref list was saved.
	GetRefsEntry *vault.RefsEntry
	GetRefsErr   error
	PutRefsCalls int
	PutRefsErr   error

	GetCollectionEntry  *vault.CollectionVaultEntry
	GetCollectionPath   string
	GetCollectionErr    error
	PutCollectionPath   string
	PutCollectionErr    error
	PutCollectionCalls  int
	DeleteCollectionErr error

	ListCachedCollectionsResult []domain.Namespace
	ListCachedCollectionsErr    error
	ListCachedCollectionsCalls  int

	SearchArrowsResult []vault.IndexRow
	SearchArrowsErr    error
	SearchArrowsQuery  vault.IndexQuery
	ForgetArrowErr     error
	ForgetArrowCalls   int

	CloseErr   error
	CloseCalls int
}

func (m *Vault) GetArrow(
	_ context.Context,
	_ domain.Namespace,
) (vault.ManifestFile, error) {
	return m.GetArrowFile, m.GetArrowErr
}

func (m *Vault) GetRefs(
	_ context.Context,
	_ domain.Namespace,
) (vault.RefsEntry, error) {
	if m.GetRefsErr != nil {
		return vault.RefsEntry{}, m.GetRefsErr
	}
	if m.GetRefsEntry == nil {
		return vault.RefsEntry{}, vault.ErrNotCached
	}
	return *m.GetRefsEntry, nil
}

func (m *Vault) PutRefs(
	_ context.Context,
	_ domain.Namespace,
	snap domain.RefSnapshot,
) error {
	m.PutRefsCalls++
	m.GetRefsEntry = &vault.RefsEntry{Snapshot: snap}
	return m.PutRefsErr
}

func (m *Vault) PutArrow(
	_ context.Context,
	ns domain.Namespace,
	file vault.ManifestFile,
) error {
	m.PutArrowCalls++
	m.ArrowOps = append(m.ArrowOps, "put "+ns.String())
	m.PutArrowFiles = append(m.PutArrowFiles, file)
	return m.PutArrowErr
}

func (m *Vault) PutArrowNotFound(
	_ context.Context,
	ns domain.Namespace,
	commit string,
) error {
	m.PutArrowNotFoundCalls++
	m.PutArrowNotFoundNamespaces = append(m.PutArrowNotFoundNamespaces, ns)
	m.PutArrowNotFoundCommits = append(m.PutArrowNotFoundCommits, commit)
	return m.PutArrowNotFoundErr
}

func (m *Vault) DeleteArrow(
	_ context.Context,
	ns domain.Namespace,
) error {
	m.DeleteArrowCalls++
	m.ArrowOps = append(m.ArrowOps, "delete "+ns.String())
	return m.DeleteArrowErr
}

func (m *Vault) ListVersions(
	_ context.Context,
	_ domain.Namespace,
) ([]string, error) {
	return m.ListVersionsResp, m.ListVersionsErr
}

func (m *Vault) WorkDir(
	_ context.Context,
	ns domain.Namespace,
) (string, error) {
	m.WorkDirNamespaces = append(m.WorkDirNamespaces, ns)
	return m.WorkDirValue, m.WorkDirErr
}

func (m *Vault) DeleteWorkDir(
	_ context.Context,
	_ domain.Namespace,
) error {
	return nil
}

func (m *Vault) GetCollection(
	_ context.Context,
	_ domain.Namespace,
) (*vault.CollectionVaultEntry, string, error) {
	return m.GetCollectionEntry, m.GetCollectionPath, m.GetCollectionErr
}

func (m *Vault) PutCollection(
	_ context.Context,
	_ domain.Namespace,
	_ *domain.Collection,
) (string, error) {
	m.PutCollectionCalls++
	return m.PutCollectionPath, m.PutCollectionErr
}

func (m *Vault) ListCachedCollections(_ context.Context) ([]domain.Namespace, error) {
	m.ListCachedCollectionsCalls++
	return m.ListCachedCollectionsResult, m.ListCachedCollectionsErr
}

func (m *Vault) DeleteCollection(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.DeleteCollectionErr
}

func (m *Vault) SearchArrows(
	_ context.Context,
	q vault.IndexQuery,
) ([]vault.IndexRow, error) {
	m.SearchArrowsQuery = q
	return m.SearchArrowsResult, m.SearchArrowsErr
}

func (m *Vault) ForgetArrow(
	_ context.Context,
	_ domain.Namespace,
) error {
	m.ForgetArrowCalls++
	return m.ForgetArrowErr
}

func (m *Vault) Start(_ context.Context) {}

func (m *Vault) Close() error {
	m.CloseCalls++
	return m.CloseErr
}
