package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

type Vault struct {
	GetArrowFile     vault.ManifestFile
	GetArrowErr      error
	PutArrowErr      error
	PutArrowCalls    int
	DeleteArrowErr   error
	DeleteArrowCalls int
	RenameArrowErr   error
	ListVersionsResp []string
	ListVersionsErr  error

	WorkDirValue string
	WorkDirErr   error

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
}

func (m *Vault) GetArrow(
	_ context.Context,
	_ domain.Namespace,
) (vault.ManifestFile, error) {
	return m.GetArrowFile, m.GetArrowErr
}

func (m *Vault) PutArrow(
	_ context.Context,
	_ domain.Namespace,
	_ vault.ManifestFile,
) error {
	m.PutArrowCalls++
	return m.PutArrowErr
}

func (m *Vault) DeleteArrow(
	_ context.Context,
	_ domain.Namespace,
) error {
	m.DeleteArrowCalls++
	return m.DeleteArrowErr
}

func (m *Vault) RenameArrow(
	_ context.Context,
	_ domain.Namespace,
	_ domain.Namespace,
) error {
	return m.RenameArrowErr
}

func (m *Vault) ListVersions(
	_ context.Context,
	_ domain.Namespace,
) ([]string, error) {
	return m.ListVersionsResp, m.ListVersionsErr
}

func (m *Vault) WorkDir(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
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
