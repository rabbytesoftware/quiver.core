package ruleset

import "github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"

// ErrInvalidManifest is the sentinel that all RuleErrors unwrap to.
var ErrInvalidManifest = aerrors.ErrInvalidManifest

// ErrNoSupportedPlatform is returned when the manifest has no target that
// compiles for any of the 6 supported OS values.
var ErrNoSupportedPlatform = aerrors.ErrNoSupportedPlatform

// RuleError is a single validation failure with a structured location,
// rule name, and human-readable message.
type RuleError = aerrors.RuleError

// RuleErrors is the collection type returned by ValidateArrow.
// It implements error so it can be used wherever error is expected.
type RuleErrors = aerrors.RuleErrors
