package storage

import (
	"context"
	"encoding/json"
	"fmt"

	gormdb "gorm.io/gorm"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
)

type Store interface {
	Save(
		ctx context.Context,
		vm ViewModel,
	) error
	Delete(
		ctx context.Context,
		ns string,
	) error
	FindByKey(
		ctx context.Context,
		ns string,
	) (*ViewModel, error)
	FindAll(
		ctx context.Context,
	) ([]ViewModel, error)
}

type blobRow struct {
	Namespace string `gorm:"primaryKey;column:namespace"`
	Data      []byte `gorm:"column:data"`
}

func (blobRow) TableName() string { return "catalog_arrows" }

type storageService struct {
	db interface {
		Save(ctx context.Context, row blobRow) error
		Delete(ctx context.Context, key string) error
		FindByKey(ctx context.Context, key string) (*blobRow, error)
		FindAll(ctx context.Context) ([]blobRow, error)
	}
}

func New(
	db *gormdb.DB,
) (Store, error) {
	if err := newSchema(db); err != nil {
		return nil, err
	}
	inner, err := adapterSQLite.NewFromDB[blobRow, string](db)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	return &storageService{db: inner}, nil
}

func (s *storageService) Save(
	ctx context.Context,
	vm ViewModel,
) error {
	data, err := json.Marshal(vm)
	if err != nil {
		return fmt.Errorf("storage: marshal: %w", err)
	}
	return s.db.Save(ctx, blobRow{Namespace: vm.Namespace.String(), Data: data})
}

func (s *storageService) Delete(
	ctx context.Context,
	ns string,
) error {
	return s.db.Delete(ctx, ns)
}

func (s *storageService) FindByKey(
	ctx context.Context,
	ns string,
) (*ViewModel, error) {
	row, err := s.db.FindByKey(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("storage: find: %w", err)
	}
	if row == nil {
		return nil, nil
	}
	return unmarshal(row.Data)
}

func (s *storageService) FindAll(
	ctx context.Context,
) ([]ViewModel, error) {
	rows, err := s.db.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage: find all: %w", err)
	}
	result := make([]ViewModel, 0, len(rows))
	for _, row := range rows {
		vm, err := unmarshal(row.Data)
		if err != nil {
			return nil, err
		}
		result = append(result, *vm)
	}
	return result, nil
}

func unmarshal(
	data []byte,
) (*ViewModel, error) {
	var vm ViewModel
	if err := json.Unmarshal(data, &vm); err != nil {
		return nil, fmt.Errorf("storage: unmarshal: %w", err)
	}
	return &vm, nil
}
