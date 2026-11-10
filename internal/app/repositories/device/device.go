package device

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/rabbytesoftware/quiver.core/internal/app/bind"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	devicecmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/device/internal/commands"
	devicestore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/device/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// Device manages paired quiver.desktop clients: their session credential,
// last-seen bookkeeping, and revocation.
type Device interface {
	// Pair (re)activates a device with tokenHash as its credential. Called
	// once a pairing code has been claimed.
	Pair(
		ctx context.Context,
		deviceID string,
		label string,
		tokenHash string,
	) error
	Revoke(
		ctx context.Context,
		deviceID string,
	) error
	// Touch stamps LastSeenAt. Callers run this in the background — it is
	// bookkeeping, not a gate on the request that triggered it.
	Touch(
		ctx context.Context,
		deviceID string,
	) error
	// Authenticate looks a device up by its bearer token's hash. It succeeds
	// only for an active device: a revoked device's row still exists, but its
	// hash no longer authenticates anything.
	Authenticate(
		ctx context.Context,
		tokenHash string,
	) (auth.Device, error)
	Get(
		ctx context.Context,
		deviceID string,
	) (auth.Device, error)
	List(
		ctx context.Context,
	) ([]auth.Device, error)

	Shutdown(ctx context.Context) error
}

type deviceRepo struct {
	ax    asynx.Asynx[auth.Device]
	store devicestore.Store
}

// New constructs a Device repository backed by its own Asynx instance and a
// GORM read model keyed by token hash.
func New(
	db *gormdb.DB,
	ax asynx.Asynx[auth.Device],
) (Device, error) {
	if ax == nil {
		return nil, fmt.Errorf("device repository: asynx instance is required")
	}

	st, err := openStore(db, ax)
	if err != nil {
		return nil, fmt.Errorf("device repository: store: %w", err)
	}

	repo := &deviceRepo{ax: ax, store: st}
	if err := repo.registerProjections(); err != nil {
		return nil, fmt.Errorf("device repository: register projections: %w", err)
	}

	return repo, nil
}

func openStore(
	db *gormdb.DB,
	ax asynx.Asynx[auth.Device],
) (devicestore.Store, error) {
	host := bind.Host(ax)
	if host == nil {
		return devicestore.New(db)
	}
	return devicestore.NewHosted(host, db)
}

func (r *deviceRepo) registerProjections() error {
	topics := []string{
		"auth.device.paired.*",
		"auth.device.revoked.*",
		"auth.device.touched.*",
	}

	for _, topic := range topics {
		if err := r.registerTopic(topic); err != nil {
			return err
		}
	}

	return nil
}

func (r *deviceRepo) registerTopic(
	topic string,
) error {
	synced, err := r.registerSyncTopic(topic)
	if err != nil || synced {
		return err
	}
	projector := devicestore.NewProjector(r.store)
	return bind.OnEvent(r.ax, topic, loggedProjection{projector: projector}.apply)
}

func (r *deviceRepo) registerSyncTopic(
	topic string,
) (bool, error) {
	txStore, ok := r.store.(devicestore.TxStore)
	if !ok {
		return false, nil
	}
	upsert := func(ctx context.Context, tx asynxModels.Tx, evt asynxModels.Event[auth.Device]) error {
		return txStore.UpsertTx(ctx, tx, evt.Aggregate)
	}
	return bind.Projection(r.ax, topic, upsert)
}

func (r *deviceRepo) Pair(
	ctx context.Context,
	deviceID string,
	label string,
	tokenHash string,
) error {
	_, err := r.ax.SendWait(ctx, devicecmds.Pair{
		DeviceID:  deviceID,
		Label:     label,
		TokenHash: tokenHash,
		Now:       time.Now(),
	})
	if err != nil {
		return fmt.Errorf("pair device: %w", apperrors.ErrStateViolation)
	}

	return nil
}

func (r *deviceRepo) Revoke(
	ctx context.Context,
	deviceID string,
) error {
	_, err := r.ax.SendWait(ctx, devicecmds.Revoke{DeviceID: deviceID})
	if err != nil {
		return fmt.Errorf("revoke device: %w", apperrors.ErrNotFound)
	}

	return nil
}

func (r *deviceRepo) Touch(
	ctx context.Context,
	deviceID string,
) error {
	_, err := r.ax.SendWait(ctx, devicecmds.Touch{DeviceID: deviceID, Now: time.Now()})
	if err != nil {
		return fmt.Errorf("touch device: %w", apperrors.ErrNotFound)
	}

	return nil
}

func (r *deviceRepo) Authenticate(
	ctx context.Context,
	tokenHash string,
) (auth.Device, error) {
	d, err := r.store.GetByTokenHash(ctx, tokenHash)
	if err != nil || !d.IsActive() {
		return auth.Device{}, fmt.Errorf("authenticate: %w", apperrors.ErrUnauthorized)
	}

	return d, nil
}

func (r *deviceRepo) Get(
	ctx context.Context,
	deviceID string,
) (auth.Device, error) {
	d, err := r.store.Get(ctx, deviceID)
	if err != nil {
		return auth.Device{}, fmt.Errorf("get device: %w", err)
	}

	return d, nil
}

func (r *deviceRepo) List(
	ctx context.Context,
) ([]auth.Device, error) {
	devices, err := r.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}

	return devices, nil
}

func (r *deviceRepo) Shutdown(
	ctx context.Context,
) error {
	return r.ax.Shutdown(ctx)
}
