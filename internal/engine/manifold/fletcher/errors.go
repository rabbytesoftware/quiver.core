package fletcher

import "errors"

const (
	ReasonHostUnsupported = "host_unsupported"
	ReasonNoReleaseAssets = "no_release_assets"
	ReasonNoUsableAsset   = "no_usable_asset"
	ReasonDisabled        = "disabled"
)

var ErrNotFletchable = errors.New("fletcher: not fletchable")

type NotFletchableError struct {
	Reason string
}

func (e NotFletchableError) Error() string {
	return ErrNotFletchable.Error() + ": " + e.Reason
}

func (e NotFletchableError) Unwrap() error {
	return ErrNotFletchable
}
