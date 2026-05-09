package aerrors

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidManifest = errors.New("ruleset: invalid manifest")

var ErrNoSupportedPlatform = errors.New("ruleset: no supported platform")

// RuleError is a single validation failure with a structured location,
// rule name, and human-readable message.
type RuleError struct {
	Field   string // YAML path, e.g. "targets[linux/*].lifecycle.install"
	Rule    string // machine-readable rule ID, e.g. "missing_pair", "invalid_range"
	Message string // human-readable description
}

func (e RuleError) Error() string {
	return fmt.Sprintf("%s: %s [%s]: %s", ErrInvalidManifest, e.Field, e.Rule, e.Message)
}

func (e RuleError) Unwrap() error {
	return ErrInvalidManifest
}

type RuleErrors []RuleError

func (e RuleErrors) Error() string {
	msgs := make([]string, len(e))
	for i, ae := range e {
		msgs[i] = ae.Error()
	}
	return strings.Join(msgs, "; ")
}

func (e RuleErrors) Unwrap() []error {
	errs := make([]error, len(e))
	for i, ae := range e {
		errs[i] = ae
	}
	return errs
}
