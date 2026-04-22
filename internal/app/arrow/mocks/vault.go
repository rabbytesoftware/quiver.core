package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

type Vault struct {
	GetArrowEntry  *vault.VaultEntry
	GetArrowPath   string
	GetArrowErr    error
	PutArrowPath   string
	PutArrowErr    error
	DeleteArrowErr error
	RenameArrowErr error
}

func (m *Vault) GetArrow(ctx context.Context, ns domain.Namespace) (*vault.VaultEntry, string, error) {
	return m.GetArrowEntry, m.GetArrowPath, m.GetArrowErr
}

func (m *Vault) GetQuiver(ctx context.Context, ns domain.Namespace) (*vault.QuiverVaultEntry, string, error) {
	return nil, "", nil
}

func (m *Vault) PutArrow(ctx context.Context, ns domain.Namespace, manifest *domain.Arrow) (string, error) {
	return m.PutArrowPath, m.PutArrowErr
}

func (m *Vault) PutQuiver(ctx context.Context, ns domain.Namespace, manifest *domain.QuiverManifest) (string, error) {
	return "", nil
}

func (m *Vault) DeleteArrow(ctx context.Context, ns domain.Namespace) error {
	return m.DeleteArrowErr
}

func (m *Vault) DeleteQuiver(ctx context.Context, ns domain.Namespace) error {
	return nil
}

func (m *Vault) RenameArrow(ctx context.Context, oldNs, newNs domain.Namespace) error {
	return m.RenameArrowErr
}

func (m *Vault) ListVersions(ctx context.Context, ns domain.Namespace) ([]string, error) {
	return nil, nil
}
