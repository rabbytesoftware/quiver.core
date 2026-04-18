package models

import (
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

var ErrNoTargetForOS = errors.New("no target matches this OS")

type Selector interface {
	SelectTarget(
		precompiled map[string]PrecompiledTarget,
		os domain.OS,
	) (domain.Target, error)
}

type AmbiguousTargetError struct {
	Key1 string
	Key2 string
	OS   string
}

func (e *AmbiguousTargetError) Error() string {
	return fmt.Sprintf(
		"ambiguous target for OS %q: keys %q and %q have equal specificity",
		e.OS,
		e.Key1,
		e.Key2,
	)
}
