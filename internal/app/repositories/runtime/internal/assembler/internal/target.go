package assemblerinternal

import (
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func ResolveTarget(
	arrow *domain.Arrow,
	os domain.OS,
) (domain.Target, error) {
	target, ok := arrow.Targets[os]
	if !ok {
		return domain.Target{}, apperrors.ErrPlatformNotSupported
	}
	return target, nil
}
