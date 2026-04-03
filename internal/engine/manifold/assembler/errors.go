package assembler

import "errors"

// ErrInvalidManifest is returned when a raw manifest violates one of the eight
// business rules enforced during assembly.
var ErrInvalidManifest = errors.New("assembler: invalid manifest")

// ErrUnsupportedPlatform is returned when the requested OS is not listed in
// the Arrow's requirements.
var ErrUnsupportedPlatform = errors.New("assembler: unsupported platform")
