package store

import (
	"context"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"gorm.io/gorm"

	eventstore "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
)

type hostedPortStore struct {
	portStore
	host *gorm.DB
}

// NewPortHosted keeps the port read model in host, the port aggregate's events
// database. Allocations an earlier release kept in its own file at legacyPath
// are copied in on first use and the file is kept beside it as
// legacyPath+".migrated". Close leaves host open.
func NewPortHosted(
	host *gorm.DB,
	legacyPath string,
) (HostedPortStore, error) {
	inner, err := adapterSQLite.NewFromDB[ports.PortAllocation, int](host)
	if err != nil {
		return nil, fmt.Errorf("port hosted: %w", err)
	}
	_, err = adapterSQLite.ImportLegacyFile[ports.PortAllocation](host, legacyPath)
	if err != nil {
		return nil, fmt.Errorf("port hosted: import legacy rows: %w", err)
	}
	return &hostedPortStore{portStore: portStore{inner: inner}, host: host}, nil
}

func (s *hostedPortStore) SaveTx(
	ctx context.Context,
	tx asynxModels.Tx,
	alloc ports.PortAllocation,
) error {
	session, err := eventstore.TxSession(ctx, s.host, tx)
	if err != nil {
		return fmt.Errorf("port hosted: save: %w", err)
	}
	err = session.Save(&alloc).Error
	if err != nil {
		return fmt.Errorf("port hosted: save: %w", err)
	}
	return nil
}

func (s *hostedPortStore) DeleteTx(
	ctx context.Context,
	tx asynxModels.Tx,
	port int,
) error {
	session, err := eventstore.TxSession(ctx, s.host, tx)
	if err != nil {
		return fmt.Errorf("port hosted: delete: %w", err)
	}
	err = session.Where("port = ?", port).Delete(&ports.PortAllocation{}).Error
	if err != nil {
		return fmt.Errorf("port hosted: delete: %w", err)
	}
	return nil
}
