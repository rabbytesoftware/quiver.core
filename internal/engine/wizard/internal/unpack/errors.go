package unpack

import "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"

var (
	ErrEscape              = models.ErrEscape
	ErrTooLarge            = models.ErrTooLarge
	ErrTooMany             = models.ErrTooMany
	ErrUnknownFormat       = models.ErrUnknownFormat
	ErrUnsupportedAppImage = models.ErrUnsupportedAppImage
	ErrNoSquashfs          = models.ErrNoSquashfs
	ErrReservedName        = models.ErrReservedName
	ErrNameCollision       = models.ErrNameCollision
	ErrUnsupportedPlatform = models.ErrUnsupportedPlatform
	ErrMsiexecFailed       = models.ErrMsiexecFailed
)
