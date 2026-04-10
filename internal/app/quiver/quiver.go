package quiver

import (
	"context"
	"fmt"

	apphub "github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

// QuiverService manages quiver lifecycle: registration, manifest updates, and removal.
type QuiverService interface {
	Add(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Update(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error
	List(ctx context.Context) ([]QuiverListDTO, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*QuiverDetailDTO, error)
}

type quiverService struct {
	catalog catalog.Catalog
	hub     apphub.WebSocketHub
}

func (svc *quiverService) broadcastQuiver(ctx context.Context, ns domain.Namespace) {
	if svc.hub == nil {
		return
	}
	q, err := svc.catalog.Get(ctx, ns)
	if err != nil || q == nil {
		return
	}
	svc.hub.BroadcastQuiver(domain.Quiver{
		Namespace: q.Namespace,
		Manifest:  q.Manifest,
		Removed:   q.Removed,
	})
}

func (svc *quiverService) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := svc.catalog.Add(ctx, ns); err != nil {
		return err
	}
	svc.broadcastQuiver(ctx, ns)
	return nil
}

func (svc *quiverService) Update(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := svc.catalog.Update(ctx, ns); err != nil {
		return err
	}
	svc.broadcastQuiver(ctx, ns)
	return nil
}

func (svc *quiverService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := svc.catalog.Remove(ctx, ns); err != nil {
		return err
	}
	svc.broadcastQuiver(ctx, ns)
	return nil
}

func (svc *quiverService) List(ctx context.Context) ([]QuiverListDTO, error) {
	quivers, err := svc.catalog.List(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]QuiverListDTO, 0, len(quivers))
	for _, q := range quivers {
		result = append(result, QuiverListDTO{
			Namespace:   q.Namespace,
			Name:        q.Manifest.Name,
			Description: q.Manifest.Description,
			Tags:        q.Manifest.Tags,
			Removed:     q.Removed,
		})
	}

	return result, nil
}

func (svc *quiverService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*QuiverDetailDTO, error) {
	q, err := svc.catalog.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}

	return &QuiverDetailDTO{
		Namespace: q.Namespace,
		Manifest:  q.Manifest,
		Removed:   q.Removed,
	}, nil
}
