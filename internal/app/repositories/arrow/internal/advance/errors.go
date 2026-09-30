package advance

import (
	"errors"
	"fmt"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset"
)

// appSentinels are the app-layer classifications an error may already carry.
// MapResolveErr consults them so a precise classification made downstream is
// not overwritten by this one.
var appSentinels = []error{
	apperrors.ErrNotFound,
	apperrors.ErrAlreadyExists,
	apperrors.ErrStateViolation,
	apperrors.ErrMethodNotFound,
	apperrors.ErrFetchFailed,
	apperrors.ErrInvalidNamespace,
	apperrors.ErrDependentsExist,
	apperrors.ErrInvalidManifest,
	apperrors.ErrPlatformNotSupported,
	apperrors.ErrMissingVariable,
	apperrors.ErrReservedVariable,
	apperrors.ErrInvalidConfig,
}

// MapResolveErr classifies a manifest-resolution failure.
//
// manifold attaches no app sentinel to its own errors, so without this every
// rejected manifest and every unreachable remote arrives at the API carrying
// nothing errors.Is can match — and apierr.StatusAndMessage answers every one
// of them with 500 "internal error", discarding the chain that said what was
// actually wrong.
//
// The remaining default is deliberate: reaching a manifest is I/O against a
// remote, so an unclassified failure there is a gateway problem rather than a
// server fault.
func MapResolveErr(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, ruleset.ErrNoSupportedPlatform):
		return fmt.Errorf("%w: %w", apperrors.ErrPlatformNotSupported, err)
	case errors.Is(err, ruleset.ErrInvalidManifest):
		return fmt.Errorf("%w: %w", apperrors.ErrInvalidManifest, err)
	}

	for _, sentinel := range appSentinels {
		if errors.Is(err, sentinel) {
			return err
		}
	}

	return fmt.Errorf("%w: %w", apperrors.ErrFetchFailed, err)
}
