package usecases

import (
	"context"
	"errors"
	"fmt"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// Launcher starts an installed arrow's desktop app. ns is always the
// ref-qualified namespace of an installed version.
type Launcher interface {
	Launchable(
		ctx context.Context,
		ns domain.Namespace,
	) bool
	Launch(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

type wizardLauncher struct {
	vault  vault.Vault
	wizard wizardPkg.Wizard
}

func NewLauncher(
	v vault.Vault,
	w wizardPkg.Wizard,
) Launcher {
	return &wizardLauncher{vault: v, wizard: w}
}

func (l *wizardLauncher) Launchable(
	ctx context.Context,
	ns domain.Namespace,
) bool {
	workdir, err := l.vault.WorkDir(ctx, ns)
	if err != nil {
		return false
	}
	return l.wizard.Launchable(ctx, workdir)
}

func (l *wizardLauncher) Launch(
	ctx context.Context,
	ns domain.Namespace,
) error {
	workdir, err := l.vault.WorkDir(ctx, ns)
	if err != nil {
		return fmt.Errorf("launch %s: %w", ns, err)
	}
	err = l.wizard.Launch(ctx, workdir)
	if errors.Is(err, wizardPkg.ErrNotLaunchable) {
		return fmt.Errorf("launch %s: %w", ns, apperrors.ErrNotOpenable)
	}
	return err
}
