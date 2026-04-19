package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

type Vault struct {
	GetArrowEntry    *vault.VaultEntry
	GetArrowPath     string
	GetArrowErr      error
	PutArrowPath     string
	PutArrowErr      error
	PutArrowCalls    int
	DeleteArrowErr   error
	DeleteArrowCalls int
	ListVersionsResp []string
	ListVersionsErr  error

	GetQuiverEntry  *vault.QuiverVaultEntry
	GetQuiverPath   string
	GetQuiverErr    error
	PutQuiverPath   string
	PutQuiverErr    error
	PutQuiverCalls  int
	DeleteQuiverErr error
}

func (m *Vault) GetArrow(_ context.Context, _ domain.Namespace) (*vault.VaultEntry, string, error) {
	return m.GetArrowEntry, m.GetArrowPath, m.GetArrowErr
}

func (m *Vault) PutArrow(_ context.Context, _ domain.Namespace, _ *domain.Arrow) (string, error) {
	m.PutArrowCalls++
	return m.PutArrowPath, m.PutArrowErr
}

func (m *Vault) DeleteArrow(
	_ context.Context,
	_ domain.Namespace,
) error {
	m.DeleteArrowCalls++
	return m.DeleteArrowErr
}

func (m *Vault) ListVersions(_ context.Context, _ domain.Namespace) ([]string, error) {
	return m.ListVersionsResp, m.ListVersionsErr
}

func (m *Vault) GetQuiver(_ context.Context, _ domain.Namespace) (*vault.QuiverVaultEntry, string, error) {
	return m.GetQuiverEntry, m.GetQuiverPath, m.GetQuiverErr
}

func (m *Vault) PutQuiver(_ context.Context, _ domain.Namespace, _ *domain.QuiverManifest) (string, error) {
	m.PutQuiverCalls++
	return m.PutQuiverPath, m.PutQuiverErr
}

func (m *Vault) DeleteQuiver(_ context.Context, _ domain.Namespace) error {
	return m.DeleteQuiverErr
}
