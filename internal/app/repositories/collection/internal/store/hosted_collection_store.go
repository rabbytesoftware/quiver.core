package store

import (
	"context"
	"encoding/json"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"gorm.io/gorm"

	eventstore "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type hostedCollectionStore struct {
	*collectionStore
	host *gorm.DB
}

// NewHosted returns a store whose table lives in the collection aggregate's
// events database, so SaveTx commits with the event it projects. Rows a
// previous release kept in its own file at legacyPath are copied in on first
// use and the file is kept beside it as legacyPath+".migrated". The store does
// not own host and Close leaves it open.
func NewHosted(
	host *gorm.DB,
	legacyPath string,
) (QuiverStore, error) {
	inner, err := sqlite.NewFromDB[collectionRow, string](host)
	if err != nil {
		return nil, fmt.Errorf("collection store: %w", err)
	}
	_, err = sqlite.ImportLegacyFile[collectionRow](host, legacyPath)
	if err != nil {
		return nil, fmt.Errorf("collection store: import legacy rows: %w", err)
	}
	return &hostedCollectionStore{
		collectionStore: &collectionStore{inner: inner},
		host:            host,
	}, nil
}

func (s *hostedCollectionStore) SaveTx(
	ctx context.Context,
	tx asynxModels.Tx,
	coll domain.Collection,
) error {
	data, err := json.Marshal(coll)
	if err != nil {
		return fmt.Errorf("collection store save: marshal: %w", err)
	}
	session, err := eventstore.TxSession(ctx, s.host, tx)
	if err != nil {
		return fmt.Errorf("collection store save: %w", err)
	}
	row := collectionRow{Namespace: coll.Namespace.String(), Data: string(data)}
	err = session.Save(&row).Error
	if err != nil {
		return fmt.Errorf("collection store save: %w", err)
	}
	return nil
}

func (s *hostedCollectionStore) Close() error {
	return nil
}
