package assembler

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidManifest is the sentinel that all AssemblerErrors unwrap to.
var ErrInvalidManifest = errors.New("assembler: invalid manifest")

// ErrUnsupportedPlatform is returned when the requested OS is not listed in
// the Arrow's requirements.
var ErrUnsupportedPlatform = errors.New("assembler: unsupported platform")

// AssemblerError is a single validation failure with a structured location,
// rule name, and human-readable message.
type AssemblerError struct {
	Field   string // YAML path, e.g. "lifecycle.install", "variables[1].min"
	Rule    string // machine-readable rule ID, e.g. "missing_pair", "invalid_range"
	Message string // human-readable description
}

func (e AssemblerError) Error() string {
	return fmt.Sprintf("%s: %s [%s]: %s", ErrInvalidManifest, e.Field, e.Rule, e.Message)
}

// Unwrap lets errors.Is(err, ErrInvalidManifest) work on any AssemblerError.
func (e AssemblerError) Unwrap() error {
	return ErrInvalidManifest
}

// AssemblerErrors is the collection type returned by ValidateArrow.
// It implements error so it can be used wherever error is expected.
type AssemblerErrors []AssemblerError

func (e AssemblerErrors) Error() string {
	msgs := make([]string, len(e))
	for i, ae := range e {
		msgs[i] = ae.Error()
	}
	return strings.Join(msgs, "; ")
}

// Unwrap implements the multi-error interface so errors.Is walks each entry.
func (e AssemblerErrors) Unwrap() []error {
	errs := make([]error, len(e))
	for i, ae := range e {
		errs[i] = ae
	}
	return errs
}
