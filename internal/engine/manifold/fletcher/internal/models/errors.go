package models

import "errors"

type Reason string

const (
	ReasonHostUnsupported Reason = "host_unsupported"
	ReasonNoReleaseAssets Reason = "no_release_assets"
	ReasonNoUsableAsset   Reason = "no_usable_asset"
	ReasonNoDigest        Reason = "no_digest"
	ReasonLowConfidence   Reason = "low_confidence"
)

var ErrNotFletchable = errors.New("fletcher: not fletchable")

type NotFletchableError struct {
	Reason Reason
}

func (e NotFletchableError) Error() string {
	return ErrNotFletchable.Error() + ": " + string(e.Reason)
}

func (e NotFletchableError) Unwrap() error {
	return ErrNotFletchable
}
