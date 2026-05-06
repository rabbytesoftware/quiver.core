package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
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

	GetQuiverEntry  *vault.QuiverVaultEntry
	GetQuiverPath   string
	GetQuiverErr    error
	PutQuiverPath   string
	PutQuiverErr    error
	PutQuiverCalls  int
	DeleteQuiverErr error

	ListCachedQuiversResult []domain.Namespace
	ListCachedQuiversErr    error
	ListCachedQuiversCalls  int
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

func (m *Vault) GetQuiver(
	_ context.Context,
	_ domain.Namespace,
) (*vault.QuiverVaultEntry, string, error) {
	return m.GetQuiverEntry, m.GetQuiverPath, m.GetQuiverErr
}

func (m *Vault) PutQuiver(
	_ context.Context,
	_ domain.Namespace,
	_ *domain.Quiver,
) (string, error) {
	m.PutQuiverCalls++
	return m.PutQuiverPath, m.PutQuiverErr
}

func (m *Vault) ListCachedQuivers(_ context.Context) ([]domain.Namespace, error) {
	m.ListCachedQuiversCalls++
	return m.ListCachedQuiversResult, m.ListCachedQuiversErr
}

func (m *Vault) DeleteQuiver(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.DeleteQuiverErr
}

func (m *Vault) Start(_ context.Context) {}
