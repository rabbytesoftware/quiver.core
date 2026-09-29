package fletcher

import "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/models"

type (
	NotFletchableError = models.NotFletchableError
	Reason             = models.Reason
)

const (
	ReasonHostUnsupported = models.ReasonHostUnsupported
	ReasonNoReleaseAssets = models.ReasonNoReleaseAssets
	ReasonNoUsableAsset   = models.ReasonNoUsableAsset
	ReasonNoDigest        = models.ReasonNoDigest
	ReasonLowConfidence   = models.ReasonLowConfidence
)

var ErrNotFletchable = models.ErrNotFletchable
