package deptree

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

var ErrCyclicDependency = errors.New("deptree: cyclic dependency detected")

type CycleError struct {
	Path []domain.Namespace
}

func (e *CycleError) Error() string {
	parts := make([]string, len(e.Path))
	for i, ns := range e.Path {
		parts[i] = ns.String()
	}
	return fmt.Sprintf("deptree: cyclic dependency detected: %s", strings.Join(parts, " -> "))
}

func (e *CycleError) Unwrap() error {
	return ErrCyclicDependency
}
